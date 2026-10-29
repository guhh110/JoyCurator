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
