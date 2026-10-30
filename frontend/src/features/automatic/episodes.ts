import type { Episode, RecognitionItem } from '../../api'

export function episodeComparison(episodes: Episode[], files: RecognitionItem[], season: number) {
  return episodes.map(episode => ({
    episode,
    files: files.filter(file => !file.removed && file.season === season && file.episodes.includes(episode.number)),
  }))
}
