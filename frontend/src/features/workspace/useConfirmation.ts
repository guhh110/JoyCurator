import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import { useToastFeedback } from '../../app/toast'
import { api, parseEpisodes } from '../../api'
import type { ConfirmItem, Entry, EpisodeBasis, Recognition, RecognitionItem, Season, Episode } from '../../model'
import { digitParts, digitPattern, episodeCandidates, episodeValues, manualBasis, takeDigits, type CandidateView } from './confirmation'

export interface ConfirmationRow {
  file: RecognitionItem
  view: CandidateView
  included: boolean
  season: number
  text: string
  basis: EpisodeBasis
  original: string
  error: string
}
export function useConfirmation(options: {
  task: Ref<Recognition | null>
  busy: Readonly<Ref<boolean>>
  act: (fn: () => Promise<void>) => Promise<void>
  accept: (task: Recognition) => void
  invalidate: () => void
}) {
  const open = ref(false), loading = ref(false), context = ref<{ id: string; revision: number } | null>(null)
  const rows = ref<ConfirmationRow[]>([]), entry = ref<Entry | null>(null), query = ref(''), kind = ref<'movie' | 'tv'>('tv')
  const results = ref<Entry[]>([]), seasons = ref<Season[]>([]), season = ref(-1), mode = ref('suggestion'), sourceId = ref('name'), offset = ref('0')
  const samplePath = ref(''), mappingKey = ref(''), error = ref(''), seasonsError = ref('')
  const episodeLists = ref<Record<number, Episode[]>>({}), episodeErrors = ref<Record<number, string>>({})
  const fetchingSeasons = ref(false), searching = ref(false), saving = ref(false)
  useToastFeedback(error)
  let generation = 0, searchRequest = 0, entryRequest = 0
  const controllers = new Set<AbortController>()
  const episodeRequests = new Set<number>()
  const initial = new Map<string, string>()
  const snapshot = (row: ConfirmationRow) => JSON.stringify([row.season, row.text, row.basis])
  const dirty = computed(() => !!entry.value || rows.value.some(row => initial.get(row.file.path) !== snapshot(row)))
  const chosen = computed(() => rows.value.filter(row => row.included))
  const sample = computed(() => chosen.value.find(row => row.file.path === samplePath.value) ?? chosen.value[0])
  const source = computed(() => sample.value?.view.document.sources.find(value => value.id === sourceId.value))
  const parts = computed(() => digitParts(source.value?.text ?? ''))
  const candidates = computed(() => sample.value?.view.document.candidates.filter(candidate => candidate.role === 'episodes') ?? [])
  const stale = computed(() => !context.value || context.value.id !== options.task.value?.id || context.value.revision !== options.task.value?.revision)
  function problem(row: ConfirmationRow) {
    if (!row.included) return ''
    const current = options.task.value?.items.find(file => file.path === row.file.path)
    if (!current || current.removed || current.execution?.locked || current.execution?.completed) return '文件已提交或不在工作区'
    if (!entry.value) return ''
    if (entry.value.type === 'movie') return ''
    if (row.error) return row.error
    if (seasonsError.value) return seasonsError.value
    if (!seasons.value.some(value => value.number === row.season)) return '请选择季度'
    try {
      const values = parseEpisodes(row.text)
      if (!values.length) return '请指定集数'
      if (episodeErrors.value[row.season]) return episodeErrors.value[row.season]!
      const allowed = episodeLists.value[row.season]
      if (!allowed) return '正在校验季集…'
      if (values.some(number => !allowed.some(value => value.number === number))) return '集数在 TMDB 中不存在'
      return ''
    } catch (value) { return (value as Error).message }
  }
  const validation = computed(() => chosen.value.map(problem))
  const canConfirm = computed(() => !!entry.value && !saving.value && !options.busy.value && !loading.value && !fetchingSeasons.value && !stale.value && chosen.value.length > 0 && validation.value.every(value => !value))
  async function read<T>(path: string, body?: unknown): Promise<T> {
    const controller = new AbortController()
    controllers.add(controller)
    try { return await api<T>(path, body, body ? 'POST' : 'GET', { signal: controller.signal }) }
    finally { controllers.delete(controller) }
  }
  function close() {
    generation++; entryRequest++; searchRequest++
    for (const controller of controllers) controller.abort()
    controllers.clear(); episodeRequests.clear(); initial.clear()
    open.value = false; context.value = null; rows.value = []; entry.value = null
    error.value = ''; seasonsError.value = ''; seasons.value = []; episodeLists.value = {}; episodeErrors.value = {}
    loading.value = false; searching.value = false; fetchingSeasons.value = false
  }
  onScopeDispose(close)
  async function add(files: RecognitionItem[], select: (file: RecognitionItem) => boolean = () => true) {
    if (!context.value || stale.value || options.busy.value || loading.value) return
    const usable = files.filter(file => !file.removed && !file.execution?.locked && !file.execution?.completed && !rows.value.some(row => row.file.path === file.path))
    if (!usable.length) return
    const request = generation, current = { ...context.value }
    loading.value = true
    try {
      const views = await read<CandidateView[]>(`/recognitions/${current.id}/candidates`, { revision: current.revision, paths: usable.map(file => file.path) })
      if (generation !== request || stale.value) return
      const added = usable.map(file => {
        const view = views.find(value => value.path === file.path)
        if (!view) throw new Error('文件缺少候选快照，请重新读取')
        const values = file.episodes.length ? file.episodes : file.metadata.episodes
        const basis: EpisodeBasis = file.confirmation || !file.traceId || JSON.stringify(values) !== JSON.stringify(file.metadata.episodes)
          ? manualBasis() : { kind: 'suggestion', traceId: file.traceId, index: 0, offset: 0 }
        const row: ConfirmationRow = { file, view, included: select(file), season: file.season >= 0 ? file.season : file.metadata.season ?? (entry.value?.type === 'tv' && season.value >= 0 ? season.value : -1), text: values.join(','), basis, original: values.join(','), error: '' }
        initial.set(file.path, snapshot(row))
        return row
      })
      rows.value.push(...added)
      const unique = new Map(results.value.map(work => [`${work.type}:${work.tmdbId}`, work]))
      for (const file of usable) for (const work of [...(file.entry ? [file.entry] : []), ...file.candidates]) unique.set(`${work.type}:${work.tmdbId}`, work)
      results.value = [...unique.values()]
      if (!query.value) query.value = usable[0]?.entry?.title || usable[0]?.metadata.title || ''
    } catch (value) { if (generation === request && (value as Error).name !== 'AbortError') error.value = (value as Error).message }
    finally { if (generation === request) loading.value = false }
  }
  async function start(files: RecognitionItem[], select?: (file: RecognitionItem) => boolean) {
    const task = options.task.value
    if (!task || options.busy.value || task.state === 'running') return
    close(); open.value = true; context.value = { id: task.id, revision: task.revision }
    results.value = []; query.value = ''; sourceId.value = 'name'; offset.value = '0'; mode.value = 'suggestion'; mappingKey.value = ''; samplePath.value = ''
    kind.value = files[0]?.entry?.type || (files[0]?.metadata.mediaType === 'movie' ? 'movie' : 'tv')
    await add(files, select)
  }
  async function search() {
    if (searching.value || !query.value.trim()) return
    const request = ++searchRequest, current = generation
    searching.value = true
    try {
      const found = await read<Entry[]>(`/search?type=${kind.value}&q=${encodeURIComponent(query.value)}`)
      if (current === generation && request === searchRequest) results.value = found
    } catch (value) { if (current === generation && (value as Error).name !== 'AbortError') error.value = (value as Error).message }
    finally { if (current === generation && request === searchRequest) searching.value = false }
  }
  async function pick(value: Entry) {
    if (options.busy.value) return
    entry.value = value; kind.value = value.type; seasons.value = []; episodeLists.value = {}; episodeErrors.value = {}; episodeRequests.clear(); seasonsError.value = ''
    const request = ++entryRequest, current = generation
    if (value.type !== 'tv') { fetchingSeasons.value = false; return }
    fetchingSeasons.value = true
    try {
      const values = await read<Season[]>(`/seasons?id=${value.tmdbId}`)
      if (request !== entryRequest || current !== generation) return
      seasons.value = values
      season.value = values.find(item => item.number === chosen.value[0]?.season)?.number ?? (values.filter(item => item.number > 0).length === 1 ? values.find(item => item.number > 0)!.number : -1)
      // This only fills the editable draft; confirmation still requires a click.
      for (const row of chosen.value) if (row.season < 0 && season.value >= 0) row.season = season.value
      loadEpisodes()
    } catch (value) {
      if (request === entryRequest && current === generation && (value as Error).name !== 'AbortError') { seasonsError.value = '季度读取失败，请重试'; error.value = (value as Error).message }
    } finally { if (request === entryRequest && current === generation) fetchingSeasons.value = false }
  }
  watch(() => chosen.value.map(row => row.season).sort().join(','), () => loadEpisodes())
  function loadEpisodes(numbers = chosen.value.map(row => row.season)) {
    const work = entry.value
    if (!work || work.type !== 'tv') return
    const current = generation, request = entryRequest
    for (const number of new Set(numbers.filter(number => seasons.value.some(value => value.number === number)))) {
      if (episodeLists.value[number] || episodeRequests.has(number) || episodeErrors.value[number]) continue
      episodeRequests.add(number)
      read<Episode[]>(`/episodes?id=${work.tmdbId}&season=${number}`).then(list => {
        if (current === generation && request === entryRequest) episodeLists.value = { ...episodeLists.value, [number]: list }
      }).catch(value => {
        if (current === generation && request === entryRequest && value.name !== 'AbortError') { episodeErrors.value = { ...episodeErrors.value, [number]: '集目读取失败，请重试' }; error.value = value.message }
      }).finally(() => { if (current === generation && request === entryRequest) episodeRequests.delete(number) })
    }
  }
  function applySeason() { for (const row of chosen.value) row.season = season.value; loadEpisodes() }
  function manual(row: ConfirmationRow) { row.basis = manualBasis(); row.error = ''; row.original = '手动填写' }
  function mappingFor(row: ConfirmationRow) {
    const delta = /^[+-]?\d+$/.test(offset.value.trim()) ? Number(offset.value) : NaN
    const sourceValue = source.value, sampleValue = sample.value
    try {
      if (!Number.isSafeInteger(delta)) throw new Error('请输入整数偏移')
      let values: number[], basis: EpisodeBasis, original: string
      if (mode.value === 'digits') {
        if (!sourceValue || mappingKey.value === '') throw new Error('请点击样本数字')
        const result = takeDigits(row.view, sourceValue.id, Number(mappingKey.value), digitPattern(sourceValue.text), delta)
        values = result.episodes; basis = result.basis; original = result.original
      } else if (mode.value === 'candidate') {
        const candidate = sampleValue?.view.document.candidates.find(value => value.id === mappingKey.value)
        if (!candidate) throw new Error('请选择集数候选')
        const source = candidate.spans[0]!.sourceId
        const index = episodeCandidates(sampleValue!.view, candidate.ruleId, source).findIndex(value => value.id === candidate.id)
        const picked = episodeCandidates(row.view, candidate.ruleId, source)[index]
        values = episodeValues(picked)
        if (!values.length) throw new Error('缺少有效的对应候选')
        original = values.join(','); values = values.map(number => number + delta)
        basis = { kind: 'candidate', traceId: row.view.traceId, candidateId: picked!.id, index: 0, offset: delta }
      } else {
        values = [...row.view.suggestion.metadata.episodes]
        if (!values.length) throw new Error('缺少建议集数')
        original = values.join(','); values = values.map(number => number + delta)
        basis = { kind: 'suggestion', traceId: row.view.traceId, index: 0, offset: delta }
      }
      if (values.some(number => !Number.isSafeInteger(number) || number < 0)) throw new Error('偏移后集号无效')
      return { text: values.join(','), basis, original, error: '' }
    } catch (value) { return { text: row.text, basis: row.basis, original: row.original, error: (value as Error).message } }
  }
  const mappingPreview = computed(() => chosen.value.map(row => ({ row, ...mappingFor(row) })))
  const canApplyMapping = computed(() => !options.busy.value && !stale.value && mappingPreview.value.some(value => !value.error))
  function applyMapping(selection?: string) {
    if (selection !== undefined) mappingKey.value = selection
    for (const { row, ...result } of mappingPreview.value) Object.assign(row, result)
  }
  async function confirm() {
    if (!canConfirm.value || !entry.value || !context.value) return false
    const current = generation, work = entry.value, saved = context.value
    const items: ConfirmItem[] = chosen.value.map(row => ({ path: row.file.path, type: work.type, tmdbId: work.tmdbId, season: work.type === 'tv' ? row.season : -1, episodes: work.type === 'tv' ? parseEpisodes(row.text) : [], basis: work.type === 'tv' ? row.basis : manualBasis() }))
    let success = false
    saving.value = true
    try { await options.act(async () => {
      const task = await read<Recognition>(`/recognitions/${saved.id}/confirm`, { revision: saved.revision, items })
      if (current !== generation) return
      options.accept(task); options.invalidate(); close(); success = true
    }) } finally { saving.value = false }
    return success
  }
  function retry() {
    error.value = ''
    if (seasonsError.value && entry.value) return pick(entry.value)
    episodeErrors.value = {}; loadEpisodes()
  }
  return { open, loading, saving, rows, entry, query, kind, results, seasons, season, mode, sourceId, offset, samplePath, mappingKey, error, seasonsError, fetchingSeasons, searching, episodeLists, episodeErrors, sample, source, parts, candidates, chosen, stale, dirty, validation, problem, canConfirm, start, add, close, search, pick, applySeason, manual, applyMapping, mappingPreview, canApplyMapping, confirm, retry, viewSeason: (number: number) => loadEpisodes([number]) }
}
