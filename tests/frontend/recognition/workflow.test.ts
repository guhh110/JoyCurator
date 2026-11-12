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
import ConfirmationForm from '../../../frontend/src/features/workspace/ConfirmationForm.vue'
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
async function fixture(items = [file('/source/a/1.mp4'), file('/source/a/3.mp4'), file('/source/b/2.mp4')], purpose: 'organize' | 'scrape' = 'organize', cached = false, completeScan = false) {
  let stored: Recognition = { id: 'workspace', purpose, revision: 1, state: 'completed', paths: ['/source'], items, total: items.length, completed: items.length, error: '', created: '' }
  let rejectConfirm: boolean | number = false
  const calls: { path: string; body: any }[] = []
  const fetch = vi.fn(async (path: string, options?: RequestInit) => {
    const body = options?.body ? JSON.parse(options.body as string) : null
    calls.push({ path, body })
    if (path.startsWith('/api/seasons')) return response([{ number: 0, name: '特别篇', count: 20 }, { number: 1, name: '第一季', count: 120 }, { number: 2, name: '第二季', count: 20 }])
    if (path.startsWith('/api/episodes')) return response(Array.from({ length: 121 }, (_, number) => ({ number, name: String(number) })))
    if (path.endsWith('/candidates')) return response(stored.items.filter(item => body.paths.includes(item.path)).map(view))
    if (path.endsWith('/confirm')) {
      if (rejectConfirm === true || body.items.some((item: any) => item.tmdbId === rejectConfirm)) return response({ message: '文件已变化，请重新识别' }, 409)
      stored = { ...stored, revision: stored.revision + 1, items: stored.items.map(item => {
        const edit = body.items.find((value: any) => value.path === item.path)
        if (!edit) return item
        const entry = stored.items.flatMap(item => [...item.candidates, ...(item.entry ? [item.entry] : [])]).find(entry => entry.type === edit.type && entry.tmdbId === edit.tmdbId) || { ...work, type: edit.type, tmdbId: edit.tmdbId }
        const saved = confirmed(item)
        return { ...saved, entry, group: `${entry.type}:${entry.tmdbId}`, season: edit.season, episodes: edit.episodes, confirmation: { ...saved.confirmation!, type: edit.type, tmdbId: edit.tmdbId, season: edit.season, episodes: edit.episodes, basis: edit.basis } }
      }) }
      return response(stored)
    }
    if (path === '/api/batches/preview') return response({ id: 'preview', groups: [], rows: [{source:'/source/a/1.mp4',target:'/target/1.mp4',season:1,episodes:[1],artifacts:[],issues:[],action:'skip'}], issues: [], mode: body.mode, duplicatePolicy: body.duplicatePolicy, required: 0 })
    if (path === '/api/batches/execute') return response({ id: 'batch', runs: [] })
    if (path === '/api/scrape/batches/preview') return response({ id: 'scrape-preview', artifacts: ['/target/show/poster.jpg', '/target/show/Season 01/poster.jpg'], skipped: ['/target/show/tvshow.nfo'], issues: ['输出路径冲突'], required: 100, groups: [], targets: [] })
    if (options?.method === 'PUT') {
      stored = { ...stored, revision: stored.revision + 1, items: stored.items.map(item => ({ ...item, ...body.items.find((edit: any) => edit.path === item.path) })) }
      return response(stored)
    }
    if (path === '/api/recognitions') {
      stored = { ...stored, id: body.id || 'new-workspace', state: 'running' }
      if (completeScan) stored = { ...stored, state: 'completed', paths: body.paths, items: body.paths.map((path: string) => file(path)), completed: body.paths.length, total: body.paths.length }
      return response(stored)
    }
    return response(stored)
  })
  vi.stubGlobal('fetch', fetch)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
  if (cached) client.setQueryData(keys.recognition('latest', purpose), stored)
  let w!: ReturnType<typeof useWorkspace>
  const app = renderer.createApp({ setup() { w = useWorkspace(purpose); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client }); app.mount({})
  cleanup.push(() => { app.unmount(); client.clear() })
  await vi.waitFor(() => expect(w.task.value).not.toBeNull())
  if (items.some(item => !item.removed)) await vi.waitFor(() => expect(w.confirmation.rows.value.length).toBeGreaterThan(0))
  return { w, calls, client, fetch, stored: () => stored, reject: (id?: number) => { rejectConfirm = id ?? true } }
}

describe('three-step workspace', () => {
  it('shows a single empty state for initial, empty-scan and pending sources', async () => {
    const { w } = await fixture([])
    w.task.value!.paths = []
    let html = await renderToString(h(SourceStep, { workspace: w }))
    expect(html).toContain('选择需要整理的视频或目录')
    expect(html).not.toContain('没有找到视频文件')
    w.task.value!.paths = ['/source/empty']
    html = await renderToString(h(SourceStep, { workspace: w }))
    expect(html).toContain('没有找到视频文件')
    expect(html).not.toContain('选择需要整理的视频或目录')
    w.addPaths(['/source/one.mp4'])
    html = await renderToString(h(SourceStep, { workspace: w }))
    expect(html).toContain('开始识别')
    expect(html).not.toContain('没有找到视频文件')
  })
  it('starts a new workspace when completed videos are added again', async () => {
    const done = { ...confirmed(file('/source/one.mp4')), removed: true, execution: { runId: 'done', state: 'completed', locked: false, completed: true, manualReview: false } }
    const { w, calls } = await fixture([done], 'organize', false, true)
    w.task.value!.paths = [done.path]
    const html = await renderToString(h(SourceStep, { workspace: w }))
    expect(html).toContain('已完成整理，可添加视频或目录开始新一轮')
    expect(html).not.toContain('没有找到视频文件')
    expect(html).toContain('>已完成</span>')
    w.addPaths([done.path, done.path])
    expect(w.pendingPaths.value).toEqual([done.path])
    const staged = await renderToString(h(SourceStep, { workspace: w }))
    expect(staged).toContain('开始识别')
    expect(staged).not.toContain('已完成')
    await w.scanSources()
    expect(calls.find(call => call.path === '/api/recognitions')?.body).toEqual({ paths: [done.path], purpose: 'organize', id: '' })
    await vi.waitFor(() => expect(w.confirmation.rows.value).toHaveLength(1))
    expect(w.step.value).toBe(2)
    expect(w.confirmation.stale.value).toBe(false)
    expect(w.confirmation.rows.value[0]!.file.path).toBe(done.path)
  })
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
    expect(w.confirmation.group('unassigned').entry.value).toBeNull()
    expect(w.confirmation.groups.value.map(group => group.key)).toEqual(['unassigned'])
  })
  it('scopes batch actions to the directory and preserves drafts and deselections when returning', async () => {
    const { w } = await fixture()
    w.confirmation.rows.value[0]!.included = false
    await w.confirmation.group('unassigned').pick(work); await settle()
    w.confirmation.rows.value[1]!.text = '20'; w.confirmation.manual(w.confirmation.rows.value[1]!)
    await w.chooseDirectory('/source/b')
    expect(w.confirmation.chosen.value.map(row => row.file.path)).toEqual(['/source/b/2.mp4'])
    expect(w.confirmation.rows.value[1]!.text).toBe('20')
    await w.chooseDirectory('/source/a')
    expect(w.confirmation.rows.value[0]!.included).toBe(false)
    expect(w.confirmation.chosen.value.map(row => row.file.path)).toEqual(['/source/a/3.mp4'])
    expect(w.confirmation.group('tv:1').entry.value?.tmdbId).toBe(1)
  })
  it('intersects directory, confirmation status and search without resetting the other filters', async () => {
    const { w } = await fixture([file('/source/a/1.mp4'), confirmed(file('/source/a/done.mp4')), file('/source/b/2.mp4'), confirmed(file('/source/b/done.mp4'))])
    w.query.value = 'done'
    w.status.value = 'confirmed'
    await w.chooseDirectory('/source/b')
    expect(w.status.value).toBe('confirmed'); expect(w.query.value).toBe('done')
    expect(w.currentFiles.value.map(file => file.path)).toEqual(['/source/b/done.mp4'])
    expect(w.confirmation.chosen.value).toEqual([])
    await w.chooseDirectory('')
    expect(w.status.value).toBe('confirmed'); expect(w.query.value).toBe('done')
    expect(w.currentFiles.value.map(file => file.path)).toEqual(['/source/a/done.mp4', '/source/b/done.mp4'])
    w.status.value = 'pending'
    expect(w.directory.value).toBe(''); expect(w.query.value).toBe('done')
    expect(w.currentFiles.value).toEqual([])
    w.query.value = ''
    expect(w.currentFiles.value.map(file => file.path)).toEqual(['/source/a/1.mp4', '/source/b/2.mp4'])
    expect(w.confirmation.chosen.value.map(row => row.file.path)).toEqual(['/source/a/1.mp4', '/source/b/2.mp4'])
    w.query.value = '2.mp4'
    await w.chooseDirectory('/source/a')
    expect(w.status.value).toBe('pending'); expect(w.query.value).toBe('2.mp4')
    expect(w.currentFiles.value).toEqual([]); expect(w.confirmation.chosen.value).toEqual([])
    await w.chooseDirectory('/source/b')
    expect(w.currentFiles.value.map(file => file.path)).toEqual(['/source/b/2.mp4'])
    w.status.value = 'confirmed'
    expect(w.directory.value).toBe('/source/b'); expect(w.currentFiles.value).toEqual([])
  })
  it('confirms only filtered files and retains hidden drafts against the new revision', async () => {
    const { w, calls } = await fixture()
    await w.chooseDirectory('')
    await w.confirmation.group('unassigned').pick(work); await settle()
    w.confirmation.group('tv:1').mode.value = 'digits'; w.confirmation.group('tv:1').applyMapping('0'); await settle()
    const hidden = w.confirmation.rows.value.find(row => row.file.path === '/source/a/3.mp4')!
    hidden.text = '20'; w.confirmation.manual(hidden)
    await w.chooseDirectory('/source/a'); w.query.value = '1.mp4'
    await w.confirmGroup('tv:1'); await settle()
    expect(calls.find(call => call.path.endsWith('/confirm'))?.body.items.map((item: any) => item.path)).toEqual(['/source/a/1.mp4'])
    expect(w.directory.value).toBe('/source/a'); expect(w.status.value).toBe('pending'); expect(w.query.value).toBe('1.mp4')
    expect(w.currentFiles.value).toEqual([]); expect(w.confirmation.chosen.value).toEqual([])
    expect(w.confirmation.stale.value).toBe(false)
    w.query.value = ''
    expect(w.confirmation.chosen.value.map(row => row.text)).toEqual(['20'])
    expect(w.confirmation.group('tv:1').canConfirm.value).toBe(true)
    await w.chooseDirectory('/source/b')
    expect(w.confirmation.chosen.value.map(row => row.text)).toEqual(['2'])
  })
  it('confirms atomically then keeps excluded files pending in the current directory', async () => {
    const { w, calls } = await fixture()
    await w.confirmation.group('unassigned').pick(work); await settle()
    w.confirmation.group('tv:1').mode.value = 'digits'; w.confirmation.group('tv:1').applyMapping('0'); await settle()
    w.confirmation.rows.value[1]!.included = false
    await w.confirmGroup('tv:1'); await settle()
    expect(calls.find(call => call.path.endsWith('/confirm'))?.body.items.map((item: any) => item.episodes)).toEqual([[1]])
    expect(w.directory.value).toBe('/source/a')
    expect(w.currentFiles.value.map(file => file.path)).toEqual(['/source/a/3.mp4'])
    expect(w.confirmation.chosen.value).toEqual([])
    expect(w.confirmation.rows.value.find(row => row.file.path === '/source/a/3.mp4')?.included).toBe(false)
    expect(w.selected.value).toHaveLength(1)
    expect(calls.some(call => call.path.includes('/batches'))).toBe(false)
  })
  it('keeps the directory and status filters after confirming its last pending file', async () => {
    const { w } = await fixture()
    await w.confirmation.group('unassigned').pick(work); await settle()
    w.confirmation.group('tv:1').mode.value = 'digits'; w.confirmation.group('tv:1').applyMapping('0'); await settle()
    await w.confirmGroup('tv:1'); await settle()
    expect(w.directory.value).toBe('/source/a'); expect(w.status.value).toBe('pending'); expect(w.step.value).toBe(2)
    expect(w.currentFiles.value).toEqual([]); expect(w.confirmation.chosen.value).toEqual([])
    w.status.value = 'confirmed'
    expect(w.directory.value).toBe('/source/a'); expect(w.currentFiles.value).toHaveLength(2)
    await w.chooseDirectory('/source/b')
    expect(w.status.value).toBe('confirmed'); expect(w.currentFiles.value).toEqual([])
    w.status.value = 'pending'
    expect(w.confirmation.chosen.value[0]?.file.path).toBe('/source/b/2.mp4')
  })
  it('prompts before discarding an edited batch and preserves already confirmed files', async () => {
    const { w } = await fixture([file('/source/a/1.mp4'), confirmed(file('/source/a/done.mp4'))])
    await w.confirmation.group('unassigned').pick(work)
    const cancelled = w.goStep(3); await nextTick(); expect(activeConfirmation.value?.options.title).toContain('放弃'); resolveConfirmation(false); await cancelled
    expect(w.step.value).toBe(2); expect(w.confirmation.group('tv:1').entry.value).not.toBeNull()
    const continued = w.goStep(3); await nextTick(); resolveConfirmation(true); await continued
    expect(w.step.value).toBe(3); expect(w.confirmation.rows.value).toEqual([]); expect(w.selected.value).toHaveLength(1)
  })
  it('retains the form on confirmation failure and never silently confirms only valid rows', async () => {
    const { w, reject } = await fixture()
    await w.confirmation.group('unassigned').pick(work); await settle(); w.confirmation.group('tv:1').mode.value = 'digits'; w.confirmation.group('tv:1').applyMapping('0'); await settle()
    reject(); await w.confirmGroup('tv:1')
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
    const s = w.confirmation.group('tv:1'); await w.confirmation.group('unassigned').pick(work); await settle()
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
    const s = w.confirmation.group('tv:1'); await w.confirmation.group('unassigned').pick(work); await settle()
    const row = s.rows.value[1]!; row.text = '9'; s.manual(row)
    s.mode.value = 'digits'; s.mappingKey.value = '0'
    expect(s.mappingPreview.value[1]?.error).not.toBe('')
    s.applyMapping(); await settle()
    expect(row.text).toBe('9'); expect(s.problem(row)).not.toBe(''); expect(s.canConfirm.value).toBe(false)
    s.manual(row); expect(s.canConfirm.value).toBe(true)
  })
  it('applies absolute offsets to original suggestions, including merged episodes and specials', async () => {
    const { w } = await fixture([file('/source/a/1.mp4',[1]),file('/source/a/merged.mp4',[2,4])])
    const s = w.confirmation.group('tv:1'); await w.confirmation.group('unassigned').pick(work); await settle()
    for (const [offset, values] of [['9',['10','11,13']],['10',['11','12,14']],['0',['1','2,4']],['-1',['0','1,3']]] as const) {
      s.offset.value = offset; s.applyMapping(); await settle(); expect(s.rows.value.map(row => row.text)).toEqual(values)
    }
    s.season.value = 0; s.applySeason(); await settle(); expect(s.canConfirm.value).toBe(true)
    expect(w.task.value!.items[0]!.metadata.episodes).toEqual([1])
  })
  it('checks exact episode membership, invalid offsets and merged values before confirming', async () => {
    const { w } = await fixture([file('/source/a/1.mp4',[1])]); const s = w.confirmation.group('tv:1')
    await w.confirmation.group('unassigned').pick(work); await settle()
    for (const offset of ['', '1.5', '-2', String(Number.MAX_SAFE_INTEGER)]) { s.offset.value = offset; s.applyMapping(); expect(s.canConfirm.value).toBe(false) }
    s.rows.value[0]!.text = '1,999'; s.manual(s.rows.value[0]!); expect(s.problem(s.rows.value[0]!)).toContain('不存在')
    s.rows.value[0]!.text = '1,1'; expect(s.canConfirm.value).toBe(false)
    s.rows.value[0]!.text = '0,2'; expect(s.canConfirm.value).toBe(true)
  })
  it('confirms movie batches with manual episode basis', async () => {
    const { w, calls } = await fixture()
    await w.confirmation.group('unassigned').pick({...work,type:'movie'}); await settle(); expect(w.confirmation.group('movie:1').canConfirm.value).toBe(true)
    await w.confirmGroup('movie:1')
    expect(calls.find(call => call.path.endsWith('/confirm'))?.body.items.every((item: any) => item.season === -1 && item.episodes.length === 0 && item.basis.kind === 'manual')).toBe(true)
  })
  it('blocks confirmation if a background task locks a selected file without changing the revision', async () => {
    const { w } = await fixture([file('/source/a/1.mp4',[1])]); const s = w.confirmation.group('tv:1')
    await w.confirmation.group('unassigned').pick(work); await settle(); expect(s.canConfirm.value).toBe(true)
    w.task.value!.items[0]!.execution = {runId:'run',state:'queued',locked:true,completed:false,manualReview:false}
    expect(s.canConfirm.value).toBe(false); expect(s.problem(s.rows.value[0]!)).toContain('已提交')
  })
  it('rejects late season responses after changing works', async () => {
    const { w } = await fixture(); const s = w.confirmation.group('tv:1')
    let resolve!: (value: Response) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(done => { resolve = done })))
    const old = w.confirmation.group('unassigned').pick(work); await s.pick({...work,type:'movie',tmdbId:2}); resolve(response([{number:9,name:'旧季'}])); await old
    expect(w.confirmation.groups.value.map(group => group.key)).toEqual(['movie:2']); expect(w.confirmation.group('movie:2').seasons.value).toEqual([])
  })
})

