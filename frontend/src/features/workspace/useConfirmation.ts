import { computed, effectScope, onScopeDispose, ref } from 'vue'
import type { Ref, EffectScope } from 'vue'
import { useToastFeedback } from '../../app/toast'
import { api, parseEpisodes } from '../../api'
import type { ConfirmItem, Entry, EpisodeBasis, Recognition, RecognitionItem } from '../../model'
import { sortByEpisode } from '../../recognition'
import { manualBasis, type CandidateView } from './confirmation'
import { useConfirmationGroup, type ConfirmationGroup } from './useConfirmationGroup'

export interface ConfirmationRow {
  file: RecognitionItem
  view: CandidateView
  entry: Entry | null
  included: boolean
  season: number
  text: string
  basis: EpisodeBasis
  original: string
  error: string
}
export const confirmationWorkKey = (entry: Entry | null) => entry ? `${entry.type}:${entry.tmdbId}` : 'unassigned'
const editable = (file: RecognitionItem) => !file.removed && !file.execution?.locked && !file.execution?.completed
const titleOrder = new Intl.Collator('zh-CN', { numeric: true, sensitivity: 'base' })

export function useConfirmation(options: {
  task: Ref<Recognition | null>
  busy: Readonly<Ref<boolean>>
  scope?: Readonly<Ref<RecognitionItem[]>>
  act: (fn: () => Promise<void>) => Promise<void>
  accept: (task: Recognition) => void
  invalidate: () => void
}) {
  const open = ref(false), loading = ref(false), session = ref(0)
  const context = ref<{ id: string; revision: number } | null>(null)
  const rows = ref<ConfirmationRow[]>([]), works = ref(new Map<string, Entry>())
  const error = ref(''), savingKey = ref('')
  const saving = computed(() => !!savingKey.value)
  useToastFeedback(error)
  let generation = 0
  const controllers = new Set<AbortController>()
  const states = new Map<string, { scope: EffectScope; state: ConfirmationGroup }>()
  const initial = new Map<string, string>()
  const snapshot = (row: ConfirmationRow) => JSON.stringify([confirmationWorkKey(row.entry), row.season, row.text, row.basis])
  const changed = (row: ConfirmationRow) => initial.get(row.file.path) !== snapshot(row)
  const dirty = computed(() => rows.value.some(changed))
  const stale = computed(() => !context.value || context.value.id !== options.task.value?.id || context.value.revision !== options.task.value?.revision)
  const scopedFiles = computed(() => sortByEpisode(options.scope?.value ?? options.task.value?.items.filter(file => !file.removed) ?? [], file => file))
  const rowByPath = computed(() => new Map(rows.value.map(row => [row.file.path, row])))
  const scopedRows = computed(() => scopedFiles.value.flatMap(file => { const row = rowByPath.value.get(file.path); return row ? [row] : [] }))
  const chosen = computed(() => scopedRows.value.filter(row => row.included))
  const entryFor = (file: RecognitionItem) => rowByPath.value.get(file.path)?.entry ?? (rowByPath.value.has(file.path) ? null : file.confirmation ? file.entry : null)
  const groups = computed(() => {
    const grouped = new Map<string, { key: string; entry: Entry | null; files: RecognitionItem[] }>()
    for (const file of scopedFiles.value) {
      const entry = entryFor(file), key = confirmationWorkKey(entry)
      const value = grouped.get(key) ?? { key, entry, files: [] }
      value.files.push(file); grouped.set(key, value)
    }
    return [...grouped.values()].sort((a, b) => a.key === 'unassigned' ? -1 : b.key === 'unassigned' ? 1 : titleOrder.compare(a.entry!.title, b.entry!.title) || a.key.localeCompare(b.key))
  })
  async function read<T>(path: string, body?: unknown): Promise<T> {
    const controller = new AbortController()
    controllers.add(controller)
    try { return await api<T>(path, body, body ? 'POST' : 'GET', { signal: controller.signal }) }
    finally { controllers.delete(controller) }
  }
  function group(key: string): ConfirmationGroup {
    const existing = states.get(key)
    if (existing) return existing.state
    const scope = effectScope(true)
    const state = scope.run(() => useConfirmationGroup({
      key, task: options.task, busy: options.busy, loading, stale,
      saving: computed(() => savingKey.value === key),
      entry: computed(() => works.value.get(key) ?? scopedFiles.value.map(entryFor).find(entry => entry && confirmationWorkKey(entry) === key) ?? null),
      files: computed(() => scopedFiles.value.filter(file => confirmationWorkKey(entryFor(file)) === key)),
      rows: computed(() => scopedRows.value.filter(row => confirmationWorkKey(row.entry) === key)),
      assign: async (entry, targets) => {
        if (options.busy.value || loading.value || stale.value) return
        const allowed = new Set(group(key).chosen.value.map(row => row.file.path))
        const workKey = confirmationWorkKey(entry)
        works.value.set(workKey, entry)
        const assigned: ConfirmationRow[] = []
        for (const row of targets) {
          const file = options.task.value?.items.find(file => file.path === row.file.path)
          if (!allowed.has(row.file.path) || !file || !editable(file)) continue
          row.entry = entry; assigned.push(row)
        }
        await group(workKey).ensure(assigned)
      },
      confirm: () => confirm(key),
    }))!
    states.set(key, { scope, state })
    return state
  }
  function close() {
    generation++; session.value++
    for (const controller of controllers) controller.abort()
    for (const value of states.values()) value.scope.stop()
    controllers.clear(); states.clear(); initial.clear()
    open.value = false; context.value = null; rows.value = []; works.value = new Map()
    error.value = ''; loading.value = false
  }
  onScopeDispose(close)
  async function add(files: RecognitionItem[], select: (file: RecognitionItem) => boolean = () => true) {
    if (!context.value || stale.value || options.busy.value || loading.value) return
    const usable = files.filter(file => editable(file) && !rowByPath.value.has(file.path))
    if (!usable.length) return
    const request = generation, current = { ...context.value }
    loading.value = true
    try {
      const views: CandidateView[] = []
      // The evidence endpoint accepts at most 2000 files per request.
      for (let index = 0; index < usable.length; index += 2000) {
        views.push(...await read<CandidateView[]>(`/recognitions/${current.id}/candidates`, { revision: current.revision, paths: usable.slice(index, index + 2000).map(file => file.path) }))
        if (generation !== request || stale.value) return
      }
      const byPath = new Map(views.map(view => [view.path, view]))
      const added = usable.map(file => {
        const view = byPath.get(file.path)
        if (!view) throw new Error('文件缺少候选快照，请重新读取')
        const values = file.episodes.length ? file.episodes : file.metadata.episodes
        const basis: EpisodeBasis = file.confirmation || !file.traceId || JSON.stringify(values) !== JSON.stringify(file.metadata.episodes)
          ? manualBasis() : { kind: 'suggestion', traceId: file.traceId, index: 0, offset: 0 }
        const entry = file.confirmation ? file.entry : null
        if (entry) works.value.set(confirmationWorkKey(entry), entry)
        const row: ConfirmationRow = { file, view, entry, included: select(file), season: file.season >= 0 ? file.season : file.metadata.season ?? -1, text: values.join(','), basis, original: values.join(','), error: '' }
        initial.set(file.path, snapshot(row))
        return row
      })
      rows.value.push(...added)
    } catch (value) { if (generation === request && (value as Error).name !== 'AbortError') error.value = (value as Error).message }
    finally { if (generation === request) loading.value = false }
  }
  async function start(files: RecognitionItem[], select?: (file: RecognitionItem) => boolean) {
    const task = options.task.value
    if (!task || options.busy.value || task.state === 'running') return
    close(); open.value = true; context.value = { id: task.id, revision: task.revision }
    await add(files, select)
  }
  function manual(row: ConfirmationRow) { row.basis = manualBasis(); row.error = ''; row.original = '手动填写' }
  async function confirm(key: string) {
    const state = group(key), work = state.entry.value
    if (!state.canConfirm.value || !work || !context.value) return false
    const current = generation, saved = { ...context.value }
    const items: ConfirmItem[] = state.chosen.value.map(row => ({ path: row.file.path, type: work.type, tmdbId: work.tmdbId, season: work.type === 'tv' ? row.season : -1, episodes: work.type === 'tv' ? parseEpisodes(row.text) : [], basis: work.type === 'tv' ? row.basis : manualBasis() }))
    let success = false
    savingKey.value = key
    try { await options.act(async () => {
      const task = await read<Recognition>(`/recognitions/${saved.id}/confirm`, { revision: saved.revision, items })
      if (current !== generation) return
      const committed = new Set(items.map(item => item.path)), files = new Map(task.items.map(file => [file.path, file]))
      rows.value = rows.value.flatMap(row => {
        const file = files.get(row.file.path)
        if (!file || !editable(file)) { initial.delete(row.file.path); return [] }
        row.file = file
        if (committed.has(file.path)) {
          row.entry = file.entry; row.season = file.season; row.text = file.episodes.join(',')
          row.basis = file.confirmation?.basis ?? manualBasis(); row.included = false; row.error = ''
          initial.set(file.path, snapshot(row))
        }
        return [row]
      })
      options.accept(task); context.value = { id: task.id, revision: task.revision }
      options.invalidate(); success = true
    }) } finally { savingKey.value = '' }
    return success
  }
  return { open, loading, session, saving, rows, chosen, groups, stale, dirty, error, start, add, close, group, manual }
}
