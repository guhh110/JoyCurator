import { computed, ref, watch, onScopeDispose, type Ref } from 'vue'
import { api } from '../../api'
import type { Batch, Recognition, ScrapeContent, ScrapePreview } from '../../model'
import type { Runtime } from '../../app/runtime'

export function useScrapeBatchPreview(options: {
  task: Ref<Recognition | null>; busy: Ref<boolean>; runtime: Runtime;
  act: (fn: () => Promise<void>) => Promise<void>; notice: Ref<string>
}) {
  const { task, busy, runtime, act, notice } = options
  const content = ref<ScrapeContent>('all'), onlyMissing = ref(true)
  const preview = ref<ScrapePreview | null>(null)
  let generation = 0, request: AbortController | null = null
  function invalidate() { generation++; request?.abort(); preview.value = null }
  watch([content, onlyMissing, () => task.value?.id, () => task.value?.revision], invalidate, { flush: 'sync' })
  onScopeDispose(invalidate)
  const ready = computed(() => !!preview.value?.artifacts.length && !preview.value.issues.length && !busy.value)
  async function prepare() {
    await act(async () => {
      if (!task.value) return
      invalidate()
      const current = generation, id = task.value.id, revision = task.value.revision
      request = new AbortController()
      const result = await api<ScrapePreview>('/scrape/batches/preview', { recognitionId: id, revision, content: content.value, onlyMissing: onlyMissing.value }, 'POST', { signal: request.signal })
      if (generation === current && task.value?.id === id && task.value.revision === revision) preview.value = result
    })
  }
  async function execute() {
    if (!ready.value) return
    await act(async () => {
      await api<Batch>('/scrape/batches/execute', { id: preview.value!.id })
      invalidate()
      await runtime.refreshRecords()
      notice.value = '已加入刮削队列'
    })
  }
  return { content, onlyMissing, preview, ready, prepare, execute, invalidate, busy }
}
