import { computed, ref, type Ref } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import { api, name } from '../../api'
import { confirmAction } from '../../app/confirm'
import { keys } from '../../app/query'
import { pageNumber } from '../../app/routing'
import type { ExtractionResult, Trace } from './model'

export const historyStates: Record<string, string> = {
  completed: '已完成', running: '运行中', cancelled: '已取消',
  interrupted: '已中断', failed: '失败',
}
export function historyContext(query: LocationQuery): LocationQueryRaw {
  return {
    ...(typeof query.path === 'string' && query.path ? { path: query.path } : {}),
    ...(pageNumber(query.page) > 1 ? { page: pageNumber(query.page) } : {}),
  }
}
export function lastHistoryPage(total: number) {
  return Math.max(1, Math.ceil(total / 20))
}
export function historyTime(value?: string) {
  return value ? new Date(value).toLocaleString() : '—'
}
export function useHistoryList(page: Ref<number>, path: Ref<string>) {
  return useQuery({
    queryKey: computed(() => [...keys.traces, page.value, path.value]),
    queryFn: ({ signal }) => api<{ items: Trace[]; total: number }>(
      `/extraction/traces?page=${page.value}&path=${encodeURIComponent(path.value)}`,
      undefined, 'GET', { signal },
    ),
    staleTime: 0,
    refetchInterval: query => query.state.data?.items.some(item => item.status === 'running') ? 1500 : false,
  })
}
export function useHistoryDetail(id: Ref<string>) {
  return useQuery({
    queryKey: computed(() => keys.trace(id.value)),
    queryFn: ({ signal }) => api<ExtractionResult & { trace: Trace }>(
      `/extraction/traces/${encodeURIComponent(id.value)}`, undefined, 'GET', { signal },
    ),
    staleTime: 0,
    refetchInterval: query => query.state.data?.trace.status === 'running' ? 1200 : false,
  })
}
export function useTraceRemoval(afterDelete: () => Promise<unknown>) {
  const client = useQueryClient()
  const pendingId = ref('')
  const error = ref('')
  const mutation = useMutation({
    mutationFn: (id: string) => api(`/extraction/traces/${encodeURIComponent(id)}`, undefined, 'DELETE'),
  })
  async function remove(trace: Trace) {
    if (pendingId.value || trace.status === 'running') return
    pendingId.value = trace.id
    error.value = ''
    try {
      const accepted = await confirmAction({
        title: '清理这条识别历史？',
        description: `清理「${name(trace.path)}」本次执行的轨迹和步骤？不会删除文件或影响工作区识别结果，清理后无法撤销。`,
        confirmLabel: '确认清理', destructive: true,
      })
      if (!accepted) return
      await mutation.mutateAsync(trace.id)
      await client.cancelQueries({ queryKey: keys.trace(trace.id) })
      client.removeQueries({ queryKey: keys.trace(trace.id) })
      await client.invalidateQueries({ queryKey: keys.traces, refetchType: 'none' })
      await afterDelete()
    } catch (cause) {
      error.value = (cause as Error).message
    } finally {
      pendingId.value = ''
    }
  }
  return { remove, pendingId, error }
}
