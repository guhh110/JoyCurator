import { createRenderer, h, nextTick, ref } from 'vue'
import { environmentManager, focusManager, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '../../../frontend/src/app/query'
import { useRecords } from '../../../frontend/src/features/records/queries'

const runtime = vi.hoisted(() => ({ executing: null as unknown as ReturnType<typeof ref<boolean>> }))
vi.mock('../../../frontend/src/app/runtime', () => ({ useRuntime: () => runtime }))
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
  runtime.executing = ref(false)
})
afterEach(() => {
  cleanup.splice(0).forEach(fn => fn())
  environmentManager.setIsServer(() => server)
  focusManager.setFocused(undefined)
  vi.useRealTimers(); vi.unstubAllGlobals()
})
async function settle() { for (let i = 0; i < 20; i++) await Promise.resolve(); await vi.advanceTimersByTimeAsync(1); await nextTick() }

describe('record polling by purpose', () => {
  it.each(['organize', 'scrape'] as const)('stops idle polling and refreshes a running %s task until completion', async purpose => {
    let state = 'completed'
    const fetch = vi.fn(async () => new Response(JSON.stringify({ items: [{ id: 'one', state, purpose }], total: 1, page: 1 })))
    vi.stubGlobal('fetch', fetch)
    const client = createQueryClient()
    let records!: ReturnType<typeof useRecords>
    const app = renderer.createApp({ setup() { records = useRecords(ref(1), purpose); return () => h('div') } })
    app.use(VueQueryPlugin, { queryClient: client }); app.mount({})
    cleanup.push(() => { app.unmount(); client.clear() })
    await settle()
    expect(fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(fetch).toHaveBeenCalledTimes(1)
    state = 'running'
    await client.invalidateQueries({ queryKey: ['runs', 'list'] }); await settle()
    expect(fetch).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1500); await settle()
    expect(fetch).toHaveBeenCalledTimes(3)
    state = 'completed'
    await vi.advanceTimersByTimeAsync(1500); await settle()
    expect(records.data.value?.items[0]?.state).toBe('completed')
    expect(fetch).toHaveBeenCalledTimes(4)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(fetch).toHaveBeenCalledTimes(4)
  })
})
