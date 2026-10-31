import {
  Bug,
  FileClock,
  FolderOpen,
  History,
  Layers3,
  ListOrdered,
  Settings2,
} from '@lucide/vue'
export const navigation = [
  {
    id: 'workspace',
    title: '工作区',
    icon: FolderOpen,
    items: [
      { name: 'automatic-recognition', title: '整理工作区', icon: Layers3 },
      { name: 'scrape', title: '媒体刮削', icon: FolderOpen },
    ],
  },
  {
    id: 'records',
    title: '记录',
    icon: Layers3,
    items: [
      { name: 'records', title: '整理记录', icon: History },
      { name: 'scrape-records', title: '刮削记录', icon: History },
      { name: 'extraction-history', title: '识别历史', icon: FileClock },
    ],
  },
  {
    id: 'config',
    title: '配置',
    icon: Settings2,
    items: [
      { name: 'recognition-rules', title: '识别规则', icon: ListOrdered },
      { name: 'recognition-debug', title: '识别调试', icon: Bug },
      { name: 'settings', title: '设置', icon: Settings2 },
    ],
  },
] as const
// Detail pages that are not in the menu still belong to their parent's group.
const detailParents: Record<string, string> = {
  'recognition-rule-edit': 'recognition-rules',
  'extraction-history-detail': 'extraction-history',
}
export function groupForRoute(name: unknown) {
  const target = menuRoute(name)
  return (
    navigation.find((group) =>
      group.items.some((item) => item.name === target),
    )?.id ?? 'workspace'
  )
}
export function menuRoute(name: unknown) {
  return detailParents[String(name)] ?? name
}
export function groupTitleForRoute(name: unknown) {
  return navigation.find(group => group.id === groupForRoute(name))!.title
}
