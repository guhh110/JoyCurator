import {
  computed,
  onActivated,
  onDeactivated,
  onUnmounted,
  ref,
  watch,
} from 'vue'

import { api, parseEpisodes } from '../../api'
import type {
  Batch,
  BatchPreview,
  Entry,
  Episode,
  Recognition,
  RecognitionItem,
  Run,
  Season,
} from '../../api'
import {
  recognitionEdits,
  recognitionGroups,
  sourceOf,
  sourceSummaries,
} from '../../recognition'
import { useQuery } from '@tanstack/vue-query'

import { useRuntime } from '../../app/runtime'
import { useAction } from '../../app/useAction'
import { keys } from '../../app/query'
import { confirmAction } from '../../app/confirm'
import { useEpisodeOffset } from './useEpisodeOffset'

export function useAutomaticRecognition() {
  const runtime = useRuntime()
  const { act: perform, busy, message: error, success: notice } = useAction()
  const active = ref(true)
  const batch = runtime.batch
  const task = ref<Recognition | null>(null),
    preview = ref<BatchPreview | null>(null)
  const browser = ref(false)
  const filter = ref('all'),
    query = ref(''),
    focus = ref(''),
    mode = ref('copy'),
    groupPage = ref(1),
    filePage = ref(1)
  const searchGroup = ref(''),
    searchQuery = ref(''),
    searchYear = ref(''),
    searchKind = ref<'movie' | 'tv'>('movie'),
    searchResults = ref<Entry[]>([]),
    searchBusy = ref(false),
    searchError = ref('')
  const editing = ref<RecognitionItem | null>(null),
    editSeason = ref(-1),
    editEpisodes = ref(''),
    seasons = ref<Season[]>([]),
    editError = ref('')
  const transfer = ref<RecognitionItem | null>(null),
    transferGroup = ref('new')
  const groupSeason = ref<number>(1),
    freshImport = ref(false)
  const groupSeasons = ref<Season[]>([])
  const groupSeasonsLoading = ref(false)
  const groupSeasonsError = ref('')
  const detailView = ref<'files' | 'episodes'>('files')
  const savingSeason = ref(false)
  let seasonRequest = 0
  let alive = true,
    searchRequest = 0,
    editRequest = 0
  const recognizing = computed(() => task.value?.state === 'running')
  const locked = computed(
    () =>
      busy.value ||
      recognizing.value ||
      !!batch.value?.active ||
      runtime.executing.value,
  )
  const groups = computed(() => recognitionGroups(task.value?.items || []))
  // '' shows every added source; otherwise only files from that added path.
  const source = ref('')
  const sources = computed(() =>
    sourceSummaries(task.value?.items || [], task.value?.paths || []),
  )
  const scoped = computed(() =>
    source.value
      ? recognitionGroups(
          (task.value?.items || []).filter(
            (file) =>
              sourceOf(file.path, task.value?.paths || []) === source.value,
          ),
        )
      : groups.value,
  )
  const filtered = computed(() =>
    scoped.value.filter(
      (group) =>
        (filter.value === 'all' ||
          group.files.some((file) => file.state === filter.value)) &&
        `${group.title} ${group.files.map((file) => file.path).join(' ')}`
          .toLowerCase()
          .includes(query.value.toLowerCase()),
    ),
  )
  const visibleGroups = computed(() =>
    filtered.value.slice((groupPage.value - 1) * 30, groupPage.value * 30),
  )
  // Works per status within the chosen source, matching how the filter works.
  const stateCounts = computed(() => {
    const counts: Record<string, number> = { all: scoped.value.length }
    for (const group of scoped.value)
      for (const state of new Set(group.files.map((file) => file.state)))
        counts[state] = (counts[state] || 0) + 1
    return counts
  })
  const current = computed(
    () =>
      scoped.value.find((group) => group.id === focus.value) ||
      filtered.value[0],
  )
  const visibleFiles = computed(
    () =>
      current.value?.files.slice(
        (filePage.value - 1) * 40,
        filePage.value * 40,
      ) || [],
  )
  watch(() => current.value?.entry?.type === 'tv' ? current.value.entry.tmdbId : null, loadGroupSeasons, { immediate: true })
  async function loadGroupSeasons(id: number | null) {
    const request = ++seasonRequest
    groupSeasons.value = []
    groupSeasonsError.value = ''
    groupSeason.value = -1
    groupSeasonsLoading.value = !!id
    if (!id) return
    try {
      const value = await runtime.client.fetchQuery({
        queryKey: ['tmdb', 'seasons', id],
        queryFn: ({ signal }) => api<Season[]>(`/seasons?id=${id}`, undefined, 'GET', { signal }),
        staleTime: 60_000,
      })
      if (!alive || request !== seasonRequest) return
      groupSeasons.value = value
      const existing = current.value?.files[0]?.season
      groupSeason.value = value.find((season) => season.number === existing)?.number ?? value[0]?.number ?? -1
    } catch (e) {
      if (alive && request === seasonRequest) groupSeasonsError.value = (e as Error).message
    } finally {
      if (request === seasonRequest) groupSeasonsLoading.value = false
    }
  }
  const episodeQuery = useQuery({
    queryKey: computed(() => ['tmdb', 'episodes', current.value?.entry?.tmdbId, groupSeason.value]),
    enabled: computed(() => active.value && current.value?.entry?.type === 'tv' && groupSeason.value >= 0 && detailView.value === 'episodes'),
    queryFn: ({ signal }) => api<Episode[]>(`/episodes?id=${current.value!.entry!.tmdbId}&season=${groupSeason.value}`, undefined, 'GET', { signal }),
    staleTime: 60_000,
  })
  async function retryComparison() {
    if (groupSeasonsError.value) {
      await loadGroupSeasons(current.value?.entry?.type === 'tv' ? current.value.entry.tmdbId : null)
    } else if (groupSeason.value >= 0) {
      await episodeQuery.refetch()
    }
  }
  const selected = computed(
    () =>
      task.value?.items.filter(
        (file) => file.selected && !file.removed && file.state === 'ready',
      ) || [],
  )
  const pendingCount = computed(
    () =>
      task.value?.items.filter(
        (file) => !file.removed && file.state !== 'ready',
      ).length || 0,
  )
  const readyCount = computed(
    () =>
      task.value?.items.filter(
        (file) => !file.removed && file.state === 'ready',
      ).length || 0,
  )
  const runStates: Record<string, string> = {
    queued: '等待中',
    running: '进行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
    interrupted: '已中断',
    undone: '已撤销',
  }
  const modes: Record<string, string> = {
    copy: '复制',
    move: '移动',
    hardlink: '硬链接',
  }
  const previewPage = ref(1)
  const previewRows = computed(
    () =>
      preview.value?.rows.slice(
        (previewPage.value - 1) * 30,
        previewPage.value * 30,
      ) || [],
  )

  const taskQuery = useQuery({
    queryKey: computed(() => keys.recognition(task.value?.id || 'latest')),
    queryFn: ({ signal }) =>
      api<Recognition | null>(
        task.value?.id ? `/recognitions/${task.value.id}` : '/recognitions',
        undefined,
        'GET',
        { signal },
      ),
    enabled: computed(() => active.value && !busy.value),
    staleTime: 0,
    refetchInterval: (query) =>
      query.state.data?.state === 'running' || batch.value?.active
        ? 1200
        : false,
  })
  const loading = taskQuery.isPending
  watch(
    taskQuery.data,
    (value) => {
      if (!alive || busy.value || value === undefined) return
      if (value === null) {
        if (!task.value) task.value = null
        return
      }
      if (
        !task.value ||
        (task.value.id === value.id && value.revision >= task.value.revision)
      )
        task.value = JSON.parse(JSON.stringify(value))
    },
    { immediate: true },
  )
  watch(
    () => batch.value?.active,
    (value, previous) => {
      if (previous && !value)
        void runtime.client.invalidateQueries({ queryKey: ['recognitions'] })
    },
  )
  async function act(fn: () => Promise<void>) {
    await perform(async () => {
      await runtime.client.cancelQueries({ queryKey: ['recognitions'] })
      await fn()
      if (alive && task.value)
        runtime.client.setQueryData(
          keys.recognition(task.value.id),
          JSON.parse(JSON.stringify(task.value)),
        )
    })
  }
  async function save(items?: RecognitionItem[]) {
    if (!task.value) return
    preview.value = null
    const original = task.value
    try {
      const value = await api<Recognition>(
        `/recognitions/${original.id}`,
        {
          revision: original.revision,
          items: recognitionEdits(items ?? original.items),
        },
        'PUT',
      )
      if (alive) task.value = value
    } catch (e) {
      // Reload the authoritative snapshot after a rejected edit (including edits
      // made from another tab), so an optimistic checkbox cannot look saved.
      const value = await api<Recognition>(
        `/recognitions/${original.id}`,
      ).catch(() => null)
      if (alive && value) task.value = value
      throw e
    }
  }
  const offsetEditor = useEpisodeOffset({
    task,
    locked,
    saveError: error,
    commit: async (items, id, revision) => {
      let saved = false
      await act(async () => {
        if (task.value?.id !== id || task.value.revision !== revision)
          throw new Error('工作区已更新，请关闭弹窗后重新打开')
        await save(items)
        if (current.value?.entry?.tmdbId === offsetEditor.context.value?.tmdbId)
          groupSeason.value = offsetEditor.season.value
        saved = true
        notice.value = '季集映射已保存，请检查文件校验状态后重新生成整理预览'
      })
      return saved
    },
  })
  function openOffset() {
    if (!current.value || groupSeasonsLoading.value) return
    offsetEditor.open(current.value.files, groupSeasons.value, groupSeason.value, source.value)
  }
  function openBrowser(fresh = false) {
    freshImport.value = fresh
    browser.value = true
  }
  async function start(paths: string[]) {
    browser.value = false
    await act(async () => {
      const value = await api<Recognition>('/recognitions', {
        paths,
        id: freshImport.value ? '' : task.value?.id || '',
      })
      if (!alive) return
      task.value = value
      preview.value = null
      source.value = ''
      focus.value = ''
      filePage.value = 1
      groupPage.value = 1
    })
  }
  async function poll() {
    const [result] = await Promise.all([
      taskQuery.refetch(),
      runtime.batchQuery.refetch(),
    ])
    if (
      result.data &&
      alive &&
      (!task.value || result.data.id === task.value.id) &&
      result.data.revision >= (task.value?.revision ?? 0)
    )
      task.value = JSON.parse(JSON.stringify(result.data))
  }
  async function retry() {
    await act(async () => {
      if (task.value)
        task.value = await api<Recognition>(
          `/recognitions/${task.value.id}/retry`,
          {},
        )
      preview.value = null
    })
  }
  async function cancelRecognition() {
    await act(async () => {
      if (task.value) await api(`/recognitions/${task.value.id}/cancel`, {})
      await poll()
    })
  }
  async function selectFile(file: RecognitionItem) {
    await act(async () => {
      file.selected = !file.selected
      await save()
    })
  }
  async function selectReady(value: boolean) {
    await act(async () => {
      for (const group of filtered.value)
        for (const file of group.files)
          file.selected = value && file.state === 'ready'
      await save()
    })
  }
  async function selectGroup(id: string) {
    await act(async () => {
      const group = scoped.value.find((g) => g.id === id)
      if (!group) return
      const ready = group.files.filter((file) => file.state === 'ready')
      const value = !ready.every((file) => file.selected)
      for (const file of ready) file.selected = value
      await save()
    })
  }
  async function removeFile(file: RecognitionItem) {
    if (!await confirmAction({
      title: '移出文件？',
      description: `将「${file.path.split('/').pop()}」移出整理工作区？磁盘上的文件不会被删除。`,
      confirmLabel: '移出文件',
      destructive: true,
    })) return
    await act(async () => {
      file.removed = true
      file.selected = false
      await save()
      filePage.value = 1
    })
  }
  function openSearch() {
    if (!current.value) return
    searchGroup.value = current.value.id
    searchQuery.value =
      current.value.entry?.title || current.value.files[0]!.metadata.title || ''
    searchYear.value = String(current.value.files[0]!.metadata.year || '')
    searchKind.value =
      current.value.entry?.type ||
      (current.value.files[0]!.metadata.mediaType === 'tv' ? 'tv' : 'movie')
    const found = new Map<string, Entry>()
    for (const file of current.value.files)
      for (const candidate of file.candidates)
        found.set(`${candidate.type}:${candidate.tmdbId}`, candidate)
    searchResults.value = [...found.values()]
    searchError.value = ''
    searchRequest++
  }
  async function search() {
    const request = ++searchRequest
    searchBusy.value = true
    searchError.value = ''
    try {
      const value = await runtime.client.fetchQuery({
        queryKey: [
          'tmdb',
          'search',
          searchKind.value,
          searchQuery.value.trim(),
          searchYear.value,
        ],
        queryFn: ({ signal }) =>
          api<Entry[]>(
            `/search?type=${searchKind.value}&q=${encodeURIComponent(searchQuery.value.trim())}&year=${encodeURIComponent(searchYear.value)}`,
            undefined,
            'GET',
            { signal },
          ),
        staleTime: 60_000,
      })
      if (alive && request === searchRequest) searchResults.value = value
    } catch (e) {
      if (alive && request === searchRequest)
        searchError.value = (e as Error).message
    } finally {
      if (request === searchRequest) searchBusy.value = false
    }
  }
  async function pick(entry: Entry) {
    await act(async () => {
      for (const file of task.value?.items || [])
        if (file.group === searchGroup.value && !file.removed) {
          file.entry = entry
          file.group = `tmdb:${entry.type}:${entry.tmdbId}`
          file.selected = true
        }
      const id = `tmdb:${entry.type}:${entry.tmdbId}`
      await save()
      focus.value = id
      filePage.value = 1
      searchGroup.value = ''
    })
  }
  async function openEdit(file: RecognitionItem) {
    editing.value = file
    editSeason.value = file.season
    editEpisodes.value = file.episodes.join(',')
    editError.value = ''
    seasons.value = []
    const request = ++editRequest
    if (file.entry?.type === 'tv')
      try {
        const value = await runtime.client.fetchQuery({
          queryKey: ['tmdb', 'seasons', file.entry.tmdbId],
          queryFn: ({ signal }) =>
            api<Season[]>(
              `/seasons?id=${file.entry!.tmdbId}`,
              undefined,
              'GET',
              { signal },
            ),
          staleTime: 60_000,
        })
        if (alive && request === editRequest) seasons.value = value
      } catch (e) {
        if (alive && request === editRequest)
          editError.value = (e as Error).message
      }
  }
  async function saveEdit() {
    try {
      if (
        !seasons.value.some(season => season.number === editSeason.value)
      )
        throw new Error('请选择 TMDB 中存在的季度')
      const eps = parseEpisodes(editEpisodes.value)
      await act(async () => {
        if (!editing.value) return
        editing.value.season = editSeason.value
        editing.value.episodes = eps
        editing.value.selected = true
        await save()
        editing.value = null
      })
    } catch (e) {
      editError.value = (e as Error).message
    }
  }
  async function changeGroupSeason(season: number) {
    const group = current.value
    if (locked.value || !group || groupSeasonsLoading.value || !groupSeasons.value.some(item => item.number === season)) return
    const previous = groupSeason.value
    groupSeason.value = season
    if (group.files.every(file => file.season === season)) return
    savingSeason.value = true
    let saved = false
    try {
      await act(async () => {
        for (const file of group.files) file.season = season
        await save()
        saved = true
      })
    } finally {
      savingSeason.value = false
      if (!saved && current.value?.id === group.id) groupSeason.value = previous
    }
  }
  async function assignEpisode(file: RecognitionItem, number: number) {
    if (locked.value || !current.value?.files.includes(file)) return
    if (!episodeQuery.data.value?.some(episode => episode.number === number)) return
    const season = groupSeason.value
    await act(async () => {
      file.season = season
      file.episodes = [number]
      file.selected = true
      await save()
    })
  }
  async function moveGroup() {
    await act(async () => {
      if (!transfer.value) return
      const target = groups.value.find(
        (group) => group.id === transferGroup.value,
      )
      transfer.value.group =
        target?.id ||
        `custom:${Date.now()}:${Math.random().toString(16).slice(2)}`
      transfer.value.entry = target?.entry || null
      transfer.value.selected = !!target?.entry
      const id = transfer.value.group
      await save()
      transfer.value = null
      focus.value = id
      filePage.value = 1
    })
  }
  async function prepare() {
    await act(async () => {
      if (!task.value) return
      preview.value = await api<BatchPreview>('/batches/preview', {
        recognitionId: task.value.id,
        revision: task.value.revision,
        mode: mode.value,
      })
      previewPage.value = 1
    })
  }
  async function execute() {
    await act(async () => {
      if (!preview.value) return
      const value = await api<Batch>('/batches/execute', {
        id: preview.value.id,
      })
      runtime.acceptBatch(value)
      preview.value = null
      notice.value = '已加入整理队列，每部作品分别记录，可独立重试或撤销。'
    })
  }
  async function cancelBatch() {
    await act(async () => {
      if (batch.value) await api(`/batches/${batch.value.id}/cancel`, {})
    })
  }
  function chooseGroup(id: string) {
    focus.value = id
    filePage.value = 1
  }
  function openTransfer(file: RecognitionItem) {
    transfer.value = file
    transferGroup.value = 'new'
  }
  function closeSearch() {
    searchGroup.value = ''
    searchRequest++
    searchBusy.value = false
  }
  function closeEdit() {
    editing.value = null
    editRequest++
  }
  async function removeSource(path: string) {
    if (!task.value) return
    const count = sources.value.find((item) => item.path === path)?.files ?? 0
    if (!await confirmAction({
      title: '移出来源？',
      description: `将「${path}」及其 ${count} 个文件移出整理工作区？磁盘上的文件不会被删除。`,
      confirmLabel: '移出来源',
      destructive: true,
    })) return
    await act(async () => {
      task.value = await api<Recognition>(
        `/recognitions/${task.value!.id}/sources/remove`,
        { path, revision: task.value!.revision },
      )
      preview.value = null
      if (source.value === path) source.value = ''
      changeFilter()
    })
  }
  function chooseSource(path: string) {
    source.value = path
    changeFilter()
  }
  function changeFilter() {
    focus.value = ''
    groupPage.value = 1
    filePage.value = 1
  }
  onActivated(() => {
    active.value = true
  })
  onDeactivated(() => {
    active.value = false
    browser.value = false
    searchGroup.value = ''
    editing.value = null
    transfer.value = null
    preview.value = null
    offsetEditor.close()
    searchRequest++
    editRequest++
  })
  onUnmounted(() => {
    alive = false
    searchRequest++
    editRequest++
  })

  return {
    runtime,
    busy,
    error,
    notice,
    batch,
    task,
    preview,
    browser,
    filter,
    query,
    focus,
    mode,
    groupPage,
    filePage,
    searchGroup,
    searchQuery,
    searchYear,
    searchKind,
    searchResults,
    searchBusy,
    searchError,
    editing,
    editSeason,
    editEpisodes,
    seasons,
    editError,
    transfer,
    transferGroup,
    groupSeason,
    detailView,
    savingSeason,
    changeGroupSeason,
    episodeQuery,
    retryComparison,
    assignEpisode,
    groupSeasons,
    groupSeasonsLoading,
    groupSeasonsError,
    freshImport,
    recognizing,
    locked,
    groups,
    source,
    sources,
    filtered,
    visibleGroups,
    stateCounts,
    current,
    visibleFiles,
    selected,
    pendingCount,
    readyCount,
    runStates,
    modes,
    previewPage,
    previewRows,
    taskQuery,
    loading,
    act,
    save,
    openBrowser,
    start,
    poll,
    retry,
    cancelRecognition,
    selectFile,
    selectReady,
    selectGroup,
    removeFile,
    openSearch,
    search,
    pick,
    openEdit,
    saveEdit,
    offsetEditor,
    openOffset,
    moveGroup,
    prepare,
    execute,
    cancelBatch,
    chooseGroup,
    openTransfer,
    closeSearch,
    closeEdit,
    changeFilter,
    chooseSource,
    removeSource,
  }
}
