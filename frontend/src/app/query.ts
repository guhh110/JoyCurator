import { QueryClient } from '@tanstack/vue-query'
import { ApiError } from '../api'
export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 5_000,
        gcTime: 5 * 60_000,
        refetchOnWindowFocus: false,
        retry: (count, error) =>
          !(error instanceof ApiError && error.status < 500) && count < 1,
      },
      mutations: { retry: false },
    },
  })
}
export const queryClient = createQueryClient()
export const keys = {
  session: ['session'] as const,
  settings: ['settings'] as const,
  batch: ['batch', 'latest'] as const,
  recognition: (id = 'latest') => ['recognitions', id] as const,
  run: (id: string) => ['runs', 'detail', id] as const,
  runs: (page: number) => ['runs', 'list', page] as const,
  ruleLibrary: ['extraction', 'rules'] as const,
  traces: ['extraction', 'traces'] as const,
  trace: (id: string) => ['extraction', 'trace', id] as const,
}
