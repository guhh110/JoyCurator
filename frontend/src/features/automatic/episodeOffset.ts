import type { Episode, RecognitionItem } from '../../api'

export interface OffsetMapping {
  path: string
  season: number
  episodes: number[]
}

export function originalEpisodesValid(file: RecognitionItem) {
  return episodesValid(file.metadata.episodes)
}

function episodesValid(episodes: number[]) {
  return episodes.length > 0 && episodes.length <= 1000 &&
    episodes.every(number => Number.isSafeInteger(number) && number >= 0) &&
    new Set(episodes).size === episodes.length
}

export function currentEpisodeOffset(file: RecognitionItem): number | null {
  if (!originalEpisodesValid(file) || !episodesValid(file.episodes) || file.episodes.length !== file.metadata.episodes.length) return null
  // Episode order is not meaningful; only a uniform shift has a single offset.
  const original = [...file.metadata.episodes].sort((a, b) => a - b)
  const current = [...file.episodes].sort((a, b) => a - b)
  const offset = current[0]! - original[0]!
  return current.every((number, index) => number - original[index]! === offset) ? offset : null
}

export function offsetRows(
  files: RecognitionItem[],
  allFiles: RecognitionItem[],
  selected: Set<string>,
  tmdbId: number,
  season: number,
  offset: number,
  episodes: Episode[] | null,
) {
  const allowed = new Set(episodes?.map(episode => episode.number))
  const rows = files.map(file => {
    const eligible = originalEpisodesValid(file)
    const mapped = file.metadata.episodes.map(number => number + offset)
    let error = ''
    if (!eligible) error = '缺少有效的原始集号，请单独修正'
    else if (!Number.isSafeInteger(offset)) error = '请输入整数偏移'
    else if (mapped.some(number => !Number.isSafeInteger(number) || number < 0)) error = '修正后的集号必须是整数，最小为 E0'
    else if (episodes && mapped.some(number => !allowed.has(number))) error = '目标集号在 TMDB 中不存在'
    return { file, episodes: mapped, currentOffset: currentEpisodeOffset(file), eligible, selected: selected.has(file.path), error, overlaps: [] as string[] }
  })
  // Compare the resulting workspace, including files outside the chosen source.
  const replacements = new Map(rows.filter(row => row.selected).map(row => [row.file.path, row]))
  const owners = new Map<number, string[]>()
  for (const file of allFiles) {
    if (file.removed || file.entry?.type !== 'tv' || file.entry.tmdbId !== tmdbId) continue
    const replacement = replacements.get(file.path)
    if ((replacement ? season : file.season) !== season || replacement?.error) continue
    for (const number of replacement?.episodes ?? file.episodes) {
      const paths = owners.get(number) ?? []
      paths.push(file.path)
      owners.set(number, paths)
    }
  }
  for (const row of rows) {
    if (!row.selected || row.error) continue
    row.overlaps = [...new Set(row.episodes.flatMap(number => owners.get(number) ?? []))]
      .filter(path => path !== row.file.path)
  }
  return rows
}

export function applyOffsetMappings(items: RecognitionItem[], mappings: OffsetMapping[]) {
  const changes = new Map(mappings.map(mapping => [mapping.path, mapping]))
  return items.map(file => {
    const mapping = changes.get(file.path)
    return mapping ? { ...file, season: mapping.season, episodes: [...mapping.episodes] } : file
  })
}
