import type { RecognitionItem } from './api'

export const recognitionStates: Record<string, string> = {
  ready: '可整理',
  review: '待确认',
  unmatched: '未匹配',
  failed: '失败',
  pending: '等待识别',
}
export function recognitionGroups(items: RecognitionItem[]) {
  const groups = new Map<string, RecognitionItem[]>()
  for (const item of items) {
    if (item.removed) continue
    const list = groups.get(item.group) || []
    list.push(item)
    groups.set(item.group, list)
  }
  return [...groups].map(([id, files]) => {
    const entry = files.find((file) => file.entry)?.entry || null
    const state =
      ['failed', 'pending', 'review', 'unmatched', 'ready'].find((value) =>
        files.some((file) => file.state === value),
      ) || 'review'
    return {
      id,
      files,
      entry,
      title: entry?.title || files[0]!.metadata.title || '未识别作品',
      state,
    }
  })
}
// The folder or file the user added that a path came from: the longest added
// path that is the path itself or one of its parent directories.
export function sourceOf(path: string, roots: string[]) {
  let found = ''
  for (const root of roots) {
    const base = root.replace(/\/+$/, '') || '/'
    const inside =
      path === base || path.startsWith(base === '/' ? '/' : `${base}/`)
    if (inside && base.length > found.length) found = base
  }
  return found
}
export interface SourceSummary {
  path: string
  files: number
  works: number
  states: Record<string, number>
}
// One summary per added path, in the order the user added them.
export function sourceSummaries(
  items: RecognitionItem[],
  roots: string[],
): SourceSummary[] {
  const summaries = new Map<string, SourceSummary & { groups: Set<string> }>()
  for (const root of roots) {
    const path = root.replace(/\/+$/, '') || '/'
    if (!summaries.has(path))
      summaries.set(path, { path, files: 0, works: 0, states: {}, groups: new Set() })
  }
  for (const item of items) {
    if (item.removed) continue
    const summary = summaries.get(sourceOf(item.path, roots))
    if (!summary) continue
    summary.files++
    summary.groups.add(item.group)
    summary.states[item.state] = (summary.states[item.state] || 0) + 1
  }
  return [...summaries.values()].map(({ groups, ...rest }) => ({
    ...rest,
    works: groups.size,
  }))
}
export function recognitionEdits(items: RecognitionItem[]) {
  return items.map((item) => ({
    path: item.path,
    group: item.group,
    type: item.entry?.type || '',
    tmdbId: item.entry?.tmdbId || 0,
    season: item.season,
    episodes: item.episodes,
    selected: item.selected,
    removed: item.removed,
  }))
}
