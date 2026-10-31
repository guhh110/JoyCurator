import { describe, expect, it } from 'vitest'
import type { MediaMapping, ScrapePreview } from '../../../frontend/src/api'
import { addSelection, artifactSummary, hasMultipleVersions, visibleEpisodes } from '../../../frontend/src/features/scrape/selection'

const item = (path: string, state: MediaMapping['state'], season = 1, episodes = [1]): MediaMapping =>
  ({ path, source: path, state, type: 'tv', tmdbId: 2, season, episodes })

describe('scrape selection', () => {
  it('selects across collapsed seasons regardless of view filter and requires explicit completed mode', () => {
    const items = [item('S01.E01.a.mkv', 'pending'), item('S01.E01.b.mkv', 'failed'),
      item('S01.E01.c.mkv', 'completed'), item('S02.E01.mkv', 'pending', 2), item('S02.missing.mkv', 'unavailable', 2)]
    expect(visibleEpisodes(items, 'failed', '').length).toBe(1)
    expect(addSelection([], items, false)).toEqual(['S01.E01.a.mkv', 'S01.E01.b.mkv', 'S02.E01.mkv'])
    expect(addSelection([], items, true)).toEqual(['S01.E01.c.mkv'])
    expect(visibleEpisodes(items, 'all', 'S02')[0]?.episodes[0]?.items.length).toBe(2)
  })

  it('never truncates an oversized batch', () => {
    const items = Array.from({ length: 1001 }, (_, i) => item(`${i}.mkv`, 'pending'))
    expect(addSelection([], items, false)).toBeNull()
    expect(addSelection([], items.slice(0, 1000), false)?.length).toBe(1000)
  })

  it('keeps a multi-version episode grouped when filters show only one version', () => {
    const items = [item('S01.E01.a.mkv', 'pending'), item('S01.E01.b.mkv', 'completed'),
      item('S01.E02.mkv', 'pending', 1, [2])]
    expect(visibleEpisodes(items, 'pending', '')[0]?.episodes.map(episode => episode.items.length)).toEqual([1, 1])
    expect(hasMultipleVersions(items, 1, '1')).toBe(true)
    expect(hasMultipleVersions(items, 1, '2')).toBe(false)
  })

  it('counts shared artwork once and per-version products separately', () => {
    const preview = { artifacts: ['/show/tvshow.nfo', '/show/poster.jpg', '/show/Season 01/season.nfo',
      '/show/Season 01/a.nfo', '/show/Season 01/a.jpg', '/show/Season 01/b.nfo'] } as ScrapePreview
    expect(artifactSummary(preview)).toEqual({ videoNfo: 2, sharedNfo: 2, videoImages: 1, sharedImages: 1 })
  })
})
