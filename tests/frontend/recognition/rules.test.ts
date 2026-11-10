import { describe, it, expect } from 'vitest'
import {
  pipelineRules,
  valueText,
} from '../../../frontend/src/features/extraction/model'
import { annotation, pipeline } from '../fixtures/pipeline'
describe('pipeline order and values', () => {
  it('orders ordinary annotations by order and places title annotations last', () => {
    const p = pipeline([
      annotation('title'),
      { ...annotation('season', 20), target: 'season' },
      { ...annotation('year', 5), target: 'year' },
    ])
    expect(pipelineRules(p).map((r) => r.id)).toEqual([
      'tokenizer',
      'year',
      'season',
      'title',
      'resolver',
    ])
  })
  it('distinguishes zero, unknown and combined episodes', () => {
    expect(valueText(null)).toBe('未知')
    expect(valueText(0)).toBe('0')
    expect(valueText([1, 3])).toBe('[1,3]')
  })
})
