import { effectScope, h, nextTick, ref } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useRuleExamples } from '../../../frontend/src/features/extraction/useRuleExamples'
import type { Rule } from '../../../frontend/src/features/extraction/model'
import RuleExamples from '../../../frontend/src/features/extraction/RuleExamples.vue'

const rule: Rule = { id: 'season', name: 'Season', kind: 'script', target: 'season', source: 'filename', order: 10, enabled: true, overwrite: false, config: { code: 'function extract(ctx) { return 1 }' }, examples: [{ path: '/第一集.mp4', expected: 1 }] }
const scopes: ReturnType<typeof effectScope>[] = []
function setup() {
  const scope = effectScope()
  scopes.push(scope)
  const rules = ref<Rule[]>(JSON.parse(JSON.stringify([rule])))
  return { scope, rules, state: scope.run(() => useRuleExamples(() => rules.value))! }
}
const response = (value: unknown, ok = true) => ({ ok, status: ok ? 200 : 400, json: async () => value })
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.unstubAllGlobals(); vi.useRealTimers() })

describe('save preflight examples', () => {
  it('fails null-equals-null execution errors and displays the rule diagnostic', async () => {
    const results = [{ ruleId: 'season', path: '/Film.mkv', expected: null, actual: null, passed: false, error: '规则 season：broken script' }]
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(results)))
    const { rules, state } = setup()
    expect(await state.run(rules.value)).toBe(false)
    expect(state.passed.value).toBe(false)
    expect(state.failures.value).toEqual(results)
    const html = await renderToString(h(RuleExamples, { results: state.results.value, checked: true, busy: false, stale: false }))
    expect(html).toContain('样例 0/1 通过')
    expect(html).toContain('执行错误：规则 season：broken script')
    expect(html).toContain('期望 null · 实际 null')
  })
  it('validates the whole draft chain and reuses only results for that exact chain', async () => {
    const fetch = vi.fn().mockResolvedValue(response([{ ruleId: 'season', path: '/第一集.mp4', expected: 1, actual: 1, passed: true, error: '' }]))
    vi.stubGlobal('fetch', fetch)
    const { rules, state } = setup()
    expect(await state.run(rules.value)).toBe(true)
    expect(JSON.parse(fetch.mock.calls[0]![1].body).rules).toEqual(rules.value)
    expect(fetch.mock.calls[0]![0]).toBe('/api/extraction/examples')
    expect(await state.run(rules.value)).toBe(true)
    expect(fetch).toHaveBeenCalledTimes(1)
    rules.value[0]!.config.code = 'function extract(ctx) { return 2 }'
    expect(state.stale.value).toBe(true)
    expect(state.passed.value).toBe(false)
    await state.run(rules.value)
    expect(fetch).toHaveBeenCalledTimes(2)
  })
  it('blocks failed samples and failed requests and allows retries', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(response([{ ruleId: 'season', path: '/第一集.mp4', expected: 1, actual: 2, passed: false, error: '' }]))
      .mockResolvedValueOnce(response({ message: 'invalid script' }, false))
      .mockResolvedValueOnce(response([]))
    vi.stubGlobal('fetch', fetch)
    const { rules, state } = setup()
    expect(await state.run(rules.value)).toBe(false)
    expect(state.failures.value).toHaveLength(1)
    expect(await state.run(rules.value)).toBe(false)
    expect(state.error.value).toBe('invalid script')
    expect(await state.run(rules.value)).toBe(true)
    expect(state.error.value).toBe('')
  })
  it('rejects duplicate operations and responses for an edited draft', async () => {
    let finish!: (value: unknown) => void
    const fetch = vi.fn(() => new Promise(resolve => { finish = resolve }))
    vi.stubGlobal('fetch', fetch)
    const { rules, state } = setup()
    const pending = state.run(rules.value)
    expect(await state.run(rules.value)).toBe(false)
    rules.value[0]!.name = 'changed'
    finish(response([]))
    expect(await pending).toBe(false)
    expect(state.stale.value).toBe(true)
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('aborts on disposal and ignores a late response', async () => {
    let finish!: (value: unknown) => void
    let signal!: AbortSignal
    vi.stubGlobal('fetch', vi.fn((_url, options) => { signal = options.signal; return new Promise(resolve => { finish = resolve }) }))
    const { rules, state, scope } = setup()
    const pending = state.run(rules.value)
    scope.stop()
    expect(signal.aborted).toBe(true)
    finish(response([]))
    expect(await pending).toBe(false)
    await nextTick()
    expect(state.checkedKey.value).toBe('')
    expect(state.results.value).toEqual([])
  })
  it('aborts and reports a request that exceeds the validation deadline', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn((_url, options) => new Promise((_resolve, reject) => {
      options.signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
    })))
    const { rules, state } = setup()
    const pending = state.run(rules.value)
    await vi.advanceTimersByTimeAsync(90_000)
    expect(await pending).toBe(false)
    expect(state.error.value).toBe('样例验证超时，请重试。')
    expect(state.busy.value).toBe(false)
  })
})
