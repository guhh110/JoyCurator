import type { EpisodeBasis } from '../../model'
import type { Document, FileResult, Candidate } from '../extraction/model'
export interface CandidateView { path: string; traceId: string; document: Document; suggestion: FileResult }
export const manualBasis = (): EpisodeBasis => ({ kind: 'manual', index: 0, offset: 0 })
export function digitParts(text: string) {
 const parts: { text: string; index: number; start: number; end: number }[] = []
 for (const match of text.matchAll(/[0-9]+/g)) parts.push({ text: match[0], index: parts.length, start: match.index!, end: match.index! + match[0].length })
 return parts
}
export function digitPattern(text: string) { return JSON.stringify(text.split(/[0-9]+/g)) }
export function episodeCandidates(view: CandidateView, ruleId: string, sourceId: string) {
 return view.document.candidates.filter(c => c.role === 'episodes' && c.ruleId === ruleId && c.spans.some(s => s.sourceId === sourceId))
  .sort((a, b) => Math.min(...a.spans.filter(s => s.sourceId === sourceId).map(s => s.start)) - Math.min(...b.spans.filter(s => s.sourceId === sourceId).map(s => s.start)) || a.id.localeCompare(b.id))
}
export function episodeValues(candidate?: Candidate): number[] {
 return candidate?.status === 'valid' && Array.isArray(candidate.value) && candidate.value.length && candidate.value.every(n => Number.isSafeInteger(n) && n >= 0) ? [...candidate.value] : []
}
export function takeDigits(view: CandidateView, sourceId: string, index: number, pattern: string, offset: number) {
 const source = view.document.sources.find(s => s.id === sourceId)
 if (!source || digitPattern(source.text) !== pattern) throw new Error('原文数字结构不一致')
 const part = digitParts(source.text)[index]
 if (!part) throw new Error('缺少所选数字')
 const number = Number(part.text) + offset
 if (!Number.isSafeInteger(offset) || !Number.isSafeInteger(number) || number < 0) throw new Error('偏移后集号无效')
 return { episodes: [number], basis: { kind: 'digits', traceId: view.traceId, sourceId, index, pattern, offset } as EpisodeBasis, original: part.text }
}
