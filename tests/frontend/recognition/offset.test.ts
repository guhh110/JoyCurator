import { createSSRApp, effectScope, h, nextTick, ref } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Episode, Recognition, RecognitionItem, Season } from '../../../frontend/src/api'
import { applyOffsetMappings, offsetRows } from '../../../frontend/src/features/automatic/episodeOffset'
import { useEpisodeOffset } from '../../../frontend/src/features/automatic/useEpisodeOffset'
import EpisodeOffsetDialog from '../../../frontend/src/features/automatic/EpisodeOffsetDialog.vue'

vi.mock('../../../frontend/src/components/Modal.vue', () => ({ default: { setup: (_props: unknown, { slots }: any) => () => h('section', [slots.default?.(), slots.footer?.()]) } }))
vi.mock('../../../frontend/src/components/ui/Select.vue', () => ({ default: { props: ['label'], setup: (props: { label: string }) => () => h('span', props.label) } }))

const seasons: Season[] = [{ number: 0, name: '特别篇', count: 12 }, { number: 1, name: '第一季', count: 20 }, { number: 2, name: '第二季', count: 12 }]
const episodes = (numbers = Array.from({ length: 20 }, (_, i) => i + 1)): Episode[] => numbers.map(number => ({ number, season: 1, name: '', airDate: '' }))
function file(path = '/source/a/01.mkv', original = [1], current = original): RecognitionItem {
  return { path, group: 'tv:1', state: 'review', size: 32, mtime: '1', season: 1, episodes: [...current], probe: { resolution: '', codec: '' },
    metadata: { title: 'Show', year: null, mediaType: 'tv', tmdbId: 1, season: 1, episodes: [...original], releaseTag: null },
    entry: { tmdbId: 1, type: 'tv', title: 'Show', originalTitle: '', year: 2024, overview: '', posterPath: '' },
    candidates: [], selected: false, removed: false, reason: '' }
}
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.unstubAllGlobals() })
const response = (data: unknown) => new Response(JSON.stringify(data), { status: 200 })
async function settle() { for (let i = 0; i < 15; i++) await Promise.resolve(); await nextTick() }
function fixture(items = [file(), file('/source/b/02.mkv', [2])]) {
  const task = ref<Recognition | null>({ id: 'workspace', revision: 2, state: 'completed', paths: ['/source/a', '/source/b'], items, completed: items.length, total: items.length, error: '', created: '' })
  const locked = ref(false), saveError = ref('')
  const commit = vi.fn(async (_items: RecognitionItem[], _id: string, _revision: number) => true)
  const scope = effectScope(); scopes.push(scope)
  const state = scope.run(() => useEpisodeOffset({ task, locked, commit, saveError }))!
  state.open(items, seasons, 1, '/source/a')
  return { state, task, locked, commit, scope, saveError }
}

