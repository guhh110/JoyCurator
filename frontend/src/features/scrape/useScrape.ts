import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/vue-query'
import { api } from '../../api'
import type { MediaMapping, Run, ScrapePreview, ScrapeWork, ScrapeWorkPage } from '../../model'
import { useRuntime } from '../../app/runtime'
import { notify } from '../../app/toast'
import { addSelection, artifactSummary, hasMultipleVersions, visibleEpisodes, type ScrapeStateFilter } from './selection'


export function useScrape() {
  let alive = true
  let previewRequest: AbortController | null = null
  const runtime = useRuntime()
  const client = useQueryClient()
  const search = ref('')
  const query = ref('')
  const active = ref<ScrapeWork | null>(null)
  const selected = ref<string[]>([])
  const expandedSeason = ref<number | null>(null)
  const expandedEpisode = ref('')
  const stateFilter = ref<ScrapeStateFilter>('all')
  const fileSearch = ref('')
  const preview = ref<ScrapePreview | null>(null)
  const artifactSearch = ref('')
  const showArtifacts = ref(false)
  const busy = ref(false)
  const worksQuery = useInfiniteQuery({
    queryKey: computed(() => ['scrape', 'works', query.value]),
    initialPageParam: '',
    queryFn: ({ signal, pageParam }) => api<ScrapeWorkPage>(`/scrape/works?q=${encodeURIComponent(query.value)}&cursor=${encodeURIComponent(pageParam)}`, undefined, 'GET', { signal }),
    getNextPageParam: page => page.nextCursor || undefined,
    refetchInterval: () => runtime.executing.value ? 1500 : false,
  })
  const works = computed(() => worksQuery.data.value?.pages.flatMap(page => page.items) ?? [])
  const cursor = computed(() => worksQuery.data.value?.pages.at(-1)?.nextCursor ?? '')
  const loadingMore = worksQuery.isFetchingNextPage
  let searchTimer: ReturnType<typeof setTimeout> | undefined
  watch(search, value => {
    clearTimeout(searchTimer)
    searchTimer = setTimeout(() => { query.value = value.trim() }, 250)
  })
  onBeforeUnmount(() => { clearTimeout(searchTimer); alive = false; previewRequest?.abort() })
  const detail = useQuery({
    queryKey: computed(() => ['scrape', 'work', active.value?.type, active.value?.tmdbId]),
    queryFn: ({ signal }) => api<MediaMapping[]>(`/scrape/works/${active.value!.type}/${active.value!.tmdbId}`, undefined, 'GET', { signal }),
    enabled: computed(() => !!active.value),
    refetchInterval: () => runtime.executing.value ? 1500 : false,
  })
  const current = computed(() => active.value && detail.data.value?.every(item => item.type === active.value?.type && item.tmdbId === active.value?.tmdbId) ? detail.data.value : [])
  watch(() => detail.data.value, items => {
    if (active.value && items) {
      const available = selected.value.filter(path => items.some(item => item.path === path && item.state !== 'unavailable'))
      if (available.length !== selected.value.length) {
        selected.value = available
        preview.value = null
      }
    }
  })
  const counts = computed(() => current.value.reduce((result, item) => {
    result[item.state] = (result[item.state] ?? 0) + 1
    return result
  }, {} as Partial<Record<MediaMapping['state'], number>>))
  const seasons = computed(() => visibleEpisodes(current.value, stateFilter.value, fileSearch.value))
  const activeSeasonItems = (number: number) => current.value.filter(item => item.season === number)
  const activeEpisodeItems = (season: number, key: string) => current.value.filter(item => item.season === season && item.episodes.join(',') === key)
  const hasMultipleVersionsInWork = (season: number, key: string) => hasMultipleVersions(current.value, season, key)
  const bulkReady = (items: MediaMapping[], completed: boolean) => items.filter(item => completed ? item.state === 'completed' : item.state === 'pending' || item.state === 'failed').length
  const artifactCounts = computed(() => preview.value ? artifactSummary(preview.value) : null)
  const filteredArtifacts = computed(() => (preview.value?.artifacts ?? []).filter(path => path.toLocaleLowerCase().includes(artifactSearch.value.trim().toLocaleLowerCase())))
  const stateNames: Record<MediaMapping['state'], string> = { pending: '待刮削', completed: '已完成', failed: '失败', unavailable: '视频不可用' }
  const tabs: { key: ScrapeStateFilter; label: string }[] = [
    { key: 'all', label: '全部' }, { key: 'pending', label: '待办' }, { key: 'failed', label: '失败' },
    { key: 'completed', label: '已完成' }, { key: 'unavailable', label: '不可用' },
  ]
  function fileName(path: string) { return path.split(/[\\/]/).pop() ?? path }
  function parentName(path: string) { return path.split(/[\\/]/).slice(-2, -1)[0] ?? '' }
  function workTitle(work: ScrapeWork) {
    const parts = work.representative.split(/[\\/]/)
    return parts[parts.length - (work.type === 'tv' ? 3 : 2)] || `TMDB ${work.tmdbId}`
  }
  function episodeLabel(episodes: number[]) { return episodes.length ? episodes.map(number => `E${String(number).padStart(2, '0')}`).join(' + ') : '影片' }
  function openWork(work: ScrapeWork) {
    if (active.value?.type === work.type && active.value.tmdbId === work.tmdbId) return
    previewRequest?.abort()
    active.value = work
    selected.value = []
    preview.value = null
    expandedSeason.value = null
    expandedEpisode.value = ''
    fileSearch.value = ''
    stateFilter.value = 'all'
  }
  function closeWork() { previewRequest?.abort(); active.value = null; selected.value = []; preview.value = null }
  async function loadMore() {
    if (!worksQuery.hasNextPage.value || worksQuery.isFetching.value) return
    const result = await worksQuery.fetchNextPage()
    if (result.error) notify(result.error.message, 'error')
  }
  function bulk(items: MediaMapping[], completed = false) {
    const next = addSelection(selected.value, items, completed)
    if (!next) { notify('单次最多选择 1000 个视频，请按季或单集拆分刮削。', 'error'); return }
    selected.value = next; preview.value = null
  }
  function toggle(item: MediaMapping) {
    if (item.state === 'unavailable' || (item.state === 'completed' && stateFilter.value !== 'completed') || busy.value) return
    if (selected.value.includes(item.path)) selected.value = selected.value.filter(path => path !== item.path)
    else if (selected.value.length >= 1000) { notify('单次最多选择 1000 个视频，请按季拆分刮削。', 'error'); return }
    else selected.value = [...selected.value, item.path]
    preview.value = null
  }
  async function prepare() {
    if (!selected.value.length || busy.value) return
    const work = `${active.value?.type}:${active.value?.tmdbId}`
    const paths = [...selected.value]
    busy.value = true
    try {
      previewRequest?.abort()
      previewRequest = new AbortController()
      const result = await api<ScrapePreview>('/scrape/preview', { paths }, 'POST', { signal: previewRequest.signal })
      if (!alive || `${active.value?.type}:${active.value?.tmdbId}` !== work || paths.join('\0') !== selected.value.join('\0')) return
      preview.value = result
      showArtifacts.value = false
      artifactSearch.value = ''
    }
    catch (e) { if ((e as Error).name !== 'AbortError') notify((e as Error).message, 'error') }
    finally { busy.value = false }
  }
  async function execute() {
    if (!preview.value || preview.value.issues.length || busy.value) return
    busy.value = true
    try {
      const run = await api<Run>('/scrape/execute', { id: preview.value.id })
      runtime.startRun(run)
      selected.value = []; preview.value = null
      await Promise.all([client.invalidateQueries({ queryKey: ['scrape', 'works'] }), client.invalidateQueries({ queryKey: ['scrape', 'work'] })])
    } catch (e) { if ((e as Error).name !== 'AbortError') notify((e as Error).message, 'error') }
    finally { busy.value = false }
  }

  return { query, search, works, cursor, loadingMore, active, selected, expandedSeason, expandedEpisode, stateFilter, fileSearch, preview, artifactSearch, showArtifacts, busy, worksQuery, detail, current, counts, seasons, activeSeasonItems, activeEpisodeItems, hasMultipleVersionsInWork, bulkReady, artifactCounts, filteredArtifacts, stateNames, tabs, fileName, parentName, workTitle, episodeLabel, openWork, closeWork, loadMore, bulk, toggle, prepare, execute }
}
