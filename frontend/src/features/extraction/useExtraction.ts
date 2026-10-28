import { onScopeDispose, ref } from 'vue'
import { api } from '../../api'
import type { ExtractionResult, Rule } from './model'

// One owner per page. Sequence checks reject stale responses even if a transport
// ignores abort; disposing the page also terminates its pending request.
export function useExtraction() {
  const path = ref(''),
    root = ref(''),
    busy = ref(false),
    error = ref(''),
    notice = ref('')
  const result = ref<ExtractionResult | null>(null)
  let sequence = 0,
    controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  function stop() {
    sequence++
    clearTimeout(timer)
    controller?.abort()
    controller = null
    busy.value = false
  }
  function cancel() {
    stop()
    error.value = ''
    notice.value = '已取消调试'
  }
  async function run(rules?: Rule[]) {
    stop()
    const current = sequence,
      active = new AbortController()
    controller = active
    busy.value = true
    error.value = ''
    notice.value = ''
    result.value = null
    timer = setTimeout(() => {
      if (current === sequence) {
        stop()
        error.value = '调试超过 90 秒，已停止请求。'
      }
    }, 90_000)
    try {
      const value = await api<ExtractionResult>(
        '/extraction/test',
        {
          path: path.value,
          ...(root.value ? { root: root.value } : {}),
          ...(rules ? { rules } : {}),
        },
        'POST',
        { signal: active.signal },
      )
      if (current === sequence) result.value = value
    } catch (e) {
      if (current === sequence) error.value = (e as Error).message
    } finally {
      if (current === sequence) {
        clearTimeout(timer)
        controller = null
        busy.value = false
      }
    }
  }
  onScopeDispose(stop)
  return { path, root, busy, error, notice, result, run, cancel }
}
