import { computed, inject, provide, ref, watch, type InjectionKey } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import {
  api,
  type Batch,
  type Run,
  type Settings,
} from '../api'
import { keys } from './query'
import { useToastFeedback } from './toast'

export function createRuntime() {
  const client = useQueryClient()
  const error = ref(''),
    activeId = ref(localStorage.getItem('joycurator-active') || ''),
    detailId = ref(''),
    detailTitle = ref(''),
    undoing = ref<Run | null>(null)
  useToastFeedback(error)
  const settingsQuery = useQuery({
    queryKey: keys.settings,
    queryFn: ({ signal }) =>
      api<Settings>('/settings', undefined, 'GET', { signal }),
  })
  const batchQuery = useQuery({
    queryKey: keys.batch,
    queryFn: ({ signal }) =>
      api<Batch | null>('/batches', undefined, 'GET', { signal }),
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchInterval: (query) => (query.state.data?.active ? 1200 : false),
  })
  const activeQuery = useQuery({
    queryKey: computed(() => activeId.value.startsWith('scrape:') ? ['scrape', 'active', activeId.value] : keys.run(activeId.value)),
    enabled: computed(() => !!activeId.value),
    queryFn: ({ signal }) =>
      api<Run>(
        `${activeId.value.startsWith('scrape:') ? '/scrape' : ''}/runs/${encodeURIComponent(activeId.value.replace(/^scrape:/, ''))}`,
        undefined,
        'GET',
        { signal },
      ),
    staleTime: 0,
    refetchInterval: (query) =>
      ['running', 'queued'].includes(query.state.data?.state ?? '')
        ? 1200
        : false,
  })
  const detailQuery = useQuery({
    queryKey: computed(() => detailId.value.startsWith('scrape:') ? ['scrape', 'detail', detailId.value] : keys.run(detailId.value)),
    enabled: computed(() => !!detailId.value),
    queryFn: ({ signal }) =>
      api<Run>(
        `${detailId.value.startsWith('scrape:') ? '/scrape' : ''}/runs/${encodeURIComponent(detailId.value.replace(/^scrape:/, ''))}`,
        undefined,
        'GET',
        { signal },
      ),
    staleTime: 0,
    refetchInterval: (query) =>
      ['running', 'queued'].includes(query.state.data?.state ?? '')
        ? 1200
        : false,
  })
  const active = computed(() => activeQuery.data.value ?? null)
  const batch = computed(() => batchQuery.data.value ?? null)
  const executing = computed(
    () =>
      ['running', 'queued'].includes(active.value?.state ?? '') ||
      !!batch.value?.active,
  )
  let completedId = ''
  watch(active, (value) => {
    if (
      !value ||
      ['running', 'queued'].includes(value.state) ||
      completedId === value.id
    )
      return
    completedId = value.id
    localStorage.removeItem('joycurator-active')
    void client.invalidateQueries({ queryKey: ['runs', 'list'] })
    void client.invalidateQueries({ queryKey: ['scrape'] })
  })
  function startRun(value: Run) {
    client.setQueryData(keys.run(value.id), value)
    activeId.value = value.purpose === 'scrape' ? `scrape:${value.id}` : value.id
    localStorage.setItem('joycurator-active', activeId.value)
  }
  function acceptBatch(value: Batch | null) {
    client.setQueryData(keys.batch, value)
  }
  function inspect(value: Run) {
    detailTitle.value = value.title
    detailId.value = value.purpose === 'scrape' ? `scrape:${value.id}` : value.id
  }
  const action = useMutation({
    mutationFn: async ({ run, kind }: { run: Run; kind: string }) => {
      error.value = ''
      const prefix = run.purpose === 'scrape' ? '/scrape' : ''
      if (kind === 'cancel') {
        await api(`${prefix}/runs/${run.id}/cancel`, {})
        await client.invalidateQueries({ queryKey: keys.run(run.id) })
      } else {
        const value = await api<Run>(`${prefix}/runs/${run.id}/${kind}`, {})
        startRun(value)
        detailId.value = ''
      }
      await Promise.all([
        client.invalidateQueries({ queryKey: ['runs', 'list'] }),
        client.invalidateQueries({ queryKey: ['scrape'] }),
        client.invalidateQueries({ queryKey: keys.batch }),
      ])
    },
  })
  async function recordAction(run: Run, kind: string) {
    undoing.value = null
    try {
      await action.mutateAsync({ run, kind })
    } catch (cause) {
      error.value = (cause as Error).message
    }
  }
  return {
    error,
    active,
    activeId,
    activeQuery,
    batch,
    batchQuery,
    settingsQuery,
    executing,
    startRun,
    acceptBatch,
    inspect,
    detailId,
    detailTitle,
    detailQuery,
    undoing,
    recordAction,
    actionBusy: action.isPending,
    client,
  }
}
export type Runtime = ReturnType<typeof createRuntime>
const runtimeKey: InjectionKey<Runtime> = Symbol('application-runtime')
export function provideRuntime() {
  const runtime = createRuntime()
  provide(runtimeKey, runtime)
  return runtime
}
export function useRuntime() {
  const runtime = inject(runtimeKey)
  if (!runtime) throw new Error('Missing application runtime')
  return runtime
}
