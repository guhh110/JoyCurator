import type { MediaMapping, ScrapePreview } from '../../api'

export type ScrapeStateFilter = 'all' | MediaMapping['state']

export function bulkCandidates(items: MediaMapping[], reScrape: boolean) {
  return items.filter(item => item.state !== 'unavailable' &&
    (reScrape ? item.state === 'completed' : item.state === 'pending' || item.state === 'failed'))
}

export function addSelection(existing: string[], items: MediaMapping[], reScrape: boolean) {
  const next = [...new Set([...existing, ...bulkCandidates(items, reScrape).map(item => item.path)])]
  return next.length <= 1000 ? next : null
}

export function hasMultipleVersions(items: MediaMapping[], season: number, key: string) {
  return items.filter(item => item.season === season && item.episodes.join(',') === key).length > 1
}

export function visibleEpisodes(items: MediaMapping[], state: ScrapeStateFilter, search: string) {
  const query = search.trim().toLocaleLowerCase()
  const seasons = new Map<number, Map<string, MediaMapping[]>>()
  for (const item of items) {
    if (state !== 'all' && item.state !== state) continue
    const label = item.episodes.map(n => `E${String(n).padStart(2, '0')}`).join(' ')
    if (query && !item.path.toLocaleLowerCase().includes(query) && !label.toLocaleLowerCase().includes(query) &&
      !`S${String(item.season).padStart(2, '0')}`.toLocaleLowerCase().includes(query)) continue
    const episodes = seasons.get(item.season) ?? new Map<string, MediaMapping[]>()
    const key = item.episodes.join(',')
    episodes.set(key, [...(episodes.get(key) ?? []), item])
    seasons.set(item.season, episodes)
  }
  return [...seasons.entries()].sort(([a], [b]) => a - b).map(([number, episodes]) => ({
    number,
    episodes: [...episodes.entries()].sort(([a], [b]) => Number(a.split(',')[0]) - Number(b.split(',')[0]))
      .map(([key, versions]) => ({ key, items: versions.sort((a, b) => a.path.localeCompare(b.path, undefined, { numeric: true })) })),
  }))
}

export function artifactSummary(preview: ScrapePreview) {
  const summary = { videoNfo: 0, sharedNfo: 0, videoImages: 0, sharedImages: 0 }
  for (const path of preview.artifacts) {
    const name = path.split(/[\\/]/).pop()?.toLowerCase() ?? ''
    const shared = ['tvshow.nfo', 'season.nfo', 'poster.jpg', 'fanart.jpg', 'clearlogo.jpg', 'clearlogo.png'].includes(name) ||
      /^season\d+-poster\.(jpg|jpeg|png)$/.test(name)
    if (name.endsWith('.nfo')) summary[shared ? 'sharedNfo' : 'videoNfo']++
    else summary[shared ? 'sharedImages' : 'videoImages']++
  }
  return summary
}
