import { computed, ref, onDeactivated, onScopeDispose, type Ref } from 'vue'
import { api } from '../../api'
import type { Batch, BatchPreview, DuplicatePolicy, Recognition } from '../../model'
import { sortByEpisode } from '../../recognition'
import type { Runtime } from '../../app/runtime'

export function useWorkspacePreview(options: {
  task: Ref<Recognition | null>
  busy: Ref<boolean>
  notice: Ref<string>
  runtime: Runtime
  act: (fn: () => Promise<void>) => Promise<void>
}) {
  const { task, busy, notice, runtime, act } = options
  const batch = runtime.batch
  let generation = 0
  let request: AbortController | null = null
  function abandon() { generation++; request?.abort(); request = null; preview.value = null }
  onDeactivated(abandon)
  onScopeDispose(abandon)
  const preview = ref<BatchPreview | null>(null)
  const mode = ref('copy')
  const previewPage = ref(1)
  const duplicatePolicy = ref<DuplicatePolicy>('skip')
  const previewReady = computed(() => !!preview.value && preview.value.duplicatePolicy === duplicatePolicy.value && !busy.value && !preview.value.issues.length && preview.value.rows.length > 0)
  const previewRows = computed(() => {
    if (!preview.value) return []
    const groupOrder = new Map(preview.value.groups.flatMap((group, index) => group.rows.map(row => [row.source, index] as const)))
    const ungrouped = preview.value.groups.length
    return sortByEpisode(preview.value.rows, row => ({ ...row, path: row.source }))
      .sort((a, b) => (groupOrder.get(a.source) ?? ungrouped) - (groupOrder.get(b.source) ?? ungrouped))
      .slice((previewPage.value - 1) * 30, previewPage.value * 30)
  })

  async function prepare() {
    await act(async () => {
      if (!task.value) return
      if (!preview.value) duplicatePolicy.value = 'skip'
      const revision = task.value.revision
      const id = task.value.id
      const current = ++generation
      request?.abort()
      request = new AbortController()
      const result = await api<BatchPreview>('/batches/preview', {
        recognitionId: task.value.id,
        revision: task.value.revision,
        mode: mode.value,
        duplicatePolicy: duplicatePolicy.value,
      }, 'POST', { signal: request.signal })
      if (current !== generation || task.value?.id !== id || task.value.revision !== revision) return
      request = null
      preview.value = result
      previewPage.value = 1
    })
  }
  async function changeDuplicatePolicy(value: string) {
    if (busy.value || !preview.value || !['skip', 'overwrite'].includes(value)) return
    duplicatePolicy.value = value as DuplicatePolicy
    await prepare()
  }
  async function execute() {
    if (!previewReady.value) return
    await act(async () => {
      if (!preview.value) return
      const value = await api<Batch>('/batches/execute', {
        id: preview.value.id,
      })
      runtime.acceptBatch(value)
      preview.value = null
      notice.value = '已加入整理队列，每部作品分别记录，可独立重试或撤销。'
    })
  }
  async function cancelBatch() {
    await act(async () => {
      if (batch.value) await api(`/batches/${batch.value.id}/cancel`, {})
    })
  }
  return { pending: busy, preview, mode, previewPage, previewRows, duplicatePolicy, previewReady, prepare, changeDuplicatePolicy, execute, cancelBatch }
}
