import { computed, ref, type Ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api } from '../../api'
import type { RunPage } from '../../model'
import { keys } from '../../app/query'
import { useRuntime } from '../../app/runtime'

export function useRecords(page: Ref<number>, purpose: 'organize' | 'scrape', search: Ref<string> = ref(''), state: Ref<string> = ref('')) {
  const runtime = useRuntime()
  return useQuery({
    queryKey: computed(() => keys.runs(page.value, purpose, search.value, state.value)),
    queryFn: ({ signal }) => api<RunPage>(`${purpose === 'scrape' ? '/scrape' : ''}/runs?page=${page.value}&search=${encodeURIComponent(search.value)}&state=${encodeURIComponent(state.value)}`, undefined, 'GET', { signal }),
    staleTime: 0,
    refetchInterval: query => runtime.executing.value || query.state.data?.items.some(run => ['running', 'queued'].includes(run.state)) ? 1500 : false,
  })
}
