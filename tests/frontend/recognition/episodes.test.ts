import { describe, expect, it } from 'vitest'
import type { Episode, RecognitionItem } from '../../../frontend/src/api'
import { episodeComparison } from '../../../frontend/src/features/automatic/episodes'

const episodes = [1, 2, 3].map(number => ({ number, season: 1, name: `第 ${number} 集`, airDate: '' })) satisfies Episode[]
function file(path: string, season: number, numbers: number[], removed = false) {
  return { path, season, episodes: numbers, removed } as RecognitionItem
}
describe('workspace episode comparison', () => {
  it('orders unsorted episode responses numerically without modifying the response', () => {
    const input = [10, 2, 0, 1].map(number => ({ number, season: 1, name: '', airDate: '' }))
    expect(episodeComparison(input, [], 1).map(row => row.episode.number)).toEqual([0, 1, 2, 10])
    expect(input.map(episode => episode.number)).toEqual([10, 2, 0, 1])
  })
  it('maps merged episodes and multiple versions without hiding missing episodes', () => {
    const merged = file('merged.mkv', 1, [1, 2])
    const version = file('version.mkv', 1, [1])
    const rows = episodeComparison(episodes, [merged, version], 1)
    expect(rows.map(row => row.files.map(item => item.path))).toEqual([
      ['merged.mkv', 'version.mkv'], ['merged.mkv'], [],
    ])
  })
  it('does not match other seasons, unknown seasons or removed files', () => {
    const files = [file('other.mkv', 2, [1]), file('unknown.mkv', -1, [2]), file('removed.mkv', 1, [3], true)]
    expect(episodeComparison(episodes, files, 1).every(row => !row.files.length)).toBe(true)
    expect(files.map(item => item.season)).toEqual([2, -1, 1])
  })
  it('matches specials using season zero', () => {
    const special = file('special.mkv', 0, [1])
    expect(episodeComparison(episodes.slice(0, 1), [special], 0)[0]!.files).toEqual([special])
  })
})
