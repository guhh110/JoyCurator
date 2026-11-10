import { describe, it, expect } from 'vitest'
import {
  useRuleDraft,
  draftRule,
  trialFingerprint,
  withRule,
  canMoveRule,
} from '../../../frontend/src/features/extraction/useRuleDraft'
import { annotation, pipeline } from '../fixtures/pipeline'
describe('pipeline drafts', () => {
  it('keeps unfinished JSON and selected identity through ordinary annotation moves', () => {
    const d = useRuleDraft()
    d.load(
      pipeline([
        { ...annotation('a'), target: 'season' },
        { ...annotation('b', 20), target: 'year' },
      ]),
    )
    const a = d.drafts.value[1]!,
      b = d.drafts.value[2]!
    a.paramsText = '{'
    d.selectedKey.value = a.key
    d.move(a.key, 1)
    expect(d.editing.value).toBe(a)
    expect(d.validation.value.errors[0]?.key).toBe(a.key)
    expect(d.drafts.value.map((x) => x.rule.id)).toEqual([
      'tokenizer',
      'b',
      'a',
      'resolver',
    ])
    a.paramsText = '{}'
    expect(d.validation.value.errors).toEqual([])
    d.move(b.key, -1)
    expect(d.drafts.value[0]?.rule.id).toBe('tokenizer')
    d.remove(d.drafts.value[0]!.key)
    expect(d.drafts.value[0]?.rule.id).toBe('tokenizer')
  })
  it('limits moves to ordinary or title groups and inserts rules into the correct group', () => {
    const source = pipeline([
      annotation('title', 0),
      { ...annotation('season', 30), target: 'season' },
      annotation('other-title', 40),
    ])
    const d = useRuleDraft()
    d.load(source)
    const season = d.drafts.value[1]!,
      title = d.drafts.value[2]!,
      other = d.drafts.value[3]!
    expect(canMoveRule(d.drafts.value, season.key, 1)).toBe(false)
    expect(canMoveRule(d.drafts.value, title.key, -1)).toBe(false)
    d.move(title.key, -1)
    expect(d.dirty.value).toBe(false)
    d.move(other.key, -1)
    expect(d.drafts.value.map((x) => x.rule.id)).toEqual([
      'tokenizer',
      'season',
      'other-title',
      'title',
      'resolver',
    ])
    const added = withRule(source, {
      ...annotation('year', 50),
      target: 'year',
    })
    expect(added.annotators.map((r) => r.id)).toEqual([
      'season',
      'year',
      'title',
      'other-title',
    ])
    const changed = withRule(
      source,
      { ...annotation('title'), target: 'year' },
      'title',
    )
    expect(changed.annotators.map((r) => r.id)).toEqual([
      'season',
      'title',
      'other-title',
    ])
    expect(source.annotators[0]!.target).toBe('title')
  })
  it('materializes current edits without UI state or input mutation', () => {
    const source = pipeline(),
      d = useRuleDraft()
    d.load(source)
    expect(d.dirty.value).toBe(false)
    const a = d.drafts.value[1]!
    a.rule.name = 'changed'
    a.paramsText = '{"max":4}'
    const out = draftRule(a)
    expect(out.config.params).toEqual({ max: 4 })
    expect(out).not.toHaveProperty('key')
    expect(source.annotators[0]!.name).toBe('title')
    expect(d.dirty.value).toBe(true)
    d.markSaved()
    expect(d.dirty.value).toBe(false)
    expect(withRule(source, out, 'title').annotators[0]!.name).toBe('changed')
  })
  it('rejects malformed examples, parameters and duplicate IDs across stages', () => {
    const d = useRuleDraft()
    d.load(pipeline())
    const a = d.drafts.value[1]!
    a.rule.id = 'tokenizer'
    expect(d.validation.value.errors[0]?.message).toContain('重复')
    a.rule.id = 'title'
    a.paramsText = '[]'
    expect(d.validation.value.errors[0]?.message).toContain('JSON 对象')
    a.paramsText = '{}'
    a.examplesText = '[{"paths":["/Film.mkv"]}]'
    expect(d.validation.value.errors[0]?.message).toContain('expected')
  })
  it('changes trial identity for directory inputs, scripts and order', () => {
    const d = useRuleDraft()
    d.load(pipeline([annotation('a'), annotation('b', 20)]))
    const before = trialFingerprint(d.fingerprint.value, '/a.mkv\n/b.mkv', '/')
    d.move(d.drafts.value[2]!.key, -1)
    expect(
      trialFingerprint(d.fingerprint.value, '/a.mkv\n/b.mkv', '/'),
    ).not.toBe(before)
    expect(trialFingerprint(d.fingerprint.value, '/a.mkv', '/')).not.toBe(
      before,
    )
  })
})
