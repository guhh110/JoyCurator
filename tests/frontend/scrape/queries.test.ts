import { createRenderer, h, nextTick, ref } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useScrape } from '../../../frontend/src/features/scrape/useScrape'

vi.mock('../../../frontend/src/app/runtime', () => ({ useRuntime: () => ({ executing: ref(false), startRun: vi.fn() }) }))
const renderer = createRenderer<object, object>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert: () => {}, remove: () => {}, setText: () => {}, setElementText: () => {}, patchProp: () => {},
  parentNode: () => null, nextSibling: () => null,
})
const cleanup: (() => void)[] = []
afterEach(() => { cleanup.splice(0).forEach(fn => fn()); vi.unstubAllGlobals() })
async function settle() { for (let i = 0; i < 30; i++) await Promise.resolve(); await nextTick() }
function fixture() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let state!: ReturnType<typeof useScrape>
  const app = renderer.createApp({ setup() { state = useScrape(); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client }); app.mount({})
  const close = () => { app.unmount(); client.clear() }
  cleanup.push(close)
  return { state, client, close }
}

describe('scrape query ownership', () => {
  it('keeps every cursor page in the query cache and resets pages when search changes', async () => {
    let count = 1
    vi.stubGlobal('fetch', vi.fn(async (path: string) => {
      const url = new URL(path, 'http://localhost')
      const search = url.searchParams.get('q')
      const second = !!url.searchParams.get('cursor')
      const id = search ? 3 : second ? 2 : 1
      return new Response(JSON.stringify({ items: [{ type: 'movie', tmdbId: id, total: count }], nextCursor: second || search ? '' : 'next' }))
    }))
    const { state, client } = fixture()
    await settle()
    expect(state.works.value.map(work => work.tmdbId)).toEqual([1])
    await state.loadMore()
    expect(state.works.value.map(work => work.tmdbId)).toEqual([1, 2])
    count = 4
    await client.invalidateQueries({ queryKey: ['scrape', 'works'] })
    expect(state.works.value.map(work => work.total)).toEqual([4, 4])
    state.query.value = 'other'
    await settle()
    expect(state.works.value.map(work => work.tmdbId)).toEqual([3])
    expect(state.cursor.value).toBe('')
  })
  it('aborts an unfinished page query when the page is disposed', async () => {
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn((_path: string, options: RequestInit) => new Promise<Response>((_resolve, reject) => {
      signal = options.signal as AbortSignal
      signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
    })))
    const { close } = fixture()
    await settle()
    expect(signal?.aborted).toBe(false)
    close()
    cleanup.length = 0
    expect(signal?.aborted).toBe(true)
  })
})
