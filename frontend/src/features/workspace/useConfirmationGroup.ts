import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import { api, parseEpisodes } from '../../api'
import { useToastFeedback } from '../../app/toast'
import type { Entry, Episode, EpisodeBasis, Recognition, RecognitionItem, Season } from '../../model'
import type { ConfirmationRow } from './useConfirmation'
import { digitParts, digitPattern, episodeCandidates, episodeValues, manualBasis, takeDigits } from './confirmation'

export function useConfirmationGroup(options: {
  key: string
  task: Ref<Recognition | null>
  entry: Readonly<Ref<Entry | null>>
  files: Readonly<Ref<RecognitionItem[]>>
  rows: Readonly<Ref<ConfirmationRow[]>>
  busy: Readonly<Ref<boolean>>
  loading: Readonly<Ref<boolean>>
  saving: Readonly<Ref<boolean>>
  stale: Readonly<Ref<boolean>>
  assign: (entry: Entry, rows: ConfirmationRow[]) => Promise<void>
  confirm: () => Promise<boolean>
}) {
  const { entry, files, rows, busy, loading, saving, stale } = options
  const chosen = computed(() => {
    const available = new Set(options.task.value?.items.filter(file => !file.removed && !file.execution?.locked && !file.execution?.completed).map(file => file.path))
    return rows.value.filter(row => row.included && available.has(row.file.path))
  })
  const query = ref(''), kind = ref<'movie' | 'tv'>('tv'), searchResults = ref<Entry[] | null>(null), searching = ref(false)
  const error = ref(''), seasons = ref<Season[]>([]), season = ref(-1), seasonsError = ref(''), fetchingSeasons = ref(false), seasonsLoaded = ref(false)
  const episodeLists = ref<Record<number, Episode[]>>({}), episodeErrors = ref<Record<number, string>>({})
  const mode = ref('suggestion'), sourceId = ref('name'), offset = ref('0'), samplePath = ref(''), mappingKey = ref('')
  const sample = computed(() => chosen.value.find(row => row.file.path === samplePath.value) ?? chosen.value[0])
  const source = computed(() => sample.value?.view.document.sources.find(value => value.id === sourceId.value))
  const parts = computed(() => digitParts(source.value?.text ?? ''))
  const candidates = computed(() => sample.value?.view.document.candidates.filter(candidate => candidate.role === 'episodes') ?? [])
  const results = computed(() => {
    if (searchResults.value) return searchResults.value
    const found = new Map<string, Entry>()
    for (const row of chosen.value) for (const work of (row.entry ? [row.entry] : [])) found.set(`${work.type}:${work.tmdbId}`, work)
    return [...found.values()]
  })
  let alive = true, searchRequest = 0, seasonsRequest: Promise<void> | null = null
  const controllers = new Set<AbortController>(), episodeRequests = new Set<number>()
  useToastFeedback(error)
  onScopeDispose(() => { alive = false; searchRequest++; for (const controller of controllers) controller.abort(); controllers.clear() })
  async function read<T>(path: string): Promise<T> {
    const controller = new AbortController(); controllers.add(controller)
    try { return await api<T>(path, undefined, 'GET', { signal: controller.signal }) }
    finally { controllers.delete(controller) }
  }
  watch(() => chosen.value.map(row => row.file.path).join('\n'), () => {
    searchRequest++; searching.value = false; searchResults.value = null
    query.value = entry.value?.title || chosen.value[0]?.file.metadata.title || ''
    kind.value = entry.value?.type || (chosen.value[0]?.file.metadata.mediaType === 'movie' ? 'movie' : 'tv')
  }, { immediate: true })
  async function search() {
    if (searching.value || !query.value.trim()) return
    const request = ++searchRequest
    searching.value = true
    try {
      const found = await read<Entry[]>(`/search?type=${kind.value}&q=${encodeURIComponent(query.value)}`)
      if (alive && request === searchRequest) searchResults.value = found
    } catch (value) { if (alive && request === searchRequest && (value as Error).name !== 'AbortError') error.value = (value as Error).message }
    finally { if (alive && request === searchRequest) searching.value = false }
  }
  async function pick(work: Entry) {
    if (busy.value || loading.value || stale.value || !chosen.value.length) return
    await options.assign(work, [...chosen.value])
  }
  async function ensure(assigned: ConfirmationRow[] = []) {
    const work = entry.value
    const missing = assigned.filter(row => row.season < 0)
    if (!alive || work?.type !== 'tv' || !rows.value.length) return
    if (!seasonsLoaded.value && !seasonsError.value && !seasonsRequest) {
      fetchingSeasons.value = true
      seasonsRequest = (async () => {
        try {
          const values = await read<Season[]>(`/seasons?id=${work.tmdbId}`)
          if (!alive) return
          seasons.value = values; seasonsLoaded.value = true
        } catch (value) {
          if (alive && (value as Error).name !== 'AbortError') { seasonsError.value = '季度读取失败，请重试'; error.value = (value as Error).message }
        } finally { if (alive) { fetchingSeasons.value = false; seasonsRequest = null } }
      })()
    }
    if (seasonsRequest) await seasonsRequest
    if (!alive || !seasonsLoaded.value) return
    const positive = seasons.value.filter(value => value.number > 0)
    if (positive.length === 1) for (const row of missing) if (chosen.value.includes(row) && row.season < 0) row.season = positive[0]!.number
    loadEpisodes()
  }
  function loadEpisodes() {
    const work = entry.value
    if (!alive || work?.type !== 'tv' || !seasonsLoaded.value) return
    for (const number of new Set(chosen.value.map(row => row.season).filter(number => seasons.value.some(value => value.number === number)))) {
      if (episodeLists.value[number] || episodeRequests.has(number) || episodeErrors.value[number]) continue
      episodeRequests.add(number)
      read<Episode[]>(`/episodes?id=${work.tmdbId}&season=${number}`).then(list => {
        if (alive) episodeLists.value = { ...episodeLists.value, [number]: list }
      }).catch(value => {
        if (alive && value.name !== 'AbortError') { episodeErrors.value = { ...episodeErrors.value, [number]: '集目读取失败，请重试' }; error.value = value.message }
      }).finally(() => { episodeRequests.delete(number) })
    }
  }
  watch(() => chosen.value.map(row => `${row.file.path}:${row.season}`).join('\n'), () => {
    const numbers = new Set(chosen.value.map(row => row.season))
    season.value = numbers.size === 1 ? [...numbers][0]! : -1
    void ensure()
  }, { immediate: true })
  function problem(row: ConfirmationRow) {
    if (!row.included) return ''
    const file = options.task.value?.items.find(file => file.path === row.file.path)
    if (!file || file.removed || file.execution?.locked || file.execution?.completed) return '文件已提交或不在工作区'
    if (!row.entry || row.entry.type === 'movie') return ''
    if (row.error) return row.error
    if (seasonsError.value) return seasonsError.value
    if (fetchingSeasons.value || !seasonsLoaded.value) return ''
    if (!seasons.value.some(value => value.number === row.season)) return '请选择季度'
    try {
      const values = parseEpisodes(row.text)
      if (!values.length) return '请指定集数'
      if (episodeErrors.value[row.season]) return episodeErrors.value[row.season]!
      const allowed = episodeLists.value[row.season]
      if (allowed && values.some(number => !allowed.some(value => value.number === number))) return '集数在 TMDB 中不存在'
      return ''
    } catch (value) { return (value as Error).message }
  }
  const validation = computed(() => chosen.value.map(problem))
  const validating = computed(() => entry.value?.type === 'tv' && chosen.value.length > 0 && (fetchingSeasons.value || !seasonsLoaded.value && !seasonsError.value || chosen.value.some(row => seasons.value.some(value => value.number === row.season) && !episodeLists.value[row.season] && !episodeErrors.value[row.season])))
  const canConfirm = computed(() => !!entry.value && !busy.value && !loading.value && !saving.value && !stale.value && !validating.value && chosen.value.length > 0 && chosen.value.length <= 2000 && validation.value.every(value => !value))
  function applySeason() { if (busy.value || stale.value) return; for (const row of chosen.value) row.season = season.value; loadEpisodes() }
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
  const canApplyMapping = computed(() => !busy.value && !stale.value && mappingPreview.value.some(value => !value.error))
  function applyMapping(selection?: string) {
    if (busy.value || stale.value) return
    if (selection !== undefined) mappingKey.value = selection
    for (const { row, ...result } of mappingPreview.value) Object.assign(row, result)
  }
  function retry() {
    error.value = ''; seasonsError.value = ''; episodeErrors.value = {}
    return ensure()
  }
  return { key: options.key, entry, files, rows, chosen, query, kind, results, searching, seasons, season, seasonsError, fetchingSeasons, episodeErrors, mode, sourceId, offset, samplePath, mappingKey, sample, source, parts, candidates, saving, error, validation, validating, canConfirm, problem, search, pick, ensure, applySeason, manual, applyMapping, mappingPreview, canApplyMapping, retry, confirm: options.confirm }
}
export type ConfirmationGroup = ReturnType<typeof useConfirmationGroup>
