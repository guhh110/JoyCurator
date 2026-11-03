import { createRenderer, h, nextTick } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { activeConfirmation, resolveConfirmation } from '../../../frontend/src/app/confirm'
import { keys } from '../../../frontend/src/app/query'
import { useDataMaintenance } from '../../../frontend/src/features/settings/useDataMaintenance'

const renderer = createRenderer<object, object>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert() {}, remove() {}, setText() {}, setElementText() {}, patchProp() {},
  parentNode: () => null, nextSibling: () => null,
})
const cleanup: (() => void)[] = []
function mount() {
  let state!: ReturnType<typeof useDataMaintenance>
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const app = renderer.createApp({ setup() { state = useDataMaintenance(); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client }); app.mount({})
  cleanup.push(() => { app.unmount(); client.clear() })
  return { state, client }
}
const preview = { traces: 3, cache: 2, request: { before: '2025-01-01T00:00:00Z', traces: true, cache: true, cacheBefore: 12345 } }
const response = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status })
async function settle() { for (let i=0; i<20; i++) await Promise.resolve(); await nextTick() }
afterEach(async () => {
  if (activeConfirmation.value) resolveConfirmation(false)
  await settle()
  cleanup.splice(0).forEach(fn => fn()); vi.unstubAllGlobals()
})

describe('data maintenance', () => {
  it('requires a preview and invalidates it immediately when selection changes', async () => {
    const fetch = vi.fn().mockResolvedValue(response(preview)); vi.stubGlobal('fetch', fetch)
    const { state } = mount()
    await state.cleanup(); expect(fetch).not.toHaveBeenCalled()
    await state.inspect(); expect(state.count.value).toBe(5)
    state.cache.value = false
    expect(state.preview.value).toBeNull()
    await state.cleanup(); expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0]![0]).toBe('/api/maintenance/preview')
  })
  it('cancels without deletion, then submits the reviewed cutoffs and clears affected caches', async () => {
    const fetch = vi.fn().mockImplementation(async (url: string) => response(url.endsWith('/preview') ? preview : { traces: 2, cache: 2 }))
    vi.stubGlobal('fetch', fetch)
    const { state, client } = mount()
    await state.inspect()
    const cancelled = state.cleanup(); await settle()
    expect(activeConfirmation.value?.options.description).toContain('3 条识别历史和 2 条过期缓存')
    resolveConfirmation(false); await cancelled
    expect(fetch).toHaveBeenCalledTimes(1)
    client.setQueryData(keys.trace('old'), { trace: {} })
    client.setQueryData([...keys.traces, 1, ''], { items: [], total: 0 })
    const confirmed = state.cleanup(); await settle()
    resolveConfirmation(true); await confirmed
    expect(fetch.mock.calls[1]![0]).toBe('/api/maintenance/cleanup')
    expect(JSON.parse(fetch.mock.calls[1]![1].body)).toEqual(preview.request)
    expect(client.getQueryData(keys.trace('old'))).toBeUndefined()
    expect(client.getQueryState([...keys.traces, 1, ''])?.isInvalidated).toBe(true)
    expect(state.preview.value).toBeNull()
  })
  it('retains the preview and cached details after a failed cleanup', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(response(preview)).mockResolvedValue(response({ message: 'disk full' }, 400))
    vi.stubGlobal('fetch', fetch)
    const { state, client } = mount()
    client.setQueryData(keys.trace('old'), { trace: {} })
    await state.inspect()
    const result = state.cleanup(); await settle(); resolveConfirmation(true); await result
    expect(state.preview.value).toEqual(preview)
    expect(client.getQueryData(keys.trace('old'))).toBeDefined()
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
