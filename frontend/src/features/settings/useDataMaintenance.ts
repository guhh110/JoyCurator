import { computed, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { api } from '../../api'
import { useAction } from '../../app/useAction'
import { confirmAction } from '../../app/confirm'
import { keys } from '../../app/query'

interface CleanupRequest { before: string; traces: boolean; cache: boolean; cacheBefore: number }
interface CleanupCounts { traces: number; cache: number }
interface CleanupPreview extends CleanupCounts { request: CleanupRequest }
function localDate(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}
export function useDataMaintenance() {
  const client = useQueryClient()
  const { act, busy, success } = useAction()
  const initial = new Date()
  initial.setDate(initial.getDate()-30)
  const before = ref(localDate(initial)), maxDate = localDate(new Date())
  const traces = ref(true), cache = ref(true)
  const preview = ref<CleanupPreview | null>(null)
  const count = computed(() => (preview.value?.traces ?? 0) + (preview.value?.cache ?? 0))
  watch([before, traces, cache], () => { preview.value = null }, { flush: 'sync' })
  async function inspect() {
    preview.value = null
    await act(async () => {
      preview.value = await api<CleanupPreview>('/maintenance/preview', {
        before: traces.value ? new Date(`${before.value}T00:00:00`).toISOString() : '',
        traces: traces.value, cache: cache.value, cacheBefore: 0,
      })
    })
  }
  async function cleanup() {
    if (!preview.value || !count.value || busy.value) return
    // Keep the reviewed cutoff fixed, including the cache expiry timestamp.
    const selected = preview.value
    await act(async () => {
      if (!await confirmAction({
        title: '确认清理这些数据？',
        description: `预计清理 ${selected.traces} 条识别历史和 ${selected.cache} 条过期缓存。预览后数据可能变化，以实际清理数量为准。清理不可撤销，不会删除媒体文件或修改工作区结果。`,
        confirmLabel: '确认清理', destructive: true,
      })) return
      const result = await api<CleanupCounts>('/maintenance/cleanup', selected.request)
      preview.value = null
      // A cleaned detail must not remain visible from an old query cache.
      await client.cancelQueries({ queryKey: ['extraction', 'trace'] })
      client.removeQueries({ queryKey: ['extraction', 'trace'] })
      await client.invalidateQueries({ queryKey: keys.traces })
      success.value = `已清理 ${result.traces} 条识别历史、${result.cache} 条过期缓存`
    })
  }
  return { before, maxDate, traces, cache, preview, count, busy, inspect, cleanup }
}
