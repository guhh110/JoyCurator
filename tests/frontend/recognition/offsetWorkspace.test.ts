import { createRenderer, h, nextTick } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAutomaticRecognition } from '../../../frontend/src/features/automatic/useAutomaticRecognition'
import type { BatchPreview, PreviewRow, Recognition, RecognitionItem } from '../../../frontend/src/api'
import { clearToasts, toasts } from '../../../frontend/src/app/toast'

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
afterEach(() => { cleanup.splice(0).forEach(fn => fn()); clearToasts(); vi.unstubAllGlobals(); vi.clearAllMocks() })
const response = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status })
async function settle() { for (let i = 0; i < 15; i++) await Promise.resolve(); await nextTick() }
async function fixture() {
  mocks.fetchQuery.mockResolvedValue([{ number: 1, name: '第一季', count: 20 }, { number: 2, name: '第二季', count: 12 }])
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
  it('rebuilds duplicate policy previews and blocks confirmation after a failed policy refresh or all-skipped result', async () => {
    let fail = false, skipAll = false
    const fetch = vi.fn(async (url: string, options: RequestInit) => {
      if (url === '/api/batches/preview') {
        if (fail) return response({ message: 'preview failed' }, 500)
        const input = JSON.parse(options.body as string)
        return response({ id: `plan-${input.duplicatePolicy}`, duplicatePolicy: input.duplicatePolicy, mode: input.mode, groups: [], issues: [], rows: [
          { source: '/source/a.mkv', target: '/target/a.mkv', season: 1, episodes: [1], artifacts: [], issues: [], action: skipAll ? 'skip' : 'overwrite' },
        ], required: 1 })
      }
      return response(null)
    })
    vi.stubGlobal('fetch', fetch)
    const state = await fixture()
    expect(state.duplicatePolicy.value).toBe('skip')
    await state.prepare()
    expect(state.preview.value?.duplicatePolicy).toBe('skip')
    fail = true
    await state.changeDuplicatePolicy('overwrite')
    expect(state.preview.value?.duplicatePolicy).toBe('skip')
    expect(state.previewReady.value).toBe(false)
    await state.execute()
    expect(fetch.mock.calls.some(([url]) => url === '/api/batches/execute')).toBe(false)
    fail = false
    await state.prepare()
    expect(state.preview.value?.duplicatePolicy).toBe('overwrite')
    expect(state.previewReady.value).toBe(true)
    skipAll = true
    await state.changeDuplicatePolicy('skip')
    expect(state.previewReady.value).toBe(false)
    expect(fetch.mock.calls.filter(([url]) => url === '/api/batches/preview').map(([, options]) => JSON.parse(options.body as string).duplicatePolicy)).toEqual(['skip', 'overwrite', 'overwrite', 'skip'])
    state.duplicatePolicy.value = 'overwrite'
    state.preview.value = null
    await state.prepare()
    expect(state.duplicatePolicy.value).toBe('skip')
    expect(state.preview.value?.duplicatePolicy).toBe('skip')
  })
  it('sorts workspace files and preview rows before pagination while preserving work groups and stored order', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => response(null)))
    const state = await fixture()
    const base = state.task.value!.items[0]!
    state.task.value!.items = Array.from({ length: 103 }, (_, i) => ({ ...base, path: `/source/${103 - i}.mkv`, episodes: [103 - i] }))
    expect(state.visibleFiles.value.map(item => item.episodes[0])).toEqual(Array.from({ length: 40 }, (_, i) => i + 1))
    const row = (source: string, episode: number): PreviewRow => ({ source, target: `/target/${source}`, season: 1, episodes: [episode], artifacts: [], issues: [], action: 'create' })
    const first = Array.from({ length: 32 }, (_, i) => row(`z/${32 - i}.mkv`, 32 - i))
    const second = [row('a/10.mkv', 10), row('a/2.mkv', 2)]
    state.preview.value = { id: 'preview', title: '', mode: 'copy', duplicatePolicy: 'skip', rows: [...first, ...second], required: 0, issues: [], groups: [
      { id: 'first', title: '', mode: 'copy', duplicatePolicy: 'skip', rows: first, required: 0, issues: [], type: 'tv', tmdbId: 1 },
      { id: 'second', title: '', mode: 'copy', duplicatePolicy: 'skip', rows: second, required: 0, issues: [], type: 'tv', tmdbId: 2 },
    ] }
    const before = JSON.stringify(state.preview.value)
    expect(state.previewRows.value.map(row => row.source)).toEqual(Array.from({ length: 30 }, (_, i) => `z/${i + 1}.mkv`))
    state.previewPage.value = 2
    expect(state.previewRows.value.map(row => row.source)).toEqual(['z/31.mkv', 'z/32.mkv', 'a/2.mkv', 'a/10.mkv'])
    expect(JSON.stringify(state.preview.value)).toBe(before)
    expect(state.task.value!.items[0]!.episodes).toEqual([103])
  })
  it('switches comparison season without saving, changing file mappings or invalidating preview, even while locked', async () => {
    const fetch = vi.fn(async (url: string, _options: RequestInit) => {
      if (url.startsWith('/api/episodes')) return response([{ number: 1, season: Number(new URL(url, 'http://localhost').searchParams.get('season')) }])
      return response(null)
    })
    vi.stubGlobal('fetch', fetch)
    const state = await fixture()
    state.preview.value = { id: 'keep-preview' } as BatchPreview
    state.detailView.value = 'episodes'
    const before = JSON.stringify(state.task.value)
    state.viewSeason(2)
    await vi.waitFor(() => expect(state.episodeQuery.data.value?.[0]?.season).toBe(2))
    expect(state.comparisonSeason.value).toBe(2)
    state.viewSeason(999)
    expect(state.comparisonSeason.value).toBe(2)
    expect(JSON.stringify(state.task.value)).toBe(before)
    expect(state.preview.value?.id).toBe('keep-preview')
    expect(mocks.setQueryData).not.toHaveBeenCalled()
    expect(toasts.value).toEqual([])

    state.task.value!.state = 'running'
    expect(state.locked.value).toBe(true)
    state.viewSeason(1)
    await settle()
    expect(state.comparisonSeason.value).toBe(1)
    expect(state.task.value!.revision).toBe(3)
    expect(state.task.value!.items[0]).toMatchObject({ season: 1, episodes: [1], selected: false })
    expect(state.preview.value?.id).toBe('keep-preview')
    expect(fetch.mock.calls.every(([, options]) => options.method === 'GET')).toBe(true)
  })
  it('reopens after each save and replaces the absolute offset without accumulating it', async () => {
    let stored: Recognition | null = null
    const submitted: number[][][] = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit) => {
      if (url.startsWith('/api/episodes'))
        return response(Array.from({ length: 30 }, (_, index) => ({ number: index + 1, season: 1 })))
      if (options.method === 'PUT') {
        const update = JSON.parse(options.body as string)
        expect(update.revision).toBe(stored!.revision)
        submitted.push(update.items.map((item: RecognitionItem) => item.episodes))
        stored = {
          ...stored!, revision: stored!.revision + 1,
          items: stored!.items.map((item, index) => ({
            ...item, season: update.items[index].season, episodes: update.items[index].episodes, state: 'ready',
          })),
        }
      }
      return response(stored)
    }))
    const state = await fixture()
    const first = state.task.value!.items[0]!
    state.task.value!.items.push({
      ...first, path: '/source/merged.mkv', episodes: [2, 3],
      metadata: { ...first.metadata, episodes: [2, 3] },
    })
    stored = JSON.parse(JSON.stringify(state.task.value))

    // Repeating +9 is idempotent; changing to +10 replaces it; zero restores
    // the original numbers. A manual edit must not replace the original base.
    for (const [input, expected] of [['+9', [[10], [11, 12]]], ['9', [[10], [11, 12]]], ['10', [[11], [12, 13]]], ['0', [[1], [2, 3]]]] as const) {
      state.openOffset()
      await settle()
      const editor = state.offsetEditor
      expect(editor.rows.value.map(row => row.file.metadata.episodes)).toEqual([[1], [2, 3]])
      editor.offsetInput.value = input
      editor.selectAll(true)
      expect(editor.rows.value.map(row => row.episodes)).toEqual(expected)
      expect(editor.canApply.value).toBe(true)
      await editor.apply()
      expect(editor.context.value).toBeNull()
      expect(state.task.value!.items.map(item => item.episodes)).toEqual(expected)
      expect(state.task.value!.items.map(item => item.metadata.episodes)).toEqual([[1], [2, 3]])
      // Simulate a fresh server snapshot rather than retaining object references.
      state.task.value = JSON.parse(JSON.stringify(stored))
    }
    expect(submitted).toEqual([[[10], [11, 12]], [[10], [11, 12]], [[11], [12, 13]], [[1], [2, 3]]])

    state.task.value!.items[0]!.episodes = [20]
    state.openOffset()
    await settle()
    state.offsetEditor.offsetInput.value = '9'
    state.offsetEditor.selectAll(true)
    expect(state.offsetEditor.rows.value.map(row => row.episodes)).toEqual([[10], [11, 12]])
  })
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
    state.offsetEditor.offsetInput.value = '9'; state.offsetEditor.selectAll(true)
    await state.offsetEditor.apply()
    expect(posted).toMatchObject({ revision: 3, items: [{ path: '/source/a.mkv', episodes: [10], selected: false }] })
    expect(posted.items[0]).not.toHaveProperty('metadata')
    expect(state.task.value!.revision).toBe(4)
    expect(state.task.value!.items[0]!.metadata.episodes).toEqual([1])
    expect(state.preview.value).toBeNull()
    expect(state.comparisonSeason.value).toBe(1)
    expect(toasts.value).toHaveLength(1)
    expect(toasts.value[0]).toMatchObject({ tone: 'success', message: '季集映射已保存，请检查文件校验状态后重新生成整理预览' })
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
    state.offsetEditor.offsetInput.value = '9'; state.offsetEditor.selectAll(true)
    await state.offsetEditor.apply()
    expect(state.task.value!.revision).toBe(4)
    expect(state.task.value!.items[0]!.episodes).toEqual([1])
    expect(state.offsetEditor.stale.value).toBe(true)
    expect(state.offsetEditor.error.value).toContain('已更新')
    expect(state.offsetEditor.context.value).not.toBeNull()
    expect(state.notice.value).toBe('')
    expect(toasts.value).toHaveLength(1)
    expect(toasts.value[0]!.tone).toBe('error')
  })
})
