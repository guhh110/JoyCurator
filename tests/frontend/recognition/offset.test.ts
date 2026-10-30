import { createSSRApp, effectScope, h, nextTick, ref } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Episode, Recognition, RecognitionItem, Season } from '../../../frontend/src/api'
import { applyOffsetMappings, currentEpisodeOffset, offsetRows } from '../../../frontend/src/features/automatic/episodeOffset'
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
const cells = (html: string, label: string) => [...html.matchAll(new RegExp(`<div role="cell" data-label="${label}"[^>]*>([\\s\\S]*?)</div>`, 'g'))].map(match => match[1]!).join('')
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
  it.each([
    { original: [1], current: [10], expected: 9 },
    { original: [10, 11], current: [1, 2], expected: -9 },
    { original: [2, 1], current: [1, 2], expected: 0 },
    { original: [4, 2], current: [11, 13], expected: 9 },
    { original: [1, 2], current: [10, 12], expected: null },
    { original: [1, 2], current: [10], expected: null },
    { original: [], current: [10], expected: null },
    { original: [1], current: [], expected: null },
    { original: [1, 1], current: [10, 11], expected: null },
    { original: [1, 2], current: [10, 10], expected: null },
    { original: [1], current: [0], expected: -1 },
    { original: [0], current: [0], expected: 0 },
    { original: [0, 1], current: [9, 10], expected: 9 },
    { original: [1], current: [-1], expected: null },
    { original: [1], current: [1.5], expected: null },
    { original: [1], current: [Number.MAX_SAFE_INTEGER + 1], expected: null },
  ])('derives saved offset $expected from $original -> $current without changing metadata', ({ original, current, expected }) => {
    const item = file('saved', original, current)
    const before = JSON.stringify(item)
    expect(currentEpisodeOffset(item)).toBe(expected)
    expect(JSON.stringify(item)).toBe(before)
  })
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
    expect(offsetRows([files[0]!], files, new Set(['valid']), 1, 1, -2, episodes())[0]!.error).toContain('最小为 E0')
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
  it('sorts previews by original episode rather than filenames or saved offsets and does not jump on edits', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state, task } = fixture([
      file('/source/a/1.mkv', [10], [1]), file('/source/a/10.mkv', [2, 3], [20, 21]),
      file('/source/a/100.mkv', [0], [30]), file('/source/a/2.mkv', [1], [100]), file('/source/a/missing.mkv', []),
    ])
    await settle()
    const before = JSON.stringify(task.value)
    const expected = ['100.mkv', '2.mkv', '10.mkv', '1.mkv', 'missing.mkv']
    for (const offset of ['0', '9', '-1', '']) {
      state.offsetInput.value = offset
      expect(state.rows.value.map(row => row.file.path.split('/').pop())).toEqual(expected)
    }
    expect(JSON.stringify(task.value)).toBe(before)
  })
  it('uses one signed offset field and resets it when the dialog reopens', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response(episodes([0, 1, 10]))))
    const { state, task } = fixture([file()])
    await settle()
    expect(state.offsetInput.value).toBe('0')
    state.selectAll(true)
    for (const [input, expected] of [['+9', 9], ['9', 9], ['-1', -1], ['0', 0], [' +9 ', 9]] as const) {
      state.offsetInput.value = input
      expect(state.offset.value).toBe(expected)
      expect(state.chosen.value[0]!.episodes).toEqual([1 + expected])
      expect(state.canApply.value).toBe(true)
    }
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html.match(/type="text"/g)).toHaveLength(1)
    expect(html).toContain('aria-label="集数偏移"')
    expect(html).not.toContain('offset-original')
    expect(html).not.toContain('offset-target')
    expect(html).not.toContain('原始识别参照集号')
    expect(html).not.toContain('实际对应集号')
    state.offsetInput.value = '-'
    const invalid = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(invalid).toContain('aria-invalid="true"')
    expect(invalid).toContain('请输入整数偏移量')
    state.close()
    state.open(task.value!.items, seasons, 1, '/source/a')
    expect(state.offsetInput.value).toBe('0')
    expect(state.selected.value.size).toBe(1)
  })
  it('allows a negative offset down to E0, saves it and renders the saved negative badge on reopen', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response(episodes([0, 1, 2]))))
    const { state, task, commit } = fixture([file('/source/a/merged.mkv', [1, 2], [10, 11])])
    await settle()
    state.offsetInput.value = '-1'
    state.selectAll(true)
    expect(state.offset.value).toBe(-1)
    expect(state.chosen.value[0]!.episodes).toEqual([0, 1])
    expect(state.canApply.value).toBe(true)
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html.match(/id="offset-input"/g)).toHaveLength(1)
    expect(cells(html, '修正后')).toContain('S01E00 · E01')
    expect(cells(html, '修正后')).toContain('>-1 集</span>')
    await state.apply()
    const items = commit.mock.calls[0]![0]
    expect(items[0]).toMatchObject({ episodes: [0, 1], metadata: { episodes: [1, 2] } })
    task.value = { ...task.value!, revision: 3, items }
    state.open(items, seasons, 1, '/source/a')
    await settle()
    const reopened = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(cells(reopened, '当前映射')).toContain('S01E00 · E01')
    expect(cells(reopened, '当前映射')).toContain('>-1 集</span>')
    state.offsetInput.value = '-1'
    state.selectAll(true)
    expect(state.chosen.value[0]!.episodes).toEqual([0, 1])
  })
  it('accepts original E0 but rejects invalid offsets and results below E0', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes([0, 1, 2]))))
    const { state, commit } = fixture([file('/source/a/zero.mkv', [0])])
    await settle()
    state.offsetInput.value = '0'
    state.selectAll(true)
    expect(state.canApply.value).toBe(true)
    state.offsetInput.value = '1'
    expect(state.chosen.value[0]!.episodes).toEqual([1])
    expect(state.canApply.value).toBe(true)
    for (const input of ['-1', '-2', '', ' ', '0.5', '+', '-', '9x', '1e2', '0x10', '9007199254740992']) {
      state.offsetInput.value = input
      expect(state.canApply.value).toBe(false)
      await state.apply()
    }
    expect(commit).not.toHaveBeenCalled()
  })
  it('still checks TMDB membership and overlap acknowledgement for E0', async () => {
    const fetch = vi.fn().mockImplementationOnce(async () => response(episodes([1])))
      .mockImplementation(async () => response(episodes([0, 1])))
    vi.stubGlobal('fetch', fetch)
    const { state } = fixture([file(), file('/source/b/zero.mkv', [0])])
    await settle()
    state.offsetInput.value = '-1'
    state.selectAll(true)
    expect(state.chosen.value[0]!.error).toContain('TMDB')
    expect(state.canApply.value).toBe(false)
    state.retry()
    await settle()
    expect(state.chosen.value[0]!.error).toBe('')
    expect(state.overlaps.value).toBe(true)
    expect(state.canApply.value).toBe(false)
    state.allowOverlap.value = true
    expect(state.canApply.value).toBe(true)
  })
  it.each([
    { input: '+9', code: 'S01E19 · E20', badge: '+9 集' },
    { input: '-9', code: 'S01E01 · E02', badge: '-9 集' },
    { input: '0', code: 'S01E10 · E11', badge: '0 集' },
  ])('shows the $badge badge after the corrected mapping, not beside the inputs', async ({ input, code, badge }) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file('/source/a/merged.mkv', [10, 11], [19, 20]), file('/source/a/unselected.mkv', [3])])
    await settle()
    state.select('/source/a/unselected.mkv', false)
    state.offsetInput.value = input
    state.select('/source/a/merged.mkv', true)
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html).not.toContain('仅修改季度')
    expect(html).not.toContain('type="radio"')
    const corrected = cells(html, '修正后')
    expect(corrected.match(/class="offset-delta-badge/g)).toHaveLength(1)
    expect(corrected).toContain(`>${code}</span><span class="offset-delta-badge`)
    expect(corrected).toContain(`>${badge}</span>`)
    expect(html).toContain('相对原始识别偏移')
    expect(html).toContain('不修改')
    if (input === '0') expect(html).toContain('恢复原始集号')
  })
  it('does not show an offset badge for invalid or unselected results', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes([1]))))
    const { state } = fixture([file()])
    await settle()
    state.selectAll(false)
    const render = () => renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(cells(await render(), '修正后')).not.toContain('class="offset-delta-badge')
    state.selectAll(true)
    for (const input of ['9', '', '-1', '1.5']) {
      state.offsetInput.value = input
      expect(state.canApply.value).toBe(false)
      expect(cells(await render(), '修正后')).not.toContain('class="offset-delta-badge')
    }
  })
  it('shows saved offsets on unselected files independently of draft corrections', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file('/source/a/positive.mkv', [1], [10]), file('/source/a/negative.mkv', [10], [1]), file('/source/a/zero.mkv', [2], [2])])
    await settle()
    state.selectAll(false)
    const render = () => renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    const saved = cells(await render(), '当前映射')
    expect(saved.match(/class="offset-delta-badge/g)).toHaveLength(3)
    for (const badge of ['+9 集', '-9 集', '0 集']) expect(saved).toContain(`>${badge}</span>`)
    expect(saved).toContain('当前映射相对原始识别偏移')
    state.select('/source/a/positive.mkv', true)
    for (const input of ['10', '0', '']) {
      state.offsetInput.value = input
      const html = await render()
      expect(cells(html, '当前映射')).toBe(saved)
      if (input === '10') expect(cells(html, '修正后')).toContain('>+10 集</span>')
    }
  })
  it('does not invent a saved offset for nonuniform mappings, missing originals or unknown seasons', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file('/source/a/nonuniform.mkv', [1, 2], [10, 12]), file('/source/a/manual.mkv', [], [10]), { ...file(), season: -1 }])
    await settle()
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(cells(html, '当前映射')).not.toContain('class="offset-delta-badge')
    expect(cells(html, '当前映射')).toContain('S01E10 · E12')
    expect(cells(html, '当前映射')).toContain('季号未知')
  })
  it('shows only one loading indicator while changing target season', async () => {
    let finish!: (value: Response) => void
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(response(episodes()))
      .mockImplementationOnce(() => new Promise<Response>(resolve => { finish = resolve })))
    const { state } = fixture([file(), file('/source/a/02.mkv', [2])])
    await settle()
    state.selectAll(true)
    expect(state.canApply.value).toBe(true)
    state.season.value = 2
    expect(state.loading.value).toBe(true)
    expect(state.canApply.value).toBe(false)
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html.match(/校验中/g)).toHaveLength(1)
    expect(html).toMatch(/role="status"[^>]*>已勾选 2 \/ 2 · 校验中…/)
    expect(html).toContain('S02E01')
    expect(html).toContain('S02E02')
    finish(response(episodes()))
    await settle()
    expect(state.canApply.value).toBe(true)
    const ready = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(ready).not.toContain('校验中')
  })
  it('renders independent selection, three mapping columns and the explicit apply action', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file('/source/a/01.mkv', [1], [7])])
    await settle(); state.offsetInput.value = '9'; state.selectAll(true)
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    for (const text of ['原始识别', '当前映射', '修正后', 'S01E07', 'S01E10', '应用到 1 个文件', '仅保存映射，不执行文件整理', '/source/a/01.mkv'])
      expect(html).toContain(text)
    expect(html).toContain('aria-label="集数偏移"')
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
    expect(html).toMatch(/<strong[^>]*>01\.mkv<\/strong>[\s\S]*?class="eli-text-content"[^>]*data-ellipsis="start"[^>]*><bdi dir="ltr">\/source\/a\/nested\/01\.mkv<\/bdi>/)
    expect(html).not.toContain('offset-subfolder')
    expect(html).toContain('应用到 1 个文件')
    expect(html).not.toContain('应用到 0 个文件')
  })
  it('groups scope and correction controls without the persistent explanation and keeps input errors accessible', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture()
    await settle()
    const render = () => renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    const html = await render()
    expect(html).toMatch(/class="offset-setup"[^>]*>\s*<section class="offset-scope"/)
    expect(html).not.toContain('以原始识别为准')
    expect(html).not.toContain('offset-rule-note')
    expect(html).not.toContain('aria-describedby="offset-error"')
    state.offsetInput.value = '1.5'
    const invalid = await render()
    expect(invalid).toContain('aria-describedby="offset-error"')
    expect(invalid).toContain('id="offset-error"')
    expect(invalid).toContain('请输入整数偏移量')
  })
  it('uses the matched work poster snapshot and refreshes it when reopening', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const item = file()
    item.entry!.posterPath = '/show-poster.jpg'
    const { state, task } = fixture([item])
    await settle()
    expect(state.context.value?.posterPath).toBe('/show-poster.jpg')
    item.entry!.posterPath = '/updated-poster.jpg'
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html).toContain('src="https://image.tmdb.org/t/p/w154/show-poster.jpg"')
    expect(html).toContain('width="52" height="78"')
    expect(html).not.toContain('offset-poster-placeholder')
    state.close()
    state.open(task.value!.items, seasons, 1, '/source/a')
    expect(state.context.value?.posterPath).toBe('/updated-poster.jpg')
  })
  it('keeps a fixed poster placeholder when the matched work has no poster', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file()])
    await settle()
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html).toContain('offset-poster-placeholder')
    expect(html).not.toContain('image.tmdb.org')
  })
  it('defaults to all eligible files in the chosen source and saves only on explicit apply', async () => {
    const fetch = vi.fn().mockResolvedValue(response(episodes())); vi.stubGlobal('fetch', fetch)
    const { state, task, commit } = fixture()
    await settle()
    expect(state.selected.value.size).toBe(1)
    expect(state.canApply.value).toBe(true)
    expect(commit).not.toHaveBeenCalled()
    state.offsetInput.value = '9'
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
  it('replaces selection with the new source defaults and cancel makes no writes', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state, commit, task } = fixture()
    const before = JSON.stringify(task.value)
    await settle(); state.selectAll(false); state.allowOverlap.value = true; state.source.value = '/source/b'
    expect(state.chosen.value).toHaveLength(1)
    expect(state.allowOverlap.value).toBe(false)
    expect(state.chosen.value[0]!.file.path).toBe('/source/b/02.mkv')
    expect([...state.selected.value]).toEqual(['/source/b/02.mkv'])
    state.source.value = '/source/a'
    expect([...state.selected.value]).toEqual(['/source/a/01.mkv'])
    state.close()
    await state.apply()
    expect(commit).not.toHaveBeenCalled()
    expect(JSON.stringify(task.value)).toBe(before)
  })
  it('renders the entire source without pagination, skips invalid originals, and preserves deselections during revalidation', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response(episodes(Array.from({ length: 120 }, (_, i) => i)))))
    const files = Array.from({ length: 103 }, (_, i) => ({ ...file(`/source/a/${i}.mkv`, [i]), selected: i % 2 === 0 }))
    files.push(file('/source/a/missing.mkv', []), file('/source/a/negative.mkv', [-1]), file('/source/a/duplicate.mkv', [1, 1]))
    const { state, task, commit } = fixture(files)
    const before = JSON.stringify(task.value)
    expect(state.selected.value.size).toBe(103)
    expect(state.canApply.value).toBe(false)
    await settle()
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html).toContain('已勾选 103 / 106')
    expect(html.match(/class="offset-checkbox"/g)).toHaveLength(106)
    expect(html).toContain('修正 /source/a/102.mkv')
    for (const text of ['offset-pagination', '上一页', '下一页']) expect(html).not.toContain(text)
    for (const text of ['offset-selection-actions', '文件快捷选择', '全不选', '反选']) expect(html).not.toContain(text)
    expect(html).toMatch(/type="checkbox" checked[^>]*aria-label="选择本来源全部可修正文件"/)
    state.select('/source/a/0.mkv', false)
    state.offsetInput.value = '1'
    state.season.value = 2
    await settle()
    state.retry()
    await settle()
    expect(state.selected.value.size).toBe(102)
    expect(state.selected.value.has('/source/a/0.mkv')).toBe(false)
    expect(JSON.stringify(task.value)).toBe(before)
    expect(commit).not.toHaveBeenCalled()
    state.close()
    state.open(task.value!.items, seasons, 1, '/source/a')
    expect(state.selected.value.size).toBe(103)
    expect(state.selected.value.has('/source/a/0.mkv')).toBe(true)
  })
  it('selects only eligible files in the active source without modifying organizer selection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state, task, commit } = fixture([
      file('/source/a/01.mkv'), { ...file('/source/a/02.mkv', [2]), selected: true },
      file('/source/a/missing.mkv', []), file('/source/b/03.mkv', [3]),
    ])
    await settle()
    const before = JSON.stringify(task.value)
    state.allowOverlap.value = true
    state.selectAll(false)
    expect(state.allowOverlap.value).toBe(false)
    expect(state.selected.value.size).toBe(0)
    state.selectAll(true)
    expect([...state.selected.value]).toEqual(['/source/a/01.mkv', '/source/a/02.mkv'])
    state.select('/source/a/02.mkv', false)
    expect([...state.selected.value]).toEqual(['/source/a/01.mkv'])
    state.source.value = '/source/b'
    state.selectAll(false)
    expect(state.selected.value.size).toBe(0)
    state.selectAll(true)
    expect([...state.selected.value]).toEqual(['/source/b/03.mkv'])
    expect(JSON.stringify(task.value)).toBe(before)
    expect(commit).not.toHaveBeenCalled()
  })
  it.each(['locked', 'saving', 'stale'] as const)('blocks header and row selection while %s', async reason => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state, task, locked } = fixture([file(), file('/source/a/02.mkv', [2])])
    await settle()
    state.select('/source/a/02.mkv', false)
    if (reason === 'locked') locked.value = true
    if (reason === 'saving') state.saving.value = true
    if (reason === 'stale') task.value!.revision++
    state.select('/source/a/02.mkv', true)
    state.selectAll(false)
    state.selectAll(true)
    expect([...state.selected.value]).toEqual(['/source/a/01.mkv'])
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: locked.value }))
    expect(html).toMatch(/type="checkbox"[^>]*disabled[^>]*aria-label="选择本来源全部可修正文件"/)
    expect(html.match(/class="offset-checkbox"[^>]*disabled/g)).toHaveLength(2)
  })
  it('keeps an entirely ineligible source unselected and disables apply', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state } = fixture([file('/source/a/missing.mkv', [])])
    await settle()
    expect(state.selected.value.size).toBe(0)
    expect(state.canApply.value).toBe(false)
    state.selectAll(true)
    expect(state.selected.value.size).toBe(0)
    const html = await renderToString(createSSRApp(EpisodeOffsetDialog, { editor: state, locked: false }))
    expect(html).toMatch(/type="checkbox"[^>]*disabled[^>]*aria-label="选择本来源全部可修正文件"/)
  })
  it('supports negative offsets and season zero while leaving missing original numbers unselected', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response(episodes())))
    const items = [file('/source/a/10.mkv', [10]), file('/source/a/missing.mkv', [])]
    const { state } = fixture(items)
    await settle(); state.offsetInput.value = '-9'; state.season.value = 0
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
    await settle(); state.offsetInput.value = '9'; state.selectAll(true)
    expect(state.overlaps.value).toBe(true)
    expect(state.canApply.value).toBe(false)
    state.allowOverlap.value = true
    expect(state.canApply.value).toBe(true)
    state.offsetInput.value = '10'
    expect(state.allowOverlap.value).toBe(false)
  })
  it('blocks stale revisions, busy tasks and invalid targets', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(episodes())))
    const { state, task, commit, locked } = fixture()
    await settle(); state.selectAll(true)
    for (const value of ['', '-1', '1.5', '100']) {
      state.offsetInput.value = value
      expect(state.canApply.value).toBe(false)
      await state.apply()
    }
    state.offsetInput.value = '9'; locked.value = true
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
