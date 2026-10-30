import { describe, expect, it } from 'vitest'
import {
  fieldMajor,
  fieldOrder,
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
  it('orders rules field by field and keeps relative order inside a field', () => {
    const a = { ...rule('a', 10), target: 'title' as const }
    const b = { ...rule('b', 20), target: 'season' as const }
    const c = { ...rule('c', 30), target: 'title' as const }
    const d = { ...rule('d', 40), target: 'mediaType' as const }
    const sorted = fieldMajor([a, b, c, d], (item) => item.target)
    expect(sorted.map((item) => item.id)).toEqual(['b', 'd', 'a', 'c'])
    expect(fieldOrder.indexOf('mediaType')).toBeGreaterThan(
      fieldOrder.indexOf('episodes'),
    )
  })
  it('distinguishes unknown season from specials and keeps combined episodes', () => {
    expect(valueText(null)).toBe('未知')
    expect(valueText(0)).toBe('0')
    expect(valueText([1, 2, 3])).toBe('[1,2,3]')
  })
})
