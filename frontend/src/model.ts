import type { Metadata } from './features/extraction/model'
export interface MediaFile {
  metadata: Metadata
  path: string
  size: number
  mtime: string
  season: number
  episodes: number[]
}
export interface Entry {
  tmdbId: number
  type: 'tv' | 'movie'
  title: string
  originalTitle: string
  year: number
  overview: string
  posterPath: string
}
export interface Season {
  number: number
  name: string
  count: number
}
export interface Episode {
  number: number
  name: string
  season: number
  airDate: string
}
export interface Category {
  name: string
  type?: string
  countries: string[]
  genres: string[]
  yearMin?: number
  yearMax?: number
}
export interface Settings {
  version: number
  sourceRoot: string
  targetRoot: string
  proxyURL: string
  language: string
  hasToken: boolean
  roots: string[]
  rules: {
    movieFolder: string
    movieFile: string
    tvFolder: string
    tvFile: string
    categories: Category[]
  }
}
export interface PreviewRow {
  source: string
  target: string
  season: number
  episodes: number[]
  artifacts: string[]
  issues: string[]
  action: 'create' | 'skip' | 'overwrite'
}
export type DuplicatePolicy = 'error' | 'skip' | 'overwrite'
export interface Preview {
  id: string
  title: string
  mode: string
  duplicatePolicy: DuplicatePolicy
  rows: PreviewRow[]
  required: number
  issues: string[]
}
export interface Operation {
  kind: string
  source: string
  target: string
  status: string
  error: string
  owner: string
  replaceIdentity?: string
}
export interface Run {
  id: string
  title: string
  mode: string
  state: string
  created: string
  error: string
  completed: number
  total: number
  undo: boolean
  ops?: Operation[]
  batchId?: string
  purpose: 'organize' | 'scrape'
}
export interface MediaMapping {
  path: string
  source: string
  type: 'movie' | 'tv'
  tmdbId: number
  season: number
  episodes: number[]
  state: 'pending' | 'completed' | 'failed' | 'unavailable' | 'queued' | 'running'
}
export interface ScrapeWork {
  type: 'movie' | 'tv'
  tmdbId: number
  representative: string
  total: number
  pending: number
  failed: number
  completed: number
}
export interface ScrapeWorkPage {
  items: ScrapeWork[]
  nextCursor: string
}
export type ScrapeContent = 'all' | 'nfo' | 'images'
export interface ScrapePreview {
  content: ScrapeContent
  onlyMissing: boolean
  skipped: string[]
  id: string
  title: string
  type: string
  tmdbId: number
  targets: string[]
  artifacts: string[]
  issues: string[]
  required: number
}
export interface RecognitionItem extends MediaFile {
  execution?: FileExecution
  traceId?: string
  group: string
  entry: Entry | null
  candidates: Entry[]
  state: 'pending' | 'reidentifying' | 'ready' | 'review' | 'unmatched' | 'failed'
  reason: string
  selected: boolean
  removed: boolean
}
export interface Recognition {
  id: string
  revision: number
  state: string
  paths: string[]
  items: RecognitionItem[]
  completed: number
  total: number
  error: string
  created: string
}
export interface Batch {
  id: string
  runs: Run[]
  active: boolean
  state: string
  position: number
}
export interface BatchPreview extends Preview {
  groups: (Preview & { type: string; tmdbId: number })[]
}

export interface RunPage { items: Run[]; total: number; page: number }

export interface FileExecution {runId: string; state: string; locked: boolean; completed: boolean; manualReview: boolean}
export interface LaneStatus {current: Run | null; queued: number; queuedBatches: number; waitReason: string}
export interface QueueStatus {active: boolean; recognizing: boolean; revision: number; organize: LaneStatus; scrape: LaneStatus}
