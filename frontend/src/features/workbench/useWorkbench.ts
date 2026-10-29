import {
  computed,
  onMounted,
  onDeactivated,
  onUnmounted,
  ref,
  watch,
} from 'vue'
import {
  api,
  parseEpisodes,
  type Entry,
  type MediaFile,
  type Episode,
  type Season,
  type Preview,
  type Run,
} from '../../api'
import { useRuntime } from '../../app/runtime'
import { useAction } from '../../app/useAction'

export function useWorkbench() {
  const runtime = useRuntime()
  const { files, entry } = runtime
  const { act, busy, message, success } = useAction()
  const running = runtime.executing
  const settings = computed(() => runtime.settingsQuery.data.value)
  const client = runtime.client
  const season = ref(1),
    seasons = ref<Season[]>([]),
    episodes = ref<Episode[]>([]),
    loadingEpisodes = ref(false)
  const filter = ref(''),
    filePage = ref(1),
    panel = ref('files'),
    mode = ref('copy'),
    browser = ref(false)
  const searchOpen = ref(false),
    query = ref(''),
    kind = ref<'tv' | 'movie'>('tv'),
    year = ref(''),
    searching = ref(false),
    searched = ref(false),
    results = ref<Entry[]>([]),
    searchError = ref('')
  const editing = ref<MediaFile | null>(null),
    editSeason = ref(1),
    editEpisodes = ref(''),
    editError = ref('')
  const preview = ref<Preview | null>(null),
    previewFilter = ref(''),
    previewPage = ref(1),
    omitted = ref(new Set<string>())
  let episodeRequest = 0,
    searchRequest = 0
  const filtered = computed(() =>
    files.value.filter((file) =>
      file.path.toLowerCase().includes(filter.value.toLowerCase()),
    ),
  )
  const visibleFiles = computed(() =>
    filtered.value.slice((filePage.value - 1) * 40, filePage.value * 40),
  )
  const filePages = computed(() =>
    Math.max(1, Math.ceil(filtered.value.length / 40)),
  )
  const mapping = (file: MediaFile) => ({
    ...file,
    season: file.season < 0 ? season.value : file.season,
  })
  const ready = computed(() =>
    !entry.value
      ? []
      : entry.value.type === 'movie'
        ? files.value
        : files.value.filter(
            (file) =>
              mapping(file).season === season.value &&
              file.episodes.length > 0 &&
              file.episodes.every((n) =>
                episodes.value.some((ep) => ep.number === n),
              ),
          ),
  )
  const unmatched = computed(() => files.value.length - ready.value.length)
  const matchedCount = computed(
    () => episodes.value.filter((ep) => assigned(ep.number).length).length,
  )
  const previewRows = computed(() =>
    (preview.value?.rows ?? []).filter((row) =>
      `${row.source} ${row.target}`
        .toLowerCase()
        .includes(previewFilter.value.toLowerCase()),
    ),
  )
  const shownPreview = computed(() =>
    previewRows.value.slice(
      (previewPage.value - 1) * 30,
      previewPage.value * 30,
    ),
  )
  const step = computed(() => (!files.value.length ? 1 : !entry.value ? 2 : 3))

  watch(filter, () => (filePage.value = 1))
  watch(
    filePages,
    (value) => (filePage.value = Math.min(filePage.value, value)),
  )
  watch(previewFilter, () => (previewPage.value = 1))
  watch([query, kind, year], () => {
    searchRequest++
    searched.value = false
    results.value = []
    searching.value = false
  })
  async function add(paths: string[]) {
    browser.value = false
    await act(async () => {
      const added = await api<MediaFile[]>('/scan', { paths })
      const merged = new Map(files.value.map((file) => [file.path, file]))
      for (const file of added) merged.set(file.path, file)
      if (merged.size > 2000)
        throw new Error('工作区最多 2000 个文件，请分批整理')
      files.value = [...merged.values()].sort((a, b) =>
        a.path.localeCompare(b.path, 'zh-CN', { numeric: true }),
      )
      success.value = `已添加 ${added.length} 个视频`
      omitted.value.clear()
    })
  }
  function remove(file: MediaFile) {
    files.value = files.value.filter((item) => item.path !== file.path)
    omitted.value.clear()
  }
  function assigned(n: number) {
    return files.value.filter(
      (file) =>
        mapping(file).season === season.value && file.episodes.includes(n),
    )
  }
  async function search() {
    const request = ++searchRequest
    searching.value = true
    searched.value = true
    searchError.value = ''
    results.value = []
    try {
      const value = await client.fetchQuery({
        queryKey: [
          'tmdb',
          'search',
          kind.value,
          query.value.trim(),
          year.value,
        ],
        queryFn: ({ signal }) =>
          api<Entry[]>(
            `/search?type=${kind.value}&q=${encodeURIComponent(query.value.trim())}&year=${encodeURIComponent(year.value)}`,
            undefined,
            'GET',
            { signal },
          ),
        staleTime: 60_000,
      })
      if (request === searchRequest) results.value = value
    } catch (e) {
      if (request === searchRequest) searchError.value = (e as Error).message
    } finally {
      if (request === searchRequest) searching.value = false
    }
  }
  async function pick(value: Entry) {
    const request = ++episodeRequest
    entry.value = value
    searchOpen.value = false
    episodes.value = []
    seasons.value = []
    omitted.value.clear()
    loadingEpisodes.value = false
    if (value.type === 'movie') return
    loadingEpisodes.value = true
    try {
      const list = await client.fetchQuery({
        queryKey: ['tmdb', 'seasons', value.tmdbId],
        queryFn: ({ signal }) =>
          api<Season[]>(`/seasons?id=${value.tmdbId}`, undefined, 'GET', {
            signal,
          }),
        staleTime: 60_000,
      })
      if (request !== episodeRequest) return
      seasons.value = list
      const known = files.value.find(
        (file) =>
          file.season >= 0 && list.some((s) => s.number === file.season),
      )
      season.value =
        known?.season ??
        list.find((s) => s.number > 0 && s.count > 0)?.number ??
        list[0]?.number ??
        1
      await changeSeason(season.value)
    } catch (e) {
      message.value = (e as Error).message
    } finally {
      if (request === episodeRequest) loadingEpisodes.value = false
    }
  }
  async function changeSeason(n: number) {
    season.value = n
    episodes.value = []
    omitted.value.clear()
    const request = ++episodeRequest
    loadingEpisodes.value = true
    message.value = ''
    try {
      const value = await client.fetchQuery({
        queryKey: ['tmdb', 'episodes', entry.value?.tmdbId, n],
        queryFn: ({ signal }) =>
          api<Episode[]>(
            `/episodes?id=${entry.value?.tmdbId}&season=${n}`,
            undefined,
            'GET',
            { signal },
          ),
        staleTime: 60_000,
      })
      if (request === episodeRequest) episodes.value = value
    } catch (e) {
      if (request === episodeRequest) message.value = (e as Error).message
    } finally {
      if (request === episodeRequest) loadingEpisodes.value = false
    }
  }
  function edit(file: MediaFile) {
    editing.value = file
    editSeason.value = mapping(file).season
    editEpisodes.value = file.episodes.join(',')
    editError.value = ''
  }
  function saveEdit() {
    try {
      if (
        !Number.isInteger(editSeason.value) ||
        editSeason.value < 0 ||
        editSeason.value > 999
      )
        throw new Error('季号为 0–999，0 表示特别篇')
      const values = parseEpisodes(editEpisodes.value)
      if (editing.value)
        files.value = files.value.map((file) =>
          file.path === editing.value!.path
            ? { ...file, season: editSeason.value, episodes: values }
            : file,
        )
      editing.value = null
      omitted.value.clear()
    } catch (e) {
      editError.value = (e as Error).message
    }
  }
  async function prepare() {
    await act(async () => {
      const chosen = ready.value.filter((file) => !omitted.value.has(file.path))
      if (!chosen.length) throw new Error('没有可整理的文件')
      preview.value = await api<Preview>('/preview', {
        type: entry.value?.type,
        tmdbId: entry.value?.tmdbId,
        mode: mode.value,
        files: chosen.map(mapping),
      })
      previewPage.value = 1
      previewFilter.value = ''
    })
  }
  async function skip(path: string) {
    omitted.value = new Set([...omitted.value, path])
    await prepare()
  }
  async function execute() {
    if (!preview.value) return
    await act(async () => {
      const run = await api<Run>('/execute', { id: preview.value!.id })
      runtime.startRun(run)
      preview.value = null
      success.value = '正在整理，进度会自动更新'
    })
  }
  function newPreview() {
    omitted.value.clear()
    void prepare()
  }
  onMounted(() => {
    if (entry.value) void pick(entry.value)
  })
  onDeactivated(() => {
    browser.value = false
    searchOpen.value = false
    editing.value = null
    preview.value = null
  })
  onUnmounted(() => {
    episodeRequest++
    searchRequest++
  })
  return {
    files,
    entry,
    act,
    busy,
    message,
    success,
    running,
    settings,
    season,
    seasons,
    episodes,
    loadingEpisodes,
    filter,
    filePage,
    panel,
    mode,
    browser,
    searchOpen,
    query,
    kind,
    year,
    searching,
    searched,
    results,
    searchError,
    editing,
    editSeason,
    editEpisodes,
    editError,
    preview,
    previewFilter,
    previewPage,
    omitted,
    filtered,
    visibleFiles,
    filePages,
    mapping,
    ready,
    unmatched,
    matchedCount,
    previewRows,
    shownPreview,
    step,
    add,
    remove,
    assigned,
    search,
    pick,
    changeSeason,
    edit,
    saveEdit,
    prepare,
    skip,
    execute,
    newPreview,
  }
}
