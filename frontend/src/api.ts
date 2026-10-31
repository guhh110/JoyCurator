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
  state: 'pending' | 'completed' | 'failed' | 'unavailable'
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
export interface ScrapePreview {
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
  traceId?: string
  group: string
  entry: Entry | null
  candidates: Entry[]
  state: 'pending' | 'ready' | 'review' | 'unmatched' | 'failed'
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
}
export interface BatchPreview extends Preview {
  groups: (Preview & { type: string; tmdbId: number })[]
}
let requestEpoch = 0
export function invalidateRequests() {
  requestEpoch++
}
export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}
export async function api<T>(
  path: string,
  body?: unknown,
  method = body === undefined ? 'GET' : 'POST',
  options: Pick<RequestInit, 'signal'> = {},
): Promise<T> {
  const epoch = requestEpoch
  const response = await fetch(`/api${path}`, {
    ...options,
    credentials: 'same-origin',
    method,
    ...(body === undefined
      ? {}
      : {
          method,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        }),
  })
  const value = await response.json().catch((error) => {
    if (options.signal?.aborted) throw error
    return { message: '服务暂时不可用，请检查后端是否启动' }
  })
  if (epoch !== requestEpoch)
    throw new DOMException('登录状态已变化，忽略过期响应', 'AbortError')
  if (!response.ok) {
    if (response.status === 401 && typeof window !== 'undefined')
      window.dispatchEvent(new Event('session-expired'))
    throw new ApiError(value.message || '请求失败', response.status)
  }
  return value
}
export function name(path: string) {
  return path.split(/[\\/]/).pop() || path
}
export function bytes(n: number) {
  return n < 1024
    ? `${n} B`
    : n < 1024 ** 2
      ? `${(n / 1024).toFixed(1)} KB`
      : n < 1024 ** 3
        ? `${(n / 1024 ** 2).toFixed(1)} MB`
        : `${(n / 1024 ** 3).toFixed(1)} GB`
}
export function parseEpisodes(text: string): number[] {
  const result: number[] = []
  for (const part of text
    .trim()
    .split(/[,，\s]+/)
    .filter(Boolean)) {
    const match = /^(\d+)(?:-(\d+))?$/.exec(part)
    if (!match) throw new Error('集数使用数字、逗号或范围，例如 1,3-5')
    const first = Number(match[1]),
      last = Number(match[2] || match[1])
    if (first < 0 || last > 9999 || last < first || last - first > 999)
      throw new Error('集数范围无效')
    for (let n = first; n <= last; n++) {
      if (result.includes(n) || result.length >= 1000)
        throw new Error('集数重复或超过 1000 集')
      result.push(n)
    }
  }
  if (!result.length) throw new Error('请填写集数')
  return result.sort((a, b) => a - b)
}
export function episodeCode(season: number, episodes: number[]) {
  return `S${String(season).padStart(2, '0')}${episodes.map((n) => `E${String(n).padStart(2, '0')}`).join(' · ')}`
}
