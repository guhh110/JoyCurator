import { describe, expect, it } from 'vitest'
import { sourceOf, sourceSummaries } from '../../../frontend/src/recognition'
import type { RecognitionItem } from '../../../frontend/src/api'
const item = (path: string, group: string, state: RecognitionItem['state'], removed = false) =>
  ({ path, group, state, removed }) as RecognitionItem
describe('recognition sources', () => {
  it('attributes a file to the longest added path that contains it', () => {
    const roots = ['/media/inbox', '/media/inbox/4k/', '/media/other']
    expect(sourceOf('/media/inbox/4k/a.mkv', roots)).toBe('/media/inbox/4k')
    expect(sourceOf('/media/inbox/b.mkv', roots)).toBe('/media/inbox')
    expect(sourceOf('/media/inbox-2/c.mkv', roots)).toBe('')
    expect(sourceOf('/media/other', roots)).toBe('/media/other')
  })
  it('summarizes files, works and states per source in added order, skipping removed files', () => {
    const roots = ['/b', '/a']
    const summaries = sourceSummaries(
      [
        item('/a/1.mkv', 'g1', 'ready'),
        item('/a/2.mkv', 'g1', 'review'),
        item('/a/3.mkv', 'g2', 'ready'),
        item('/a/4.mkv', 'g2', 'ready', true),
      ],
      roots,
    )
    expect(summaries.map((s) => s.path)).toEqual(['/b', '/a'])
    expect(summaries[0]).toMatchObject({ files: 0, works: 0 })
    expect(summaries[1]).toMatchObject({ files: 3, works: 2, states: { ready: 2, review: 1 } })
  })
})
