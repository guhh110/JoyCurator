import { createRenderer, h, nextTick } from 'vue'
import { environmentManager, focusManager, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Batch } from '../../../frontend/src/api'
import { createRuntime } from '../../../frontend/src/app/runtime'
import { createQueryClient } from '../../../frontend/src/app/query'

const renderer = createRenderer<object, object>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert: () => {}, remove: () => {}, setText: () => {}, setElementText: () => {}, patchProp: () => {},
  parentNode: () => null, nextSibling: () => null,
})
const cleanup: (() => void)[] = []
const server = environmentManager.isServer()
beforeEach(() => {
  vi.useFakeTimers()
  environmentManager.setIsServer(() => false)
  focusManager.setFocused(true)
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
})
afterEach(() => {
  cleanup.splice(0).forEach(fn => fn())
  environmentManager.setIsServer(() => server)
  focusManager.setFocused(undefined)
  vi.useRealTimers()
  vi.unstubAllGlobals()
})
async function settle() {
  for (let i = 0; i < 20; i++) await Promise.resolve()
  await vi.advanceTimersByTimeAsync(1)
  await nextTick()
}
async function fixture(initial: Batch | null) {
  let remote = initial
  const fetch = vi.fn(async (url: string) => new Response(JSON.stringify(url === '/api/batches' ? remote : {})))
  vi.stubGlobal('fetch', fetch)
  const client = createQueryClient()
  let runtime!: ReturnType<typeof createRuntime>
  const app = renderer.createApp({ setup() { runtime = createRuntime(); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client })
  app.mount({})
  cleanup.push(() => { app.unmount(); client.clear() })
  await settle()
  return { runtime, setRemote: (value: Batch | null) => { remote = value },
    requests: () => fetch.mock.calls.filter(([url]) => url === '/api/batches').length }
}

describe('batch polling lifecycle', () => {
  it.each([null, { id: 'done', active: false, runs: [] }])('does not poll an idle batch (%j)', async initial => {
    const { requests } = await fixture(initial)
    expect(requests()).toBe(1)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(requests()).toBe(1)
  })
  it('starts polling after execution and stops after the completion response', async () => {
    const { runtime, requests, setRemote } = await fixture(null)
    const active: Batch = { id: 'running', active: true, runs: [] }
    setRemote(active)
    runtime.acceptBatch(active)
    await settle()
    await vi.advanceTimersByTimeAsync(1200)
    await settle()
    expect(requests()).toBe(2)
    setRemote({ ...active, active: false })
    await vi.advanceTimersByTimeAsync(1200)
    await settle()
    expect(requests()).toBe(3)
    expect(runtime.executing.value).toBe(false)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(requests()).toBe(3)
  })
  it('recovers a running batch on mount and resumes polling after a focus refresh discovers one', async () => {
    const active: Batch = { id: 'resumed', active: true, runs: [] }
    const { runtime, requests, setRemote } = await fixture(active)
    await vi.advanceTimersByTimeAsync(1200)
    await settle()
    expect(requests()).toBe(2)
    runtime.acceptBatch({ ...active, active: false })
    await settle()
    await vi.advanceTimersByTimeAsync(5000)
    expect(requests()).toBe(2)
    focusManager.setFocused(false)
    setRemote(active)
    focusManager.setFocused(true)
    await settle()
    expect(requests()).toBe(3)
    expect(runtime.batch.value?.active).toBe(true)
    await vi.advanceTimersByTimeAsync(1200)
    await settle()
    expect(requests()).toBe(4)
  })
})
