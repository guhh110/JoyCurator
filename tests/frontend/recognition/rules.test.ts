import { describe, expect, it } from 'vitest'
import {
  moveRule,
  valueText,
  type Rule,
} from '../../../frontend/src/features/extraction/model'
function rule(id: string, order: number): Rule {
  return {
    id,
    order,
    name: id,
    target: 'title',
    source: 'filename',
    enabled: true,
    kind: 'regex',
    overwrite: false,
    config: { pattern: '(?P<value>.+)', capture: 'value' },
    examples: [],
  }
}
describe('rule ordering and metadata display', () => {
  it('moves a rule and assigns distinct ascending orders while retaining IDs and settings', () => {
    const rules = [rule('a', 10), rule('b', 20), rule('c', 30)]
    rules[2]!.overwrite = true
    moveRule(rules, 2, 0)
    expect(rules.map((item) => item.id)).toEqual(['c', 'a', 'b'])
    expect(rules.map((item) => item.order)).toEqual([10, 20, 30])
    expect(rules[0]!.overwrite).toBe(true)
    moveRule(rules, 0, -1)
    expect(rules.map((item) => item.id)).toEqual(['c', 'a', 'b'])
  })
  it('distinguishes unknown season from specials and keeps combined episodes', () => {
    expect(valueText(null)).toBe('未知')
    expect(valueText(0)).toBe('0')
    expect(valueText([1, 2, 3])).toBe('[1,2,3]')
  })
})
