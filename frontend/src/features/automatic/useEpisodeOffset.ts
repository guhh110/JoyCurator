import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import { api, type Episode, type Recognition, type RecognitionItem, type Season } from '../../api'
import { sortByEpisode, sourceOf } from '../../recognition'
import { applyOffsetMappings, offsetRows, originalEpisodesValid } from './episodeOffset'
import { useToastFeedback } from '../../app/toast'

interface OffsetContext {
  id: string
  revision: number
  tmdbId: number
  title: string
  posterPath: string
  files: RecognitionItem[]
  seasons: Season[]
  paths: string[]
}

export function useEpisodeOffset(options: {
  task: Ref<Recognition | null>
  locked: Readonly<Ref<boolean>>
  commit: (items: RecognitionItem[], id: string, revision: number) => Promise<boolean>
  saveError: Ref<string>
}) {
  const context = ref<OffsetContext | null>(null)
  const source = ref('')
  const season = ref(-1)
  const offsetInput = ref('0')
  const selected = ref(new Set<string>())
  const episodes = ref<Episode[] | null>(null)
  const loading = ref(false)
  const error = ref('')
  useToastFeedback(error)
  const saving = ref(false)
  const allowOverlap = ref(false)
  const retryKey = ref(0)
  let generation = 0
  const sources = computed(() => [...new Set(context.value?.files.map(file => sourceOf(file.path, context.value!.paths)) ?? [])])
  const files = computed(() => sortByEpisode(
    context.value?.files.filter(file => sourceOf(file.path, context.value!.paths) === source.value) ?? [],
    file => ({ season: file.metadata.season, episodes: file.metadata.episodes, path: file.path }),
  ))
  const offset = computed(() => {
    const text = offsetInput.value.trim()
    if (!/^[+-]?\d+$/.test(text)) return NaN
    const value = Number(text)
    return Number.isSafeInteger(value) ? value : NaN
  })
  const stale = computed(() => !!context.value &&
    (options.task.value?.id !== context.value.id || options.task.value.revision !== context.value.revision))
  const rows = computed(() => offsetRows(files.value, options.task.value?.items ?? [], selected.value,
    context.value?.tmdbId ?? 0, season.value, offset.value, episodes.value))
  const chosen = computed(() => rows.value.filter(row => row.selected))
  const overlaps = computed(() => chosen.value.some(row => row.overlaps.length))
  const canApply = computed(() => !!context.value && !options.locked.value && !saving.value && !stale.value &&
    !loading.value && episodes.value !== null && Number.isSafeInteger(offset.value) &&
    context.value.seasons.some(item => item.number === season.value) && chosen.value.length > 0 &&
    chosen.value.every(row => !row.error) && (!overlaps.value || allowOverlap.value))

  function open(files: RecognitionItem[], seasons: Season[], currentSeason: number, currentSource: string) {
    const task = options.task.value
    const entry = files.find(file => file.entry?.type === 'tv')?.entry
    if (!task || !entry || options.locked.value || !seasons.length) return
    generation++
    context.value = JSON.parse(JSON.stringify({ id: task.id, revision: task.revision, tmdbId: entry.tmdbId,
      title: entry.title, posterPath: entry.posterPath, files: files.filter(file => !file.removed && file.entry?.type === 'tv' && file.entry.tmdbId === entry.tmdbId), seasons, paths: task.paths }))
    source.value = sources.value.includes(currentSource) ? currentSource : sources.value[0] ?? ''
    season.value = seasons.some(item => item.number === currentSeason) ? currentSeason : seasons[0]!.number
    offsetInput.value = '0'
    resetSelection()
    allowOverlap.value = false
    episodes.value = null
    error.value = ''
  }
  function close() {
    generation++
    context.value = null
    selected.value = new Set()
    episodes.value = null
    error.value = ''
  }
  function resetSelection() {
    selected.value = new Set(files.value.filter(originalEpisodesValid).map(file => file.path))
  }
  watch(source, resetSelection, { flush: 'sync' })
  watch([offsetInput, season, selected, source], () => { allowOverlap.value = false }, { flush: 'sync' })
  watch([() => context.value?.tmdbId, season, retryKey], async ([id, number], _old, onCleanup) => {
    episodes.value = null
    error.value = ''
    loading.value = false
    if (!id || !context.value?.seasons.some(item => item.number === number)) return
    const controller = new AbortController()
    let valid = true
    onCleanup(() => { valid = false; controller.abort() })
    loading.value = true
    try {
      const value = await api<Episode[]>(`/episodes?id=${id}&season=${number}`, undefined, 'GET', { signal: controller.signal })
      if (valid) episodes.value = value
    } catch (e) {
      if (valid) error.value = (e as Error).message
    } finally {
      if (valid) loading.value = false
    }
  }, { flush: 'sync' })
  function select(path: string, checked: boolean) {
    if (saving.value || options.locked.value || stale.value) return
    const file = files.value.find(file => file.path === path)
    if (!file || !originalEpisodesValid(file)) return
    const next = new Set(selected.value)
    if (checked) next.add(path)
    else next.delete(path)
    selected.value = next
  }
  function selectAll(checked: boolean) {
    if (saving.value || options.locked.value || stale.value) return
    selected.value = new Set(checked ? files.value.filter(originalEpisodesValid).map(file => file.path) : [])
  }
  async function apply() {
    if (!canApply.value || !context.value || !options.task.value) return
    const snapshot = context.value
    const request = generation
    const items = applyOffsetMappings(options.task.value.items, chosen.value.map(row => ({path: row.file.path, season: season.value, episodes: row.episodes})))
    saving.value = true
    error.value = ''
    try {
      const success = await options.commit(items, snapshot.id, snapshot.revision)
      if (request !== generation) return
      if (success) close()
      else error.value = options.saveError.value || '保存失败，请重试'
    } catch (e) {
      if (request === generation) error.value = (e as Error).message
    } finally {
      saving.value = false
    }
  }
  onScopeDispose(close)
  return { context, source, sources, season, offsetInput, offset, selected, rows, chosen, episodes,
    loading, saving, error, stale, allowOverlap, overlaps, canApply, open, close, select, selectAll,
    retry: () => { retryKey.value++ }, apply }
}
