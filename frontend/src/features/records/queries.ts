import { computed, type Ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api } from '../../api'
import type { RunPage, BatchPage } from '../../model'
import { keys } from '../../app/query'
import { useRuntime } from '../../app/runtime'

export function useRecords(page: Ref<number>, purpose: 'organize' | 'scrape') {
  const runtime = useRuntime()
  return useQuery({
    queryKey: computed(() => keys.runs(page.value, purpose)),
    queryFn: ({ signal }) => api<RunPage>(`${purpose === 'scrape' ? '/scrape' : ''}/runs?page=${page.value}`, undefined, 'GET', { signal }),
    staleTime: 0,
    refetchInterval: query => runtime.executing.value || query.state.data?.items.some(run => ['running', 'queued'].includes(run.state)) ? 1500 : false,
  })
}

export function useQueuedRecords(page: Ref<number>, purpose: 'organize' | 'scrape') {
  const runtime = useRuntime()
  return useQuery({
    queryKey: computed(() => keys.queued(purpose, page.value)),
    queryFn: ({ signal }) => api<RunPage>(`/queue/items?purpose=${purpose}&page=${page.value}`, undefined, 'GET', { signal }),
    refetchInterval: computed(() => runtime.executing.value ? 1500 : false),
  })
}
export function useBatches(page: Ref<number>) {
  const runtime = useRuntime()
  return useQuery({
    queryKey: computed(() => keys.batches(page.value)),
    queryFn: ({ signal }) => api<BatchPage>(`/batches?page=${page.value}`, undefined, 'GET', { signal }),
    refetchInterval: computed(() => runtime.executing.value ? 1500 : false),
  })
}