describe('per-file work groups', () => {
  const second: Entry = { ...work, tmdbId: 2, title: '另一部作品' }
  const movie: Entry = { ...work, type: 'movie', title: '电影作品' }
  it('keeps season, episode and natural path ordering across directories without sorting by draft edits', async () => {
    const ordered = [
      { ...file('/source/b/10.mp4'), season: 2, episodes: [1] },
      { ...file('/source/a/10.mp4'), season: 1, episodes: [2] },
      { ...file('/source/b/2.mp4'), season: 1, episodes: [1] },
      { ...file('/source/a/2.mp4'), season: 1, episodes: [2] },
    ]
    const { w } = await fixture([...ordered].reverse())
    await w.chooseDirectory(''); await w.confirmation.group('unassigned').pick(work)
    const group = w.confirmation.group('tv:1')
    const expected = ['/source/b/2.mp4','/source/a/2.mp4','/source/a/10.mp4','/source/b/10.mp4']
    expect(group.files.value.map(file => file.path)).toEqual(expected)
    group.rows.value[0]!.season = 9; group.rows.value[0]!.text = '100'
    expect(group.files.value.map(file => file.path)).toEqual(expected)
    expect(w.task.value!.items.map(file => file.path)).toEqual([...ordered].reverse().map(file => file.path))
  })
  it('retains separate work and episode drafts while switching directories and merges the same work in all directories', async () => {
    const { w } = await fixture()
    await w.confirmation.group('unassigned').pick(work)
    w.confirmation.group('tv:1').rows.value[0]!.text = '10'
    await w.chooseDirectory('/source/b')
    expect(w.confirmation.groups.value.map(group => group.key)).toEqual(['unassigned'])
    await w.confirmation.group('unassigned').pick(second)
    const b = w.confirmation.group('tv:2').rows.value[0]!
    b.season = 2; b.text = '4'
    await w.chooseDirectory('/source/a')
    expect(w.confirmation.groups.value.map(group => group.key)).toEqual(['tv:1'])
    expect(w.confirmation.group('tv:1').rows.value.map(row => row.text)).toEqual(['10',''])
    await w.chooseDirectory('/source/b')
    expect(w.confirmation.group('tv:2').rows.value[0]).toBe(b)
    expect([b.season,b.text]).toEqual([2,'4'])
    await w.confirmation.group('tv:2').pick(work)
    await w.chooseDirectory('')
    expect(w.confirmation.groups.value.map(group => [group.key,group.files.length])).toEqual([['tv:1',3]])
    expect(w.confirmation.group('tv:1').rows.value.map(row => [row.season,row.text])).toEqual([[1,'10'],[1,''],[2,'4']])
  })
  it('splits a mixed directory using only checked files and distinguishes movie and TV IDs', async () => {
    const { w } = await fixture()
    w.confirmation.rows.value[1]!.included = false
    await w.confirmation.group('unassigned').pick(work)
    expect(w.confirmation.groups.value.map(group => group.key)).toEqual(['unassigned','tv:1'])
    const pending = w.confirmation.group('unassigned').rows.value[0]!
    pending.included = true
    await w.confirmation.group('unassigned').pick(movie)
    expect(w.confirmation.groups.value.map(group => group.key).sort()).toEqual(['movie:1','tv:1'])
    expect(w.confirmation.group('movie:1').chosen.value.map(row => row.file.path)).toEqual(['/source/a/3.mp4'])
    expect(w.confirmation.group('tv:1').chosen.value.map(row => row.file.path)).toEqual(['/source/a/1.mp4'])
    expect(w.confirmation.dirty.value).toBe(true)
    expect(w.task.value!.items.every(file => file.entry === null)).toBe(true)
  })
  it('scopes season, mapping and reassignment to checked filtered rows and retains hidden selections', async () => {
    const { w } = await fixture()
    await w.chooseDirectory(''); await w.confirmation.group('unassigned').pick(work); await settle()
    const s = w.confirmation.group('tv:1')
    s.mode.value = 'digits'; s.applyMapping('0')
    const before = s.rows.value.map(row => row.text)
    await w.chooseDirectory('/source/a'); w.query.value = '1.mp4'; await settle()
    s.season.value = 2; s.applySeason(); s.offset.value = '9'; s.applyMapping(); await settle()
    await s.pick(second)
    expect(w.confirmation.groups.value.map(group => group.key)).toEqual(['tv:2'])
    await w.chooseDirectory(''); w.query.value = ''; await settle()
    expect(s.rows.value.map(row => [row.file.path,row.season,row.text,row.included])).toEqual([['/source/a/3.mp4',1,before[1],true],['/source/b/2.mp4',1,before[2],true]])
    expect(w.confirmation.group('tv:2').rows.value.map(row => [row.season,row.text])).toEqual([[2,'10']])
  })
  it('confirms each work independently and rebases other drafts to the next revision', async () => {
    const { w, calls } = await fixture()
    await w.confirmation.group('unassigned').pick(work); await settle()
    const a = w.confirmation.group('tv:1'); a.mode.value = 'digits'; a.applyMapping('0')
    await w.chooseDirectory('/source/b'); await w.confirmation.group('unassigned').pick(second); await settle()
    const b = w.confirmation.group('tv:2'); b.rows.value[0]!.text = '20'; b.manual(b.rows.value[0]!)
    await w.chooseDirectory(''); await w.confirmGroup('tv:1'); await settle()
    expect(w.confirmation.groups.value.map(group => group.key)).toEqual(['tv:2'])
    expect(b.rows.value[0]!.text).toBe('20'); expect(b.canConfirm.value).toBe(true)
    expect(w.confirmation.stale.value).toBe(false); expect(w.confirmation.dirty.value).toBe(true)
    await w.confirmGroup('tv:2'); await settle()
    const posted = calls.filter(call => call.path.endsWith('/confirm')).map(call => call.body)
    expect(posted.map(body => body.revision)).toEqual([1,2])
    expect(posted.map(body => body.items.map((item:any) => item.tmdbId))).toEqual([[1,1],[2]])
    expect(w.confirmation.dirty.value).toBe(false)
    expect(w.ready.value.map(file => file.entry!.tmdbId)).toEqual([1,1,2])
  })
  it('allows another work to confirm after a group failure and keeps the failed edits', async () => {
    const { w, reject } = await fixture()
    await w.confirmation.group('unassigned').pick(work); await settle()
    const a = w.confirmation.group('tv:1'); a.mode.value = 'digits'; a.applyMapping('0')
    await w.chooseDirectory('/source/b'); await w.confirmation.group('unassigned').pick(movie); await settle()
    await w.chooseDirectory(''); reject(1)
    await w.confirmGroup('tv:1')
    expect(a.chosen.value.map(row => row.text)).toEqual(['1','3'])
    // Use a distinct work ID for the group accepted by the server.
    await w.confirmation.group('movie:1').pick({...movie,tmdbId:3})
    await w.confirmGroup('movie:3'); await settle()
    expect(w.ready.value).toHaveLength(1); expect(a.chosen.value.map(row => row.text)).toEqual(['1','3'])
    expect(a.canConfirm.value).toBe(true); expect(w.confirmation.stale.value).toBe(false)
  })
  it('revalidates preserved season and merged episode values after changing work', async () => {
    const { w } = await fixture([file('/source/a/1.mp4',[1,2])])
    await w.confirmation.group('unassigned').pick(work); await settle()
    vi.stubGlobal('fetch', vi.fn(async (path:string) => path.startsWith('/api/seasons') ? response([{number:2,name:'第二季',count:1}]) : response([{number:1}])))
    await w.confirmation.group('tv:1').pick(second); await settle()
    const s = w.confirmation.group('tv:2'), row = s.chosen.value[0]!
    expect([row.season,row.text]).toEqual([1,'1,2']); expect(s.problem(row)).toContain('季度')
    row.season = 2; await settle(); expect(s.problem(row)).toContain('不存在')
    row.text = '1'; s.manual(row); expect(s.canConfirm.value).toBe(true)
    row.season = -1; await settle(); expect(row.season).toBe(-1); expect(s.canConfirm.value).toBe(false)
  })
  it('ignores stale search results when the group scope changes and keeps work caches isolated', async () => {
    const { w, fetch } = await fixture()
    await w.chooseDirectory(''); await w.confirmation.group('unassigned').pick(work); await settle()
    const s = w.confirmation.group('tv:1')
    let finish!:(value:Response)=>void
    vi.stubGlobal('fetch', vi.fn((path:string, options?:RequestInit) => path.startsWith('/api/search') ? new Promise<Response>(done => {finish=done}) : fetch(path, options)))
    s.query.value = '迟到结果'; const pending = s.search()
    w.query.value = '1.mp4'; await settle(); finish(response([second])); await pending
    expect(s.results.value.map(entry => entry.tmdbId)).toEqual([1])
    expect(s.searching.value).toBe(false)
    w.task.value = {...w.task.value!,items:w.task.value!.items.map(file => ({...file,execution:{runId:'run',state:'queued',locked:true,completed:false,manualReview:false}}))}
    await s.pick(second)
    expect(w.confirmation.rows.value.every(row => row.entry?.tmdbId === 1)).toBe(true)
    expect(s.chosen.value).toEqual([])
  })
  it('renders 40 files per work group with separate confirmation and a preview-only global footer', async () => {
    const items = Array.from({length:45},(_,index)=>({...file(`/source/a/${index+1}.mp4`,[index+1]),traceId:undefined}))
    const { w } = await fixture(items)
    await w.confirmation.group('unassigned').pick(work); await settle()
    const html = await renderToString(h(ConfirmationForm,{workspace:w}))
    expect((html.match(/\bconfirm-file-row\b/g)||[])).toHaveLength(40)
    expect(html).toContain('1 / 2 · 共 45 条'); expect(html).toContain('确认 45 个文件')
    expect(html).toContain('选择作品组内筛选结果中的文件')
    const footer = html.slice(html.lastIndexOf('<footer'))
    expect(footer).toContain('前往预览'); expect(footer).not.toContain('确认 45')
  })
})
