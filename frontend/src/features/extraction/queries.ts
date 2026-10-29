import { useQuery } from '@tanstack/vue-query'
import { api } from '../../api'
import { keys } from '../../app/query'
import type { RuleLibrary } from './model'
export function useRuleLibrary() {
  return useQuery({
    queryKey: keys.ruleLibrary,
    queryFn: ({ signal }) =>
      api<RuleLibrary>('/extraction/rules', undefined, 'GET', { signal }),
  })
}
