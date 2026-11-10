import { computed, onScopeDispose, ref } from 'vue'
import { api } from '../../api'
import type { Pipeline } from './model'
import { useToastFeedback } from '../../app/toast'

export interface ExampleResult {
  ruleId: string
  paths: string[]
  expected: unknown
  actual: unknown
  passed: boolean
  error: string
}

export function useRuleExamples(currentRules: () => Pipeline | undefined) {
  const results = ref<ExampleResult[]>([])
  const checkedKey = ref(''),
    error = ref(''),
    busy = ref(false)
  useToastFeedback(error)
  let controller: AbortController | undefined
  let sequence = 0
  const currentKey = computed(() => JSON.stringify(currentRules()))
  const stale = computed(
    () => !!checkedKey.value && checkedKey.value !== currentKey.value,
  )
  const failures = computed(() =>
    results.value.filter((result) => !result.passed),
  )
  const passed = computed(
    () =>
      checkedKey.value === currentKey.value &&
      !error.value &&
      !failures.value.length,
  )

  async function run(pipeline: Pipeline) {
    if (busy.value) return false
    const key = JSON.stringify(pipeline)
    if (passed.value && checkedKey.value === key) return true
    const request = ++sequence
    const active = new AbortController()
    controller = active
    busy.value = true
    error.value = ''
    checkedKey.value = ''
    results.value = []
    const timer = setTimeout(() => active.abort(), 90_000)
    try {
      const value = await api<ExampleResult[]>(
        '/extraction/examples',
        { pipeline },
        'POST',
        { signal: active.signal },
      )
      if (request !== sequence) return false
      results.value = value
      checkedKey.value = key
      return key === currentKey.value && !failures.value.length
    } catch (e) {
      if (request === sequence)
        error.value = active.signal.aborted
          ? '样例验证超时，请重试。'
          : (e as Error).message
      return false
    } finally {
      clearTimeout(timer)
      if (request === sequence) {
        busy.value = false
        controller = undefined
      }
    }
  }
  function reset() {
    sequence++
    controller?.abort()
    controller = undefined
    busy.value = false
    checkedKey.value = ''
    results.value = []
    error.value = ''
  }
  onScopeDispose(reset)
  return {
    results,
    checkedKey,
    error,
    busy,
    stale,
    failures,
    passed,
    run,
    reset,
  }
}
