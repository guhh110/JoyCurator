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
    id: 'organize',
    title: '文件整理',
    icon: FolderOpen,
    items: [
      { name: 'workbench', title: '整理工作台', icon: FolderOpen },
      { name: 'records', title: '整理记录', icon: History },
    ],
  },
  {
    id: 'recognition',
    title: '自动识别',
    icon: Layers3,
    items: [
      { name: 'automatic-recognition', title: '识别工作区', icon: Layers3 },
      { name: 'recognition-rules', title: '识别规则', icon: ListOrdered },
      { name: 'extraction-history', title: '执行历史', icon: FileClock },
      { name: 'recognition-debug', title: '识别调试', icon: Bug },
    ],
  },
  {
    id: 'system',
    title: '系统设置',
    icon: Settings2,
    items: [{ name: 'settings', title: '设置', icon: Settings2 }],
  },
] as const
// Detail pages that are not in the menu still belong to their parent's group.
const detailParents: Record<string, string> = {
  'recognition-rule-edit': 'recognition-rules',
}
export function groupForRoute(name: unknown) {
  const target = detailParents[String(name)] ?? name
  return (
    navigation.find((group) =>
      group.items.some((item) => item.name === target),
    )?.id ?? 'organize'
  )
}
