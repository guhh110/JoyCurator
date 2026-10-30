import { createRenderer, h, nextTick } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAutomaticRecognition } from '../../../frontend/src/features/automatic/useAutomaticRecognition'
import type { BatchPreview, Recognition, RecognitionItem } from '../../../frontend/src/api'

const mocks = vi.hoisted(() => ({ cancelQueries: vi.fn(async () => {}), setQueryData: vi.fn(), fetchQuery: vi.fn() }))
vi.mock('../../../frontend/src/app/runtime', async () => {
  const { ref } = await import('vue')
  return { useRuntime: () => ({ batch: ref(null), executing: ref(false), client: mocks }) }
})
const renderer = createRenderer<object, object>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert: () => {}, remove: () => {}, setText: () => {}, setElementText: () => {}, patchProp: () => {},
  parentNode: () => null, nextSibling: () => null,
})
const cleanup: (() => void)[] = []
afterEach(() => { cleanup.splice(0).forEach(fn => fn()); vi.unstubAllGlobals(); vi.clearAllMocks() })
const response = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status })
async function settle() { for (let i = 0; i < 15; i++) await Promise.resolve(); await nextTick() }
async function fixture() {
  mocks.fetchQuery.mockResolvedValue([{ number: 1, name: '第一季', count: 20 }])
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  let state!: ReturnType<typeof useAutomaticRecognition>
  const app = renderer.createApp({ setup() { state = useAutomaticRecognition(); return () => h('div') } })
  app.use(VueQueryPlugin, { queryClient: client })
  app.mount({})
  cleanup.push(() => { app.unmount(); client.clear() })
  const item: RecognitionItem = { path: '/source/a.mkv', group: 'tv:1', state: 'review', reason: '', selected: false, removed: false,
    size: 1, mtime: '1', season: 1, episodes: [1], probe: { codec: '', resolution: '' },
    metadata: { title: 'Show', year: null, season: 1, episodes: [1], tmdbId: 1, mediaType: 'tv', releaseTag: null },
    entry: { type: 'tv', tmdbId: 1, title: 'Show', originalTitle: '', year: 0, overview: '', posterPath: '' }, candidates: [] }
  state.task.value = { id: 'workspace', revision: 3, state: 'completed', paths: ['/source'], items: [item], completed: 1, total: 1, error: '', created: '' }
  await settle()
  return state
}

describe('workspace offset save integration', () => {
  it('submits a full revision-bound workspace edit, invalidates preview and updates query cache', async () => {
    let posted: any
    const fetch = vi.fn(async (url: string, options: RequestInit) => {
      if (url.startsWith('/api/episodes')) return response([{ number: 10, season: 1 }])
      if (options.method !== 'PUT') return response(null)
      posted = JSON.parse(options.body as string)
      return response({ ...state.task.value!, revision: 4, items: state.task.value!.items.map(item => ({ ...item, episodes: [10], state: 'ready' })) })
    })
    vi.stubGlobal('fetch', fetch)
    const state = await fixture()
    state.preview.value = { id: 'old-preview' } as BatchPreview
    state.openOffset(); await settle()
    state.offsetEditor.target.value = 10; state.offsetEditor.selectAll(true)
    await state.offsetEditor.apply()
    expect(posted).toMatchObject({ revision: 3, items: [{ path: '/source/a.mkv', episodes: [10], selected: false }] })
    expect(posted.items[0]).not.toHaveProperty('metadata')
    expect(state.task.value!.revision).toBe(4)
    expect(state.task.value!.items[0]!.metadata.episodes).toEqual([1])
    expect(state.preview.value).toBeNull()
    expect(state.groupSeason.value).toBe(1)
    expect(mocks.setQueryData).toHaveBeenCalled()
    expect(fetch.mock.calls.some(([, options]) => options.method === 'PUT')).toBe(true)
    expect(fetch.mock.calls.every(([url]) => !url.includes('/batches'))).toBe(true)
  })
  it('reloads authoritative state after a failed save without showing an unsaved correction', async () => {
    let snapshot: Recognition
    vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit) => {
      if (url.startsWith('/api/episodes')) return response([{ number: 10, season: 1 }])
      if (options.method === 'PUT') return response({ message: '识别结果已更新，请重新加载' }, 409)
      return response(snapshot ? { ...snapshot, revision: 4 } : null)
    }))
    const state = await fixture()
    snapshot = JSON.parse(JSON.stringify(state.task.value))
    state.openOffset(); await settle()
    state.offsetEditor.target.value = 10; state.offsetEditor.selectAll(true)
    await state.offsetEditor.apply()
    expect(state.task.value!.revision).toBe(4)
    expect(state.task.value!.items[0]!.episodes).toEqual([1])
    expect(state.offsetEditor.stale.value).toBe(true)
    expect(state.offsetEditor.error.value).toContain('已更新')
    expect(state.offsetEditor.context.value).not.toBeNull()
    expect(state.notice.value).toBe('')
  })
})
