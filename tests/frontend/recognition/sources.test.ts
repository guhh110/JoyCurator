import { describe, expect, it } from 'vitest'
import { sourceFolder, sourceOf, sourceSummaries } from '../../../frontend/src/recognition'
import type { RecognitionItem } from '../../../frontend/src/model'
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
  it('shows a file folder starting from the added source name', () => {
    const roots = ['/media/inbox/anime', '/media/one.mkv']
    expect(sourceFolder('/media/inbox/anime/Titan/01.mkv', roots)).toBe('anime/Titan')
    expect(sourceFolder('/media/inbox/anime/02.mkv', roots)).toBe('anime')
    expect(sourceFolder('/media/one.mkv', roots)).toBe('')
    expect(sourceFolder('/elsewhere/x.mkv', roots)).toBe('')
  })
  it('counts completed files against their owning source while omitting them from pending totals', () => {
    const done = { ...item('/source/one.mp4', 'g1', 'ready', true), execution: { runId: 'done', state: 'completed', locked: false, completed: true, manualReview: false } }
    expect(sourceSummaries([done], ['/source', done.path])).toMatchObject([
      { path: '/source', files: 0, completed: 0 },
      { path: done.path, files: 0, completed: 1 },
    ])
  })
})
