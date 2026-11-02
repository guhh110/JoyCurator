import { computed, inject, provide, ref, watch, type InjectionKey } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { api } from '../api'
import type { Run, Settings, QueueStatus } from '../model'
import { keys } from './query'
import { useToastFeedback, notify } from './toast'

export function createRuntime() {
  const client = useQueryClient()
  const error = ref(''), detailId = ref(''), detailTitle = ref(''), undoing = ref<Run | null>(null)
  useToastFeedback(error)
  const settingsQuery = useQuery({ queryKey: keys.settings, queryFn: ({ signal }) => api<Settings>('/settings', undefined, 'GET', { signal }) })
  const queueQuery = useQuery({
    queryKey: keys.queue,
    queryFn: ({ signal }) => api<QueueStatus>('/queue', undefined, 'GET', { signal }),
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchInterval: query => query.state.data?.active || query.state.data?.recognizing ? 1200 : false,
  })
  const queue = computed(() => queueQuery.data.value ?? null)
  const executing = computed(() => !!queue.value?.active)
  watch(queue, (value, previous) => {
    if (!value || !previous || value.revision === previous.revision) return
    for (const key of [['runs', 'list'], ['recognitions'], ['scrape']]) void client.invalidateQueries({ queryKey: key })
  })
  const detailQuery = useQuery({
    queryKey: computed(() => keys.run(detailId.value)),
    enabled: computed(() => !!detailId.value),
    queryFn: ({ signal }) => api<Run>(`${detailId.value.startsWith('scrape:') ? '/scrape' : ''}/runs/${encodeURIComponent(detailId.value.replace(/^scrape:/, ''))}`, undefined, 'GET', { signal }),
    staleTime: 0,
    refetchInterval: query => ['running', 'queued'].includes(query.state.data?.state ?? '') ? 1200 : false,
  })
  function inspect(run: Run) { detailTitle.value = run.title; detailId.value = run.purpose === 'scrape' ? `scrape:${run.id}` : run.id }
  async function refreshQueue() { await client.invalidateQueries({ queryKey: keys.queue }) }
  async function refreshRecords() {
    await Promise.all([refreshQueue(), client.invalidateQueries({ queryKey: ['runs', 'list'] }), client.invalidateQueries({ queryKey: ['recognitions'] }), client.invalidateQueries({ queryKey: ['scrape'] })])
  }
  const action = useMutation({
    mutationFn: async ({ run, kind }: { run: Run; kind: string }) => {
      const id = run.purpose === 'scrape' ? `scrape:${run.id}` : run.id
      await api(`${run.purpose === 'scrape' ? '/scrape' : ''}/runs/${run.id}/${kind}`, {})
      notify(kind === 'cancel' ? '取消请求已提交' : kind === 'undo' ? '已加入撤销队列' : '已加入重试队列', 'success')
      await Promise.all([refreshRecords(), client.invalidateQueries({ queryKey: keys.run(id) })])
    },
  })
  async function recordAction(run: Run, kind: string) {
    undoing.value = null; error.value = ''
    try { await action.mutateAsync({ run, kind }) } catch (cause) { error.value = (cause as Error).message }
  }
  return {
    error, queue, queueQuery, executing, settingsQuery, detailId, detailTitle, detailQuery, undoing, inspect, recordAction, refreshQueue, refreshRecords,
    actionBusy: action.isPending, client
  }
}
export type Runtime = ReturnType<typeof createRuntime>
const runtimeKey: InjectionKey<Runtime> = Symbol('application-runtime')
export function provideRuntime() { const runtime = createRuntime(); provide(runtimeKey, runtime); return runtime }
export function useRuntime() { const runtime = inject(runtimeKey); if (!runtime) throw new Error('Missing application runtime'); return runtime }
