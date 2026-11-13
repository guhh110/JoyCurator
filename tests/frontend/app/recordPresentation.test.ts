import { describe, expect, it } from 'vitest'
import { operationNeedsAttention, operationTiming, recordState } from '../../../frontend/src/features/records/presentation'

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

describe('operation timing', () => {
  it('shows whole-item duration and decimal MB/s', () => {
    expect(operationTiming({ durationMs: 42_300, speedBps: 118_600_000 })).toBe('42.3 秒 · 118.6 MB/s')
  })
  it('shows no metrics for an unfinished item and no speed without a transfer', () => {
    expect(operationTiming({ durationMs: null, speedBps: null })).toBe('')
    expect(operationTiming({ durationMs: 0.25, speedBps: null })).toBe('<1 毫秒 · —')
    expect(operationTiming({ durationMs: 12.3, speedBps: null })).toBe('12 毫秒 · —')
  })
})
