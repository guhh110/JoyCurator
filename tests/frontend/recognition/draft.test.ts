import { describe, expect, it } from 'vitest'
import {
  useRuleDraft,
  draftRule,
  trialFingerprint,
} from '../../../frontend/src/features/extraction/useRuleDraft'
import type { Rule } from '../../../frontend/src/features/extraction/model'
const rule = (id: string, order: number): Rule => ({
  id,
  order,
  name: id,
  kind: 'script',
  target: 'title',
  source: 'filename',
  enabled: true,
  overwrite: false,
  config: { code: 'function extract(ctx) { return ctx.text; }', params: {} },
  examples: [],
})
describe('rule workbench draft', () => {
  it('includes current edits in trial payloads without an apply step', () => {
    const draft = useRuleDraft()
    draft.load([rule('a', 10), rule('b', 20)], {})
    expect(draft.dirty.value).toBe(false)
    draft.selectedKey.value = draft.drafts.value[0]!.key
    draft.editing.value!.rule.config.code =
      'function extract(ctx) { return "edited"; }'
    draft.editing.value!.paramsText = '{"title":"current"}'
    expect(draft.validation.value.rules[0]!.config.params).toEqual({
      title: 'current',
    })
    expect(draft.validation.value.rules[0]!.config.code).toContain('edited')
    expect(draft.dirty.value).toBe(true)
    draft.markSaved()
    expect(draft.dirty.value).toBe(false)
  })
  it('retains invalid JSON, edited IDs and selection through switching and reordering', () => {
    const draft = useRuleDraft()
    draft.load([rule('a', 10), rule('b', 20)], {})
    const first = draft.drafts.value[0]!
    first.rule.id = 'renamed'
    first.paramsText = '{ unfinished'
    draft.selectedKey.value = draft.drafts.value[1]!.key
    draft.move(first.key, 1)
    expect(draft.editing.value!.rule.id).toBe('b')
    draft.selectedKey.value = first.key
    expect(draft.editing.value!.paramsText).toBe('{ unfinished')
    expect(draft.validation.value.errors[0]!.key).toBe(first.key)
    expect(draft.drafts.value.map((item) => item.rule.order)).toEqual([10, 20])
    first.paramsText = '{}'
    expect(draft.validation.value.errors).toEqual([])
    expect(draft.validation.value.rules.map((item) => item.id)).toEqual([
      'b',
      'renamed',
    ])
  })
  it('invalidates trial identity for rule changes, input changes and order changes', () => {
    const draft = useRuleDraft()
    draft.load([rule('a', 10), rule('b', 20)], {})
    const baseline = trialFingerprint(draft.fingerprint.value, '/a.mkv', '/')
    expect(trialFingerprint(draft.fingerprint.value, '/b.mkv', '/')).not.toBe(
      baseline,
    )
    expect(
      trialFingerprint(draft.fingerprint.value, '/a.mkv', '/source'),
    ).not.toBe(baseline)
    const second = draft.drafts.value[1]!.key
    draft.move(second, -1)
    expect(trialFingerprint(draft.fingerprint.value, '/a.mkv', '/')).not.toBe(
      baseline,
    )
    draft.move(second, 1)
    expect(trialFingerprint(draft.fingerprint.value, '/a.mkv', '/')).toBe(
      baseline,
    )
    draft.drafts.value[0]!.rule.enabled = false
    expect(trialFingerprint(draft.fingerprint.value, '/a.mkv', '/')).not.toBe(
      baseline,
    )
  })
  it('blocks malformed parameters, examples and duplicate IDs across the whole chain', () => {
    const draft = useRuleDraft()
    draft.load([rule('a', 10), rule('b', 20)], {})
    draft.drafts.value[1]!.rule.id = 'a'
    expect(draft.validation.value.errors[0]!.message).toContain('重复')
    draft.drafts.value[1]!.rule.id = 'b'
    draft.drafts.value[1]!.examplesText = '[{"path":"/a.mkv"}]'
    expect(draft.validation.value.errors[0]!.message).toContain('expected')
    draft.drafts.value[1]!.examplesText = '[]'
    draft.drafts.value[1]!.paramsText = '[]'
    expect(draft.validation.value.errors[0]!.message).toContain('JSON 对象')
  })
  it('keeps raw JSON UI state out of saved rules and materializes source scripts', () => {
    const draft = useRuleDraft()
    const current = rule('clean', 10)
    current.source = 'cleanFilename'
    draft.load([current], {
      cleanFilename: 'function extract(ctx) { return ctx.text.trim(); }',
    })
    draft.selectedKey.value = draft.drafts.value[0]!.key
    const output = draftRule(draft.editing.value!)
    expect(output.config.sourceCode).toContain('trim()')
    expect(output).not.toHaveProperty('key')
    expect(output).not.toHaveProperty('paramsText')
    expect(current.config.sourceCode).toBeUndefined()
    draft.remove(draft.editing.value!.key)
    expect(draft.editing.value).toBeUndefined()
    expect(draft.validation.value.rules).toEqual([])
  })
  it('keeps the chain field-major and only moves rules within their field', () => {
    const draft = useRuleDraft()
    const season = { ...rule('season', 10), target: 'season' as const }
    const title = rule('title', 20)
    const late = { ...rule('late-season', 30), target: 'season' as const }
    const reordered = draft.load([title, season, late], {})
    expect(reordered).toBe(true)
    expect(draft.drafts.value.map((item) => item.rule.id)).toEqual([
      'season',
      'late-season',
      'title',
    ])
    expect(draft.drafts.value.map((item) => item.rule.order)).toEqual([
      10, 20, 30,
    ])
    const lateKey = draft.drafts.value[1]!.key
    draft.move(lateKey, 1)
    expect(draft.drafts.value[1]!.key).toBe(lateKey)
    draft.move(lateKey, -1)
    expect(draft.drafts.value[0]!.key).toBe(lateKey)
    expect(draft.siblings(lateKey)).toEqual({ first: true, last: false })
    draft.drafts.value[2]!.rule.target = 'tmdbId'
    draft.normalize()
    expect(draft.drafts.value[0]!.rule.id).toBe('title')
  })
})
