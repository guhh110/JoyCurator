import { describe, expect, it } from 'vitest'
import { operationNeedsAttention, recordState } from '../../../frontend/src/features/records/presentation'

describe('record filters', () => {
  it('only accepts current task states from the URL', () => {
    expect(recordState('queued')).toBe('queued')
    expect(recordState('interrupted')).toBe('interrupted')
    expect(recordState('manual_review')).toBe('')
    expect(recordState(['failed'])).toBe('')
  })
  it('includes errors and interrupted intermediate operations without flagging normal progress', () => {
    expect(operationNeedsAttention({ status: 'completed', error: '' }, 'completed')).toBe(false)
    expect(operationNeedsAttention({ status: 'pending', error: '' }, 'cancelled')).toBe(false)
    expect(operationNeedsAttention({ status: 'preparing', error: '' }, 'running')).toBe(false)
    expect(operationNeedsAttention({ status: 'preparing', error: '' }, 'interrupted')).toBe(true)
    expect(operationNeedsAttention({ status: 'manual_review', error: '' }, 'failed')).toBe(true)
    expect(operationNeedsAttention({ status: 'pending', error: 'failure' }, 'failed')).toBe(true)
  })
})
