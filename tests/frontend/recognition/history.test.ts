import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, h, nextTick, ref } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { activeConfirmation, resolveConfirmation } from '../../../frontend/src/app/confirm'
import { keys } from '../../../frontend/src/app/query'
import { lastHistoryPage, useHistoryDetail, useHistoryList, useTraceRemoval } from '../../../frontend/src/features/extraction/history'
import type { Trace } from '../../../frontend/src/features/extraction/model'

const trace = { id: 'trace-1', path: '/media/a.mkv', status: 'completed' } as Trace
const renderer = createRenderer<object, object>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert() {}, remove() {}, setText() {}, setElementText() {}, patchProp() {},
  parentNode: () => null, nextSibling: () => null,
})
const cleanup: (() => void)[] = []
function mount<T>(setup: () => T) {
  let result: T
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const app = renderer.createApp({ setup() { result = setup(); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client })
  app.mount({})
  cleanup.push(() => { app.unmount(); client.clear() })
  return { state: result!, client }
}
afterEach(async () => {
  if (activeConfirmation.value) resolveConfirmation(false)
  await nextTick()
  cleanup.splice(0).forEach(fn => fn())
  vi.unstubAllGlobals()
})
function mockResponse(value: unknown, status = 200) {
  return { ok: status < 400, status, json: async () => value }
}

describe('recognition history queries', () => {
  it('loads only the list with exact path and page filters', async () => {
    const fetch = vi.fn().mockResolvedValue(mockResponse({ items: [], total: 0 }))
    vi.stubGlobal('fetch', fetch)
    const { state } = mount(() => useHistoryList(ref(2), ref('/media/a.mkv')))
    await state.refetch()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0]![0]).toBe('/api/extraction/traces?page=2&path=%2Fmedia%2Fa.mkv')
  })
  it('loads only the selected detail and changes requests when its ID changes', async () => {
    const fetch = vi.fn().mockResolvedValue(mockResponse({ trace, metadata: {}, steps: [] }))
    vi.stubGlobal('fetch', fetch)
    const id = ref('trace-1')
    const { state } = mount(() => useHistoryDetail(id))
    await state.refetch()
    id.value = 'trace-2'
    await nextTick()
    await state.refetch()
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/extraction/traces/trace-1', '/api/extraction/traces/trace-2'])
  })
  it('returns the last valid page after deleting a final row', () => {
    expect(lastHistoryPage(20)).toBe(1)
    expect(lastHistoryPage(21)).toBe(2)
    expect(lastHistoryPage(0)).toBe(1)
  })
})

describe('recognition history cleanup', () => {
  it('does not send a request when cancelled or when a trace is running', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const after = vi.fn()
    const { state } = mount(() => useTraceRemoval(after))
    await state.remove({ ...trace, status: 'running' })
    expect(activeConfirmation.value).toBeNull()
    const removal = state.remove(trace)
    expect(activeConfirmation.value?.options.title).toBe('清理这条识别历史？')
    resolveConfirmation(false)
    await removal
    expect(fetch).not.toHaveBeenCalled()
    expect(after).not.toHaveBeenCalled()
  })
  it('deletes only the confirmed trace and invalidates list caches once', async () => {
    const fetch = vi.fn().mockResolvedValue(mockResponse({ ok: true }))
    vi.stubGlobal('fetch', fetch)
    const after = vi.fn().mockResolvedValue(undefined)
    const { state, client } = mount(() => useTraceRemoval(after))
    client.setQueryData(keys.trace(trace.id), { trace })
    client.setQueryData([...keys.traces, 1, ''], { items: [trace], total: 1 })
    const removal = state.remove(trace)
    await state.remove({ ...trace, id: 'other' })
    resolveConfirmation(true)
    await removal
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0]![0]).toBe('/api/extraction/traces/trace-1')
    expect(fetch.mock.calls[0]![1].method).toBe('DELETE')
    expect(client.getQueryData(keys.trace(trace.id))).toBeUndefined()
    expect(client.getQueryState([...keys.traces, 1, ''])?.isInvalidated).toBe(true)
    expect(after).toHaveBeenCalledTimes(1)
    expect(state.pendingId.value).toBe('')
  })
  it('keeps the record and reports a failed delete without navigating', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse({ message: '清理失败' }, 500)))
    const after = vi.fn()
    const { state, client } = mount(() => useTraceRemoval(after))
    client.setQueryData(keys.trace(trace.id), { trace })
    const removal = state.remove(trace)
    resolveConfirmation(true)
    await removal
    expect(state.error.value).toBe('清理失败')
    expect(client.getQueryData(keys.trace(trace.id))).toEqual({ trace })
    expect(after).not.toHaveBeenCalled()
  })
})
