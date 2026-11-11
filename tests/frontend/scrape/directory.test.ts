import { createRenderer, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useScrapeBatchPreview } from '../../../frontend/src/features/scrape/useScrapeBatchPreview'
import { keys } from '../../../frontend/src/app/query'
import { recognitionEdits } from '../../../frontend/src/recognition'
import type { Recognition, RecognitionItem } from '../../../frontend/src/model'
import type { Runtime } from '../../../frontend/src/app/runtime'

const renderer = createRenderer<object, object>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert: () => {}, remove: () => {}, setText: () => {}, setElementText: () => {}, patchProp: () => {}, parentNode: () => null, nextSibling: () => null,
})
const cleanup: (() => void)[] = []
afterEach(() => { cleanup.splice(0).forEach(fn => fn()); vi.unstubAllGlobals() })
function fixture() {
  const task = ref<Recognition | null>({ purpose: 'scrape', id: 'directory', revision: 2, state: 'completed', paths: ['/target'], items: [], completed: 0, total: 0, error: '', created: '' })
  const busy = ref(false), notice = ref('')
  const refreshRecords = vi.fn(async () => {})
  let state!: ReturnType<typeof useScrapeBatchPreview>
  const app = renderer.createApp({ setup() {
    state = useScrapeBatchPreview({ task, busy, notice, runtime: { refreshRecords } as unknown as Runtime, act: async fn => { busy.value = true; try { await fn() } finally { busy.value = false } } })
    return () => h('div')
  } })
  app.mount({}); cleanup.push(() => app.unmount())
  return { state, task, refreshRecords }
}
const plan = { id: 'preview', targets: ['/target/a.mkv'], artifacts: ['/target/a.nfo'], issues: [], skipped: [], groups: [], content: 'all', onlyMissing: true }

describe('directory scrape preview', () => {
  it('separates workspace caches by purpose and preserves layout edits', () => {
    expect(keys.recognition('latest', 'scrape')).not.toEqual(keys.recognition('latest', 'organize'))
    const layout = { workDir: '/target/show', seasonDir: '', moviePrefix: false, manual: true }
    expect(recognitionEdits([{ path: '/target/show/a.mkv', layout } as RecognitionItem])[0]?.layout).toEqual(layout)
  })
  it('previews one frozen revision and submits only the preview ID', async () => {
    const fetch = vi.fn(async (path: string) => new Response(JSON.stringify(path.endsWith('/preview') ? plan : { id: 'batch', runs: [] })))
    vi.stubGlobal('fetch', fetch)
    const { state, refreshRecords } = fixture()
    expect(state.onlyMissing.value).toBe(true)
    await state.prepare()
    expect(JSON.parse(fetch.mock.calls[0]![1].body)).toEqual({ recognitionId: 'directory', revision: 2, content: 'all', onlyMissing: true })
    expect(state.ready.value).toBe(true)
    await state.execute()
    expect(fetch.mock.calls[1]![0]).toBe('/api/scrape/batches/execute')
    expect(JSON.parse(fetch.mock.calls[1]![1].body)).toEqual({ id: 'preview' })
    expect(refreshRecords).toHaveBeenCalledOnce()
    expect(state.preview.value).toBeNull()
  })
  it('invalidates on options or revision changes and rejects late responses', async () => {
    let resolve!: (value: Response) => void
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn((_path, options) => { signal = options.signal; return new Promise<Response>(done => { resolve = done }) }))
    const { state, task } = fixture()
    const pending = state.prepare()
    task.value!.revision++
    expect(signal?.aborted).toBe(true)
    resolve(new Response(JSON.stringify(plan))); await pending
    expect(state.preview.value).toBeNull()
    state.preview.value = plan as never
    state.onlyMissing.value = false
    expect(state.preview.value).toBeNull()
    await nextTick()
  })
  it('does not execute empty or conflicting previews', async () => {
    const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
    const { state } = fixture()
    state.preview.value = { ...plan, artifacts: [] } as never
    await state.execute()
    state.preview.value = { ...plan, issues: ['collision'] } as never
    await state.execute()
    expect(fetch).not.toHaveBeenCalled()
  })
})
