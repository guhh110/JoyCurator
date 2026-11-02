import { computed, ref, type Ref } from 'vue'
import { api } from '../../api'
import type { Recognition } from '../../model'
import { recognitionGroups, sourceOf, sourceSummaries } from '../../recognition'
import { confirmAction } from '../../app/confirm'

type Group = ReturnType<typeof recognitionGroups>[number]
export function useWorkspaceSources(options: {
  task: Ref<Recognition | null>
  groups: Readonly<Ref<Group[]>>
  act: (fn: () => Promise<void>) => Promise<void>
  invalidatePreview: () => void
  changeFilter: () => void
}) {
  const { task, groups, act, invalidatePreview, changeFilter } = options
  // '' shows every added source; otherwise only files from that added path.
  const source = ref('')
  const sources = computed(() =>
    sourceSummaries(task.value?.items || [], task.value?.paths || []),
  )
  const scoped = computed(() =>
    source.value
      ? recognitionGroups(
        (task.value?.items || []).filter(
          (file) =>
            sourceOf(file.path, task.value?.paths || []) === source.value,
        ),
      )
      : groups.value,
  )
  async function removeSource(path: string) {
    if (!task.value) return
    const count = sources.value.find((item) => item.path === path)?.files ?? 0
    if (!await confirmAction({
      title: '移出来源？',
      description: `将「${path}」及其 ${count} 个文件移出整理工作区？磁盘上的文件不会被删除。`,
      confirmLabel: '移出来源',
      destructive: true,
    })) return
    await act(async () => {
      task.value = await api<Recognition>(
        `/recognitions/${task.value!.id}/sources/remove`,
        { path, revision: task.value!.revision },
      )
      invalidatePreview()
      if (source.value === path) source.value = ''
      changeFilter()
    })
  }
  function chooseSource(path: string) {
    source.value = path
    changeFilter()
  }
  return { source, sources, scoped, chooseSource, removeSource }
}
