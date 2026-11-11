import { createRenderer, h, nextTick, ref } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { renderToString } from 'vue/server-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useWorkspace } from '../../../frontend/src/features/workspace/useWorkspace'
import { activeConfirmation, resolveConfirmation } from '../../../frontend/src/app/confirm'
import { clearToasts } from '../../../frontend/src/app/toast'
import { keys } from '../../../frontend/src/app/query'
import SourceStep from '../../../frontend/src/features/workspace/SourceStep.vue'
import PreviewStep from '../../../frontend/src/features/workspace/PreviewStep.vue'
import type { Entry, Recognition, RecognitionItem } from '../../../frontend/src/model'
import type { CandidateView } from '../../../frontend/src/features/workspace/confirmation'

vi.mock('../../../frontend/src/app/runtime', async () => {
  const { useQueryClient } = await import('@tanstack/vue-query')
  return { useRuntime: () => ({ queue: ref(null), refreshQueue: async () => {}, refreshRecords: async () => {}, client: useQueryClient() }) }
})
const renderer = createRenderer<object, object>({ createElement: () => ({}), createText: () => ({}), createComment: () => ({}), insert: () => {}, remove: () => {}, setText: () => {}, setElementText: () => {}, patchProp: () => {}, parentNode: () => null, nextSibling: () => null })
const cleanup: (() => void)[] = []
afterEach(() => { cleanup.splice(0).forEach(fn => fn()); if (activeConfirmation.value) resolveConfirmation(false); clearToasts(); vi.unstubAllGlobals() })
const response = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status })
const work: Entry = { type: 'tv', tmdbId: 1, title: '作品', originalTitle: '', year: 2024, overview: '', posterPath: '' }
function file(path: string, episodes: number[] = []): RecognitionItem {
  return { path, size: 10, mtime: '1', group: path, state: 'review', selected: false, removed: false, reason: '待确认', season: -1, episodes: [], entry: null, candidates: [work], traceId: 'trace', metadata: { title: '作品', year: 2024, tmdbId: 1, mediaType: 'tv', season: 1, episodes } }
}
function confirmed(item: RecognitionItem): RecognitionItem {
  return { ...item, group: 'tv:1', season: 1, episodes: [1], entry: work, state: 'ready', selected: true, confirmation: { source: 'user', at: '', identity: '', size: 10, mTime: 1, type: 'tv', tmdbId: 1, season: 1, episodes: [1], basis: { kind: 'manual', index: 0, offset: 0 } } }
}
function view(item: RecognitionItem): CandidateView {
  const text = item.path.split('/').pop()!.replace(/\.[^.]+$/, '')
  return { path: item.path, traceId: 'trace', document: { path: item.path, directory: '/source', fileId: item.path, sources: [{ id: 'name', kind: 'filename', depth: 0, text }], tokens: [], candidates: [] }, suggestion: { path: item.path, fileId: item.path, metadata: item.metadata, decisions: {} } }
}
async function settle() { for (let i = 0; i < 20; i++) await Promise.resolve(); await nextTick() }
async function fixture(items = [file('/source/a/1.mp4'), file('/source/a/3.mp4'), file('/source/b/2.mp4')], purpose: 'organize' | 'scrape' = 'organize', cached = false) {
  let stored: Recognition = { id: 'workspace', purpose, revision: 1, state: 'completed', paths: ['/source'], items, total: items.length, completed: items.length, error: '', created: '' }
  let rejectConfirm = false
  const calls: { path: string; body: any }[] = []
  const fetch = vi.fn(async (path: string, options?: RequestInit) => {
    const body = options?.body ? JSON.parse(options.body as string) : null
    calls.push({ path, body })
    if (path.startsWith('/api/seasons')) return response([{ number: 0, name: '特别篇', count: 20 }, { number: 1, name: '第一季', count: 120 }, { number: 2, name: '第二季', count: 20 }])
    if (path.startsWith('/api/episodes')) return response(Array.from({ length: 121 }, (_, number) => ({ number, name: String(number) })))
    if (path.endsWith('/candidates')) return response(stored.items.filter(item => body.paths.includes(item.path)).map(view))
    if (path.endsWith('/confirm')) {
      if (rejectConfirm) return response({ message: '文件已变化，请重新识别' }, 409)
      stored = { ...stored, revision: stored.revision + 1, items: stored.items.map(item => { const edit = body.items.find((value: any) => value.path === item.path); return edit ? { ...confirmed(item), season: edit.season, episodes: edit.episodes } : item }) }
      return response(stored)
    }
    if (path === '/api/batches/preview') return response({ id: 'preview', groups: [], rows: [{source:'/source/a/1.mp4',target:'/target/1.mp4',season:1,episodes:[1],artifacts:[],issues:[],action:'skip'}], issues: [], mode: body.mode, duplicatePolicy: body.duplicatePolicy, required: 0 })
    if (path === '/api/batches/execute') return response({ id: 'batch', runs: [] })
    if (path === '/api/scrape/batches/preview') return response({ id: 'scrape-preview', artifacts: ['/target/show/poster.jpg', '/target/show/Season 01/poster.jpg'], skipped: ['/target/show/tvshow.nfo'], issues: ['输出路径冲突'], required: 100, groups: [], targets: [] })
    if (options?.method === 'PUT') {
      stored = { ...stored, revision: stored.revision + 1, items: stored.items.map(item => ({ ...item, ...body.items.find((edit: any) => edit.path === item.path) })) }
      return response(stored)
    }
    if (path === '/api/recognitions') { stored = { ...stored, id: body.id || 'new-workspace', state: 'running' }; return response(stored) }
    return response(stored)
  })
  vi.stubGlobal('fetch', fetch)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
  if (cached) client.setQueryData(keys.recognition('latest', purpose), stored)
  let w!: ReturnType<typeof useWorkspace>
  const app = renderer.createApp({ setup() { w = useWorkspace(purpose); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client }); app.mount({})
  cleanup.push(() => { app.unmount(); client.clear() })
  await vi.waitFor(() => expect(w.confirmation.rows.value.length).toBeGreaterThan(0))
  return { w, calls, client, fetch, stored: () => stored, reject: () => { rejectConfirm = true } }
}

describe('three-step workspace', () => {
  it('initializes the directory and draft when the latest workspace is already cached', async () => {
    const { w } = await fixture([file('/source/a/1.mp4')], 'organize', true)
    expect(w.step.value).toBe(2)
    expect(w.directory.value).toBe('/source/a')
    expect(w.confirmation.chosen.value.map(row => row.file.path)).toEqual(['/source/a/1.mp4'])
    await w.goStep(1); await w.goStep(2)
    expect(w.confirmation.chosen.value).toHaveLength(1)
  })
  it('restores at confirmation and selects only direct pending files, keeping preview selection separate', async () => {
    const { w } = await fixture([file('/source/a/1.mp4'), confirmed(file('/source/a/done.mp4')), file('/source/a/sub/2.mp4')])
    expect(w.step.value).toBe(2)
    expect(w.confirmation.chosen.value.map(row => row.file.path)).toEqual(['/source/a/1.mp4'])
    expect(w.selected.value.map(item => item.path)).toEqual(['/source/a/done.mp4'])
    expect(w.pendingCount.value).toBe(2)
    expect(w.confirmation.entry.value).toBeNull()
  })
  it('preserves a batch across directories and deselections when returning', async () => {
    const { w } = await fixture()
    w.confirmation.rows.value[0]!.included = false
    await w.confirmation.pick(work); await settle()
    w.confirmation.rows.value[1]!.text = '20'; w.confirmation.manual(w.confirmation.rows.value[1]!)
    await w.chooseDirectory('/source/b')
    expect(w.confirmation.chosen.value.map(row => row.file.path)).toEqual(['/source/a/3.mp4', '/source/b/2.mp4'])
    expect(w.confirmation.rows.value[1]!.text).toBe('20')
    await w.chooseDirectory('/source/a')
    expect(w.confirmation.rows.value[0]!.included).toBe(false)
    expect(w.confirmation.entry.value?.tmdbId).toBe(1)
  })
  it('confirms atomically then keeps excluded files pending in the current directory', async () => {
    const { w, calls } = await fixture()
    await w.confirmation.pick(work); await settle()
    w.confirmation.mode.value = 'digits'; w.confirmation.applyMapping('0'); await settle()
    w.confirmation.rows.value[1]!.included = false
    await w.confirmBatch(); await settle()
    expect(calls.find(call => call.path.endsWith('/confirm'))?.body.items.map((item: any) => item.episodes)).toEqual([[1]])
    expect(w.directory.value).toBe('/source/a')
    expect(w.confirmation.chosen.value.map(row => row.file.path)).toEqual(['/source/a/3.mp4'])
    expect(w.selected.value).toHaveLength(1)
    expect(calls.some(call => call.path.includes('/batches'))).toBe(false)
  })
  it('advances to the next pending directory without executing after confirming a directory', async () => {
    const { w } = await fixture()
    await w.confirmation.pick(work); await settle()
    w.confirmation.mode.value = 'digits'; w.confirmation.applyMapping('0'); await settle()
    await w.confirmBatch(); await settle()
    expect(w.directory.value).toBe('/source/b'); expect(w.step.value).toBe(2)
    expect(w.confirmation.chosen.value[0]?.file.path).toBe('/source/b/2.mp4')
  })
  it('prompts before discarding an edited batch and preserves already confirmed files', async () => {
    const { w } = await fixture([file('/source/a/1.mp4'), confirmed(file('/source/a/done.mp4'))])
    await w.confirmation.pick(work)
    const cancelled = w.goStep(3); await nextTick(); expect(activeConfirmation.value?.options.title).toContain('放弃'); resolveConfirmation(false); await cancelled
    expect(w.step.value).toBe(2); expect(w.confirmation.entry.value).not.toBeNull()
    const continued = w.goStep(3); await nextTick(); resolveConfirmation(true); await continued
    expect(w.step.value).toBe(3); expect(w.confirmation.rows.value).toEqual([]); expect(w.selected.value).toHaveLength(1)
  })
  it('retains the form on confirmation failure and never silently confirms only valid rows', async () => {
    const { w, reject } = await fixture()
    await w.confirmation.pick(work); await settle(); w.confirmation.mode.value = 'digits'; w.confirmation.applyMapping('0'); await settle()
    reject(); await w.confirmBatch()
    expect(w.confirmation.chosen.value).toHaveLength(2); expect(w.selected.value).toHaveLength(0)
    expect(w.error.value).toContain('文件已变化')
  })
  it('stages sources without starting recognition and appends using the existing workspace', async () => {
    const { w, calls } = await fixture()
    await w.goStep(1); w.addPaths(['/source/new', '/source/new'])
    expect(calls.some(call => call.path === '/api/recognitions')).toBe(false)
    expect(w.pendingPaths.value).toEqual(['/source/new'])
    await w.scanSources()
    expect(calls.find(call => call.path === '/api/recognitions')?.body).toEqual({ paths: ['/source/new'], purpose: 'organize', id: 'workspace' })
    expect(w.step.value).toBe(1)
  })
  it('invalidates preview on options, selections and revisions and allows an all-skipped batch', async () => {
    const { w, calls } = await fixture([confirmed(file('/source/a/1.mp4'))])
    await w.goStep(3); await w.previewState.prepare(); expect(w.previewState.previewReady.value).toBe(true)
    w.previewState.duplicatePolicy.value = 'overwrite'; expect(w.previewState.preview.value).toBeNull()
    expect(calls.filter(call => call.path === '/api/batches/preview')).toHaveLength(1)
    await w.previewState.prepare(); expect(w.previewState.preview.value?.duplicatePolicy).toBe('overwrite')
    await w.selectFile(w.task.value!.items[0]!); expect(w.previewState.preview.value).toBeNull()
    await w.selectReady(true); await w.previewState.prepare(); await w.execute()
    expect(calls.find(call => call.path === '/api/batches/execute')?.body).toEqual({id:'preview'})
    expect(w.step.value).toBe(2)
  })
  it('never offers unconfirmed or submitted files for execution even when marked ready and selected', async () => {
    const unconfirmed = { ...file('/source/a/raw.mp4'), state: 'ready' as const, selected: true }
    const submitted = { ...confirmed(file('/source/a/locked.mp4')), execution: {runId:'run',state:'queued',locked:true,completed:false,manualReview:false} }
    const { w } = await fixture([unconfirmed, submitted])
    expect(w.ready.value).toEqual([]); expect(w.workGroups.value).toEqual([])
    expect(w.confirmation.rows.value.some(row => row.file.path === submitted.path)).toBe(false)
  })
  it('renders source staging and execution selection in separate steps', async () => {
    const { w } = await fixture([confirmed(file('/source/a/1.mp4'))])
    await w.goStep(1); w.addPaths(['/source/new'])
    const source = await renderToString(h(SourceStep, { workspace: w }))
    expect(source).toContain('开始识别'); expect(source).toContain('/source/new')
    expect(source).not.toContain('type="checkbox"'); expect(source).not.toContain('确认作品与季集')
    const preview = await renderToString(h(PreviewStep, { workspace: w }))
    expect(preview).toContain('执行文件 /source/a/1.mp4')
    expect(preview).not.toContain('确认文件 '); expect(preview).not.toContain('role="dialog"')
  })
  it('shows scrape layouts and conflicting output paths inline without an enqueue action', async () => {
    const item = { ...confirmed(file('/target/show/Season 01/1.mp4')), layout: { workDir: '/target/show', seasonDir: '/target/show/Season 01', moviePrefix: false, manual: true } }
    const { w } = await fixture([item], 'scrape')
    await w.goStep(3); await w.scrapePreviewState.prepare()
    const html = await renderToString(h(PreviewStep, { workspace: w }))
    expect(html).toContain('作品目录'); expect(html).toContain('季目录')
    expect(html).toContain('/target/show/Season 01/poster.jpg')
    expect(html).toContain('输出路径冲突'); expect(html).toContain('重新生成预览')
    expect(html).not.toContain('加入刮削队列'); expect(html).not.toContain('role="dialog"')
    await w.saveLayouts([{ paths: [item.path], layout: { ...item.layout, seasonDir: '' } }])
    expect(w.scrapePreviewState.preview.value).toBeNull()
  })
})

describe('unified episode draft', () => {
  it('previews a raw-digit mapping without changing rows until applying it', async () => {
    const { w } = await fixture([file('/source/a/1.mp4'),file('/source/a/3.mp4')])
    const s = w.confirmation; await s.pick(work); await settle()
    s.mode.value = 'digits'; s.mappingKey.value = '0'; s.offset.value = '2'
    expect(s.mappingPreview.value.map(value => value.text)).toEqual(['3','5'])
    expect(s.rows.value.map(row => row.text)).toEqual(['',''])
    expect(s.canConfirm.value).toBe(false)
    s.applyMapping(); await settle()
    expect(s.rows.value.map(row => row.text)).toEqual(['3','5'])
    expect(s.canConfirm.value).toBe(true)
  })
  it('preserves manually entered values when a batch mapping cannot match the row', async () => {
    const { w } = await fixture([file('/source/a/1.mp4'),file('/source/a/extra-2.mp4')])
    const s = w.confirmation; await s.pick(work); await settle()
    const row = s.rows.value[1]!; row.text = '9'; s.manual(row)
    s.mode.value = 'digits'; s.mappingKey.value = '0'
    expect(s.mappingPreview.value[1]?.error).not.toBe('')
    s.applyMapping(); await settle()
    expect(row.text).toBe('9'); expect(s.problem(row)).not.toBe(''); expect(s.canConfirm.value).toBe(false)
    s.manual(row); expect(s.canConfirm.value).toBe(true)
  })
  it('applies absolute offsets to original suggestions, including merged episodes and specials', async () => {
    const { w } = await fixture([file('/source/a/1.mp4',[1]),file('/source/a/merged.mp4',[2,4])])
    const s = w.confirmation; await s.pick(work); await settle()
    for (const [offset, values] of [['9',['10','11,13']],['10',['11','12,14']],['0',['1','2,4']],['-1',['0','1,3']]] as const) {
      s.offset.value = offset; s.applyMapping(); await settle(); expect(s.rows.value.map(row => row.text)).toEqual(values)
    }
    s.season.value = 0; s.applySeason(); await settle(); expect(s.canConfirm.value).toBe(true)
    expect(w.task.value!.items[0]!.metadata.episodes).toEqual([1])
  })
  it('checks exact episode membership, invalid offsets and merged values before confirming', async () => {
    const { w } = await fixture([file('/source/a/1.mp4',[1])]); const s = w.confirmation
    await s.pick(work); await settle()
    for (const offset of ['', '1.5', '-2', String(Number.MAX_SAFE_INTEGER)]) { s.offset.value = offset; s.applyMapping(); expect(s.canConfirm.value).toBe(false) }
    s.rows.value[0]!.text = '1,999'; s.manual(s.rows.value[0]!); expect(s.problem(s.rows.value[0]!)).toContain('不存在')
    s.rows.value[0]!.text = '1,1'; expect(s.canConfirm.value).toBe(false)
    s.rows.value[0]!.text = '0,2'; expect(s.canConfirm.value).toBe(true)
  })
  it('reuses the same confirmation mechanism for manual catalog assignment and movie batches', async () => {
    const { w, calls } = await fixture(); const s = w.confirmation
    await s.pick({...work,type:'movie'}); await settle(); expect(s.canConfirm.value).toBe(true)
    await w.confirmBatch()
    expect(calls.find(call => call.path.endsWith('/confirm'))?.body.items.every((item: any) => item.season === -1 && item.episodes.length === 0 && item.basis.kind === 'manual')).toBe(true)
  })
  it('views TMDB seasons without changing the batch or saving any file', async () => {
    const { w, calls } = await fixture([file('/source/a/1.mp4',[1])]); const s = w.confirmation
    await s.pick(work); await settle(); const before = s.rows.value[0]!.season
    s.viewSeason(2); await settle()
    expect(s.episodeLists.value[2]).toHaveLength(121); expect(s.rows.value[0]!.season).toBe(before)
    expect(calls.some(call => call.path.endsWith('/confirm') || call.path === '/api/batches/preview')).toBe(false)
  })
  it('blocks confirmation if a background task locks a selected file without changing the revision', async () => {
    const { w } = await fixture([file('/source/a/1.mp4',[1])]); const s = w.confirmation
    await s.pick(work); await settle(); expect(s.canConfirm.value).toBe(true)
    w.task.value!.items[0]!.execution = {runId:'run',state:'queued',locked:true,completed:false,manualReview:false}
    expect(s.canConfirm.value).toBe(false); expect(s.problem(s.rows.value[0]!)).toContain('已提交')
  })
  it('rejects late season responses after changing works', async () => {
    const { w } = await fixture(); const s = w.confirmation
    let resolve!: (value: Response) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(done => { resolve = done })))
    const old = s.pick(work); await s.pick({...work,type:'movie',tmdbId:2}); resolve(response([{number:9,name:'旧季'}])); await old
    expect(s.entry.value?.type).toBe('movie'); expect(s.seasons.value).toEqual([])
  })
})
