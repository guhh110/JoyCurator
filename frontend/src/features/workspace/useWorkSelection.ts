import { ref, onScopeDispose, type Ref } from 'vue'
import { api, parseEpisodes } from '../../api'
import type { Entry, Recognition, RecognitionItem, Season } from '../../model'
import type { Runtime } from '../../app/runtime'
import type { recognitionGroups } from '../../recognition'
import { useToastFeedback } from '../../app/toast'

type Group = ReturnType<typeof recognitionGroups>[number]
export function useWorkSelection(options: {
  task: Ref<Recognition | null>
  current: Readonly<Ref<Group | undefined>>
  focus: Ref<string>
  filePage: Ref<number>
  runtime: Runtime
  act: (fn: () => Promise<void>) => Promise<void>
  save: () => Promise<void>
}) {
  const { task, current, focus, filePage, runtime, act, save } = options
  let alive = true, searchRequest = 0, editRequest = 0
  let editWorkspace = '', editRevision = 0
  onScopeDispose(() => { alive = false; searchRequest++; editRequest++ })
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
  useToastFeedback(searchError)
  useToastFeedback(editError)
  function openSearch() {
    if (!current.value || current.value.files.every(file => file.execution?.locked || file.execution?.completed)) return
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
    const kind = searchKind.value, query = searchQuery.value.trim(), year = searchYear.value
    searchBusy.value = true
    searchError.value = ''
    try {
      const value = await runtime.client.fetchQuery({
        queryKey: [
          'tmdb',
          'search',
          kind, query, year,
        ],
        queryFn: ({ signal }) =>
          api<Entry[]>(
            `/search?type=${kind}&q=${encodeURIComponent(query)}&year=${encodeURIComponent(year)}`,
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
        if (file.group === searchGroup.value && !file.removed && !file.execution?.locked && !file.execution?.completed) {
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
    if (file.execution?.locked || file.execution?.completed) return
    editing.value = file
    editWorkspace = task.value?.id ?? ''
    editRevision = task.value?.revision ?? 0
    editSeason.value = file.season
    editEpisodes.value = file.episodes.join(',')
    editError.value = ''
    seasons.value = []
    const request = ++editRequest
    const id = file.entry?.tmdbId
    if (file.entry?.type === 'tv')
      try {
        const value = await runtime.client.fetchQuery({
          queryKey: ['tmdb', 'seasons', id],
          queryFn: ({ signal }) =>
            api<Season[]>(
              `/seasons?id=${id}`,
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
    editError.value = ''
    try {
      if (
        !seasons.value.some(season => season.number === editSeason.value)
      )
        throw new Error('请选择 TMDB 中存在的季度')
      const eps = parseEpisodes(editEpisodes.value)
      await act(async () => {
        if (!editing.value) return
        if (task.value?.id !== editWorkspace || task.value.revision !== editRevision)
          throw new Error('工作区已更新，请关闭弹窗后重新打开')
        const file = task.value.items.find(item => item.path === editing.value!.path)
        if (!file || file.execution?.locked || file.execution?.completed)
          throw new Error('文件已提交整理，请刷新后查看关联任务')
        file.season = editSeason.value
        file.episodes = eps
        file.selected = true
        await save()
        editing.value = null
      })
    } catch (e) {
      editError.value = (e as Error).message
    }
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
  return { searchGroup, searchQuery, searchYear, searchKind, searchResults, searchBusy, searchError, editing, editSeason, editEpisodes, seasons, editError, openSearch, search, pick, openEdit, saveEdit, closeSearch, closeEdit }
}