describe('episode offset mapping', () => {
  it('offsets raw recognition rather than current edits and preserves merged episodes', () => {
    const files = [file('/source/01.mkv', [1], [10]), file('/source/merged.mkv', [2, 4], [11, 13])]
    const before = JSON.stringify(files)
    const result = offsetRows(files, files, new Set(files.map(item => item.path)), 1, 1, 9, episodes())
    expect(result.map(row => row.episodes)).toEqual([[10], [11, 13]])
    expect(JSON.stringify(files)).toBe(before)
    expect(offsetRows(files, files, new Set(), 1, 0, -1, episodes()).map(row => row.episodes)).toEqual([[0], [1, 3]])
  })
  it('validates exact episode membership instead of count and rejects invalid inputs', () => {
    const files = [file('valid', [1]), file('missing', [2]), file('empty', []), file('duplicate', [1, 1])]
    const result = offsetRows(files, files, new Set(files.map(item => item.path)), 1, 1, 9, episodes([10, 12]))
    expect(result.map(row => !!row.error)).toEqual([false, true, true, true])
    expect(offsetRows([files[0]!], files, new Set(['valid']), 1, 1, -1, episodes())[0]!.error).toContain('正整数')
    for (const offset of [NaN, 1.5, Number.MAX_SAFE_INTEGER])
      expect(offsetRows([files[0]!], files, new Set(['valid']), 1, 1, offset, episodes())[0]!.error).not.toBe('')
  })
  it('reports overlaps outside the source but ignores removed files, other works and seasons', () => {
    const changing = file('changing')
    const conflict = file('other-source', [10])
    const removed = { ...file('removed', [10]), removed: true }
    const otherSeason = { ...file('season', [10]), season: 2 }
    const otherWork = file('work', [10]); otherWork.entry!.tmdbId = 2
    const all = [changing, conflict, removed, otherSeason, otherWork]
    expect(offsetRows([changing], all, new Set(['changing']), 1, 1, 9, episodes())[0]!.overlaps).toEqual(['other-source'])
    expect(offsetRows([changing, conflict], all, new Set(['changing', 'other-source']), 1, 1, 9, episodes())[0]!.overlaps).toEqual([])
  })
  it('copies only selected mappings, preserves organizer selections and leaves raw metadata intact', () => {
    const first = file(), second = file('other', [2]); second.selected = true
    const result = applyOffsetMappings([first, second], [{ path: first.path, season: 0, episodes: [10] }])
    expect(result[0]).toMatchObject({ season: 0, episodes: [10], selected: false, metadata: { episodes: [1] } })
    expect(result[1]).toBe(second)
    expect(first.episodes).toEqual([1])
  })
})

