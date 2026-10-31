export const modeNames: Record<string, string> = {
  copy: '复制',
  move: '移动',
  hardlink: '硬链接',
  scrape: '刮削',
}
export const kindNames: Record<string, string> = {
  video: '视频',
  subtitle: '字幕',
  nfo: 'NFO',
  image: '图片',
}
export const states: Record<string, string> = {
  queued: '等待中',
  running: '进行中',
  completed: '已完成',
  failed: '部分失败',
  cancelled: '已取消',
  interrupted: '已中断',
  undone: '已撤销',
  pending: '等待中',
  published: '已写入',
  staged: '待发布',
  preparing: '处理中',
  undoing: '撤销中',
}
export function date(value: string) {
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
