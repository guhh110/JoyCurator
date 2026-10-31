import { describe, expect, it } from 'vitest'
import type { RecognitionItem } from '../../../frontend/src/api'
import { recognitionEdits, recognitionGroups, sortByEpisode } from '../../../frontend/src/recognition'
function file(
  path: string,
  group: string,
  state: RecognitionItem['state'],
): RecognitionItem {
  return {
    path,
    group,
    state,
    size: 32,
    mtime: '1234567890123456789',
    season: -1,
    episodes: [],
    metadata: {
      releaseTag: null,
      title: '片名',
      year: 2024,
      mediaType: null,
      tmdbId: null,
      season: null,
      episodes: [],
    },
    entry: null,
    candidates: [],
    selected: false,
    removed: false,
    reason: '',
  }
}
describe('recognition workspace', () => {
  it('sorts season and episode numbers numerically, including E0 and merged files, without mutating inputs', () => {
    const items = [
      { path: '100.mkv', season: 1, episodes: [100] },
      { path: 'unknown.mkv', season: -1, episodes: [] },
      { path: '10.mkv', season: 1, episodes: [10] },
      { path: 'merged.mkv', season: 1, episodes: [4, 2] },
      { path: 'version10.mkv', season: 1, episodes: [2] },
      { path: 'version2.mkv', season: 1, episodes: [2] },
      { path: 'zero.mkv', season: 1, episodes: [0] },
      { path: 'special.mkv', season: 0, episodes: [5] },
      { path: 'season2.mkv', season: 2, episodes: [1] },
      { path: 'missing.mkv', season: 1, episodes: [] },
    ]
    const before = JSON.stringify(items)
    expect(sortByEpisode(items, item => item).map(item => item.path)).toEqual([
      'special.mkv', 'zero.mkv', 'version2.mkv', 'version10.mkv', 'merged.mkv', '10.mkv', '100.mkv', 'missing.mkv', 'season2.mkv', 'unknown.mkv',
    ])
    expect(JSON.stringify(items)).toBe(before)
    const files = items.map(item => ({ ...file(item.path, 'tv:1', 'ready'), season: item.season, episodes: item.episodes }))
    expect(recognitionGroups(files)[0]!.files.map(item => item.path)).toEqual(sortByEpisode(items, item => item).map(item => item.path))
  })
  it('groups mixed works and omits removed files without losing individual states', () => {
    const a = file('/a.mkv', 'movie:1', 'ready'),
      b = file('/b.mkv', 'tv:2', 'review'),
      c = file('/c.mkv', 'tv:2', 'ready'),
      removed = file('/d.mkv', 'movie:3', 'ready')
    removed.removed = true
    const groups = recognitionGroups([a, b, c, removed])
    expect(groups).toHaveLength(2)
    expect(groups[1]!.files).toHaveLength(2)
    expect(groups[1]!.state).toBe('review')
    expect(groups[1]!.files[1]!.state).toBe('ready')
  })
  it('uses the assigned work title and preserves edits in the save payload', () => {
    const a = file('/episode.mkv', 'custom', 'ready')
    a.entry = {
      tmdbId: 2,
      type: 'tv',
      title: '剧名',
      originalTitle: 'Show',
      year: 2020,
      overview: '',
      posterPath: '',
    }
    a.season = 0
    a.episodes = [1, 3]
    a.selected = true
    expect(recognitionGroups([a])[0]!.title).toBe('剧名')
    expect(recognitionEdits([a])).toEqual([
      {
        path: '/episode.mkv',
        group: 'custom',
        type: 'tv',
        tmdbId: 2,
        season: 0,
        episodes: [1, 3],
        selected: true,
        removed: false,
      },
    ])
    expect(recognitionEdits([a])[0]).not.toHaveProperty('mtime')
    expect(recognitionEdits([a])[0]).not.toHaveProperty('size')
  })
})