describe('episode offset dialog state', () => {
  it('renders independent selection, three mapping columns and the explicit apply action', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file('/source/a/01.mkv', [1], [7])])
    await settle(); state.target.value = 10; state.selectAll(true)
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    for (const text of ['原始识别', '当前映射', '修正后', 'S01E07', 'S01E10', '应用到 1 个文件', '仅保存映射，不执行文件整理', '/source/a/01.mkv'])
      expect(html).toContain(text)
    expect(html).toContain('aria-label="实际对应集号"')
    expect(html).toContain('role="table"')
    expect(html).not.toContain('offset-source-picker')
    expect(html).toContain('查看路径')
    expect(html).toContain('eli-text offset-file-path')
    expect(html).toContain('eli-text-content')
    expect(html).not.toContain('offset-file-detail')
    expect(html).not.toContain('点击文件名查看路径')
  })
  it('shows a source picker only for multiple sources and renders the full path under the filename using EliText', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file('/source/a/nested/01.mkv'), file('/source/b/01.mkv')])
    await settle()
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html).toContain('offset-source-picker')
    expect(html).toMatch(/<strong[^>]*>01\.mkv<\/strong>[\s\S]*?class="eli-text-content"[^>]*>\/source\/a\/nested\/01\.mkv<\/span>/)
    expect(html).not.toContain('offset-subfolder')
    expect(html).toContain('应用修正')
    expect(html).not.toContain('应用到 0 个文件')
  })
  it('requires independent explicit selection and saves only the chosen source without executing', async () => {
    const fetch = vi.fn().mockResolvedValue(response(episodes())); vi.stubGlobal('fetch', fetch)
    const { state, task, commit } = fixture()
    await settle()
    expect(state.selected.value.size).toBe(0)
    expect(state.canApply.value).toBe(false)
    state.target.value = 10
    state.selectAll(true)
    expect(state.chosen.value).toHaveLength(1)
    expect(state.canApply.value).toBe(true)
    const before = JSON.stringify(task.value)
    await state.apply()
    expect(commit).toHaveBeenCalledOnce()
    const [items, id, revision] = commit.mock.calls[0]!
    expect([id, revision]).toEqual(['workspace', 2])
    expect(items.map(item => item.episodes)).toEqual([[10], [2]])
    expect(items.every(item => !item.selected)).toBe(true)
    expect(JSON.stringify(task.value)).toBe(before)
    expect(state.context.value).toBeNull()
    expect(fetch.mock.calls.every(([url]) => url.startsWith('/api/episodes?'))).toBe(true)
  })
  it('clears selected files on source changes and cancel makes no writes', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state, commit, task } = fixture()
    const before = JSON.stringify(task.value)
    await settle(); state.selectAll(true); state.source.value = '/source/b'
    expect(state.chosen.value).toHaveLength(0)
    state.selectAll(true)
    expect(state.chosen.value[0]!.file.path).toBe('/source/b/02.mkv')
    state.close()
    await state.apply()
    expect(commit).not.toHaveBeenCalled()
    expect(JSON.stringify(task.value)).toBe(before)
  })
  it('supports negative offsets and season zero while leaving missing original numbers unselected', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response(episodes())))
    const items = [file('/source/a/10.mkv', [10]), file('/source/a/missing.mkv', [])]
    const { state } = fixture(items)
    await settle(); state.original.value = 10; state.target.value = 1; state.season.value = 0
    expect(state.canApply.value).toBe(false)
    await settle(); state.selectAll(true)
    expect(state.offset.value).toBe(-9)
    expect(state.chosen.value).toHaveLength(1)
    expect(state.chosen.value[0]!.episodes).toEqual([1])
    expect(state.canApply.value).toBe(true)
  })
  it('requires overlap acknowledgement and resets it on every mapping change', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response(episodes())))
    const { state } = fixture([file(), file('/source/b/10.mkv', [10])])
    await settle(); state.target.value = 10; state.selectAll(true)
    expect(state.overlaps.value).toBe(true)
    expect(state.canApply.value).toBe(false)
    state.allowOverlap.value = true
    expect(state.canApply.value).toBe(true)
    state.target.value = 11
    expect(state.allowOverlap.value).toBe(false)
  })
  it('blocks stale revisions, busy tasks and invalid targets', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state, task, commit, locked } = fixture()
    await settle(); state.selectAll(true)
    for (const value of ['', 0, 1.5, 100]) {
      state.target.value = value
      expect(state.canApply.value).toBe(false)
      await state.apply()
    }
    state.target.value = 10; locked.value = true
    await state.apply(); locked.value = false
    task.value!.revision++
    expect(state.stale.value).toBe(true)
    await state.apply()
    expect(commit).not.toHaveBeenCalled()
  })
  it('discards old episode responses on season change and aborts on disposal', async () => {
    const requests: { resolve: (response: Response) => void; signal: AbortSignal }[] = []
    vi.stubGlobal('fetch', vi.fn((_url, options) => new Promise<Response>(resolve => requests.push({ resolve, signal: options.signal }))))
    const { state, scope } = fixture()
    state.season.value = 2
    expect(requests[0]!.signal.aborted).toBe(true)
    requests[1]!.resolve(response(episodes([10])))
    await settle()
    requests[0]!.resolve(response(episodes([1])))
    await settle()
    expect(state.episodes.value?.map(item => item.number)).toEqual([10])
    state.season.value = 1
    scope.stop()
    expect(requests[2]!.signal.aborted).toBe(true)
    requests[2]!.resolve(response(episodes([5])))
    await settle()
    expect(state.context.value).toBeNull()
    expect(state.episodes.value).toBeNull()
  })
  it('keeps the draft on validation and save failures, and prevents duplicate submission', async () => {
    const fetch = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(response(episodes()))
    vi.stubGlobal('fetch', fetch)
    const { state, commit, saveError } = fixture()
    await settle(); state.selectAll(true)
    expect(state.error.value).toBe('offline')
    expect(state.canApply.value).toBe(false)
    state.retry(); await settle()
    let finish!: (value: boolean) => void
    commit.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const pending = state.apply()
    await state.apply()
    expect(commit).toHaveBeenCalledOnce()
    saveError.value = 'revision rejected'; finish(false); await pending
    expect(state.context.value).not.toBeNull()
    expect(state.error.value).toBe('revision rejected')
    expect(state.saving.value).toBe(false)
  })
})
