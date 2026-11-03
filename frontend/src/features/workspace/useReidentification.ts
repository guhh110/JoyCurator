import { computed, ref, type Ref } from 'vue'
import type { Recognition, RecognitionItem } from '../../model'
import { notify } from '../../app/toast'

export function canReidentify(file: RecognitionItem) {
  return !file.removed && !file.execution?.locked && !file.execution?.completed
}
export function useReidentification(options: {
  task: Ref<Recognition | null>
  locked: Readonly<Ref<boolean>>
  commit: (paths: string[], id: string, revision: number) => Promise<boolean>
}) {
  const files = ref<RecognitionItem[]>([])
  const selected = ref<string[]>([])
  const context = ref<{ id: string; revision: number } | null>(null)
  const eligible = computed(() => files.value.filter(canReidentify))
  function open(items: RecognitionItem[]) {
    if (!options.task.value || options.locked.value) return
    if (!items.some(canReidentify)) { notify('没有可以重新识别的文件', 'error'); return }
    files.value = items
    selected.value = items.filter(canReidentify).map(file => file.path)
    context.value = { id: options.task.value.id, revision: options.task.value.revision }
  }
  function close() { context.value = null; files.value = []; selected.value = [] }
  async function submit() {
    if (!context.value || !selected.value.length || options.locked.value) return
    const current = context.value
    if (await options.commit([...selected.value], current.id, current.revision)) close()
  }
  function selectAll(value: boolean) { selected.value = value ? eligible.value.map(file => file.path) : [] }
  return { files, selected, context, eligible, open, close, submit, selectAll }
}
