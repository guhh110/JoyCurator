import { createRenderer, h, nextTick } from 'vue'
import { environmentManager, focusManager, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { QueueStatus, Run } from '../../../frontend/src/model'
import { createRuntime } from '../../../frontend/src/app/runtime'
import { createQueryClient } from '../../../frontend/src/app/query'

const renderer = createRenderer<object, object>({ createElement: () => ({}), createText: () => ({}), createComment: () => ({}), insert: () => { }, remove: () => { }, setText: () => { }, setElementText: () => { }, patchProp: () => { }, parentNode: () => null, nextSibling: () => null })
const cleanup: (() => void)[] = []
const server = environmentManager.isServer()
beforeEach(() => { vi.useFakeTimers(); environmentManager.setIsServer(() => false); focusManager.setFocused(true) })
afterEach(() => { cleanup.splice(0).forEach(fn => fn()); environmentManager.setIsServer(() => server); focusManager.setFocused(undefined); vi.useRealTimers(); vi.unstubAllGlobals() })
const idle = (): QueueStatus => ({ active: false, recognizing: false, revision: 0, organize: { current: null, queued: 0, queuedBatches: 0, waitReason: '' }, scrape: { current: null, queued: 0, queuedBatches: 0, waitReason: '' } })
const task = (purpose: 'organize' | 'scrape', state = 'running'): Run => ({ id: purpose, purpose, state, title: purpose, mode: 'copy', created: '', error: '', completed: 0, total: 1, undo: false })
async function settle() { for (let i = 0; i < 20; i++)await Promise.resolve(); await vi.advanceTimersByTimeAsync(1); await nextTick() }
async function fixture(initial = idle()) {
  let remote = initial
  const fetch = vi.fn(async (path: string) => new Response(JSON.stringify(path === '/api/queue' ? remote : path.includes('/runs/') ? task(path.includes('/scrape/') ? 'scrape' : 'organize') : {})))
  vi.stubGlobal('fetch', fetch)
  const client = createQueryClient(); let runtime!: ReturnType<typeof createRuntime>
  const app = renderer.createApp({ setup() { runtime = createRuntime(); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client }); app.mount({}); cleanup.push(() => { app.unmount(); client.clear() }); await settle()
  return { runtime, fetch, client, setRemote: (value: QueueStatus) => { remote = value }, requests: () => fetch.mock.calls.filter(([path]) => path === '/api/queue').length }
}
describe('durable two-lane queue runtime', () => {
  it('does not poll idle queues', async () => { const { requests } = await fixture(); expect(requests()).toBe(1); await vi.advanceTimersByTimeAsync(10_000); expect(requests()).toBe(1) })
  it('discovers both lanes after submission and stops after both finish', async () => {
    const { runtime, requests, setRemote } = await fixture(); const active = idle(); active.active = true; active.revision = 1; active.organize.current = task('organize'); active.scrape.current = task('scrape'); active.organize.queued = 2
    setRemote(active); await runtime.refreshQueue(); await settle(); expect(runtime.activeRuns.value).toHaveLength(2); expect(runtime.queue.value?.organize.queued).toBe(2)
    await vi.advanceTimersByTimeAsync(1200); await settle(); expect(requests()).toBe(3)
    setRemote({ ...idle(), revision: 2 }); await vi.advanceTimersByTimeAsync(1200); await settle(); expect(requests()).toBe(4); expect(runtime.executing.value).toBe(false)
    await vi.advanceTimersByTimeAsync(10_000); expect(requests()).toBe(4)
  })
  it('recovers server-owned tasks on mount and on focus without local active IDs', async () => {
    const active = idle(); active.active = true; active.organize.current = task('organize')
    const { runtime, requests, setRemote } = await fixture(active); expect(runtime.activeRuns.value).toHaveLength(1)
    setRemote({ ...idle(), revision: 1 }); await vi.advanceTimersByTimeAsync(1200); await settle(); expect(requests()).toBe(2)
    focusManager.setFocused(false); setRemote({ ...active, revision: 2 }); focusManager.setFocused(true); await settle(); expect(requests()).toBe(3); expect(runtime.executing.value).toBe(true)
  })
  it('observes recognition for settings admission even when execution lanes are idle', async () => {
    const recognizing = { ...idle(), recognizing: true }; const { runtime, requests, setRemote } = await fixture(recognizing)
    expect(runtime.executing.value).toBe(false); setRemote({ ...idle(), revision: 1 }); await vi.advanceTimersByTimeAsync(1200); await settle(); expect(requests()).toBe(2)
    await vi.advanceTimersByTimeAsync(10_000); expect(requests()).toBe(2)
  })
  it('invalidates workspace, library, batch and pending-task caches when progress changes', async () => {
    const { runtime, client, setRemote } = await fixture(); const invalidate = vi.spyOn(client, 'invalidateQueries')
    setRemote({ ...idle(), revision: 1 }); await runtime.refreshQueue(); await settle()
    for (const key of [['recognitions'], ['scrape'], ['batches'], ['queue', 'items']]) expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: key }))
  })
  it.each(['organize', 'scrape'] as const)('opens %s details without a duplicate active-task detail request', async purpose => {
    const active = idle(); active.active = true; active[purpose].current = task(purpose)
    const { runtime, fetch } = await fixture(active); runtime.inspect(task(purpose)); await settle()
    const path = purpose === 'scrape' ? '/api/scrape/runs/scrape' : '/api/runs/organize'
    expect(fetch.mock.calls.filter(([url]) => url === path)).toHaveLength(1)
  })
})
