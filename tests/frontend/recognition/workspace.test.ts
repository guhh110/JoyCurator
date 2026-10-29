import { describe, expect, it } from 'vitest'
import type { RecognitionItem } from '../../../frontend/src/api'
import { recognitionEdits, recognitionGroups } from '../../../frontend/src/recognition'
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
    probe: { resolution: '', codec: '' },
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
