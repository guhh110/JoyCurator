export type Target =
  | 'tmdbId'
  | 'title'
  | 'mediaType'
  | 'year'
  | 'season'
  | 'episodes'
export type Role = Target
export type Source = 'filename' | 'directories' | 'all'
export type Stage = 'tokenize' | 'annotate' | 'resolve'
export interface Rule {
  id: string
  name: string
  description?: string
  stage: Stage
  target?: Role
  source?: Source
  order: number
  enabled: boolean
  kind: 'script' | 'regex'
  config: {
    code?: string
    params?: Record<string, unknown>
    pattern?: string
    capture?: string
    value?: unknown
  }
  examples: {
    paths: string[]
    root: string
    targets: string[]
    expected: unknown
  }[]
}
export interface Pipeline {
  tokenizer: Rule
  annotators: Rule[]
  resolver: Rule
}
export interface Metadata {
  tmdbId: number | null
  title: string | null
  mediaType: 'movie' | 'tv' | null
  year: number | null
  season: number | null
  episodes: number[]
}
export interface Span {
  sourceId: string
  start: number
  end: number
}
export interface Token extends Span {
  id: string
  parentId: string | null
  kind: 'text' | 'number' | 'separator' | 'group'
  text: string
}
export interface Candidate {
  id: string
  ruleId: string
  role: Role
  value: unknown
  spans: Span[]
  strength: 'explicit' | 'contextual' | 'heuristic'
  status: 'valid' | 'invalid'
  reason: string
}
export interface Document {
  fileId: string
  path: string
  directory: string
  sources: { id: string; kind: Source; text: string; depth: number }[]
  tokens: Token[]
  candidates: Candidate[]
}
export interface FileResult {
  fileId: string
  path: string
  metadata: Metadata
  decisions: Partial<
    Record<
      Target,
      { candidates: { fileId: string; candidateId: string }[]; reason: string }
    >
  >
}
export interface ScriptEvent {
  phase: string
  line: number
  column: number
  function?: string
  operation: string
  message: string
  data?: unknown
}
export interface Step {
  ruleId: string
  ruleName: string
  stage: Stage
  fileId: string
  order: number
  ruleConfig: Rule
  target?: Role
  inputs: string[]
  attempts: {
    input: string
    matched: boolean
    error?: string
    trace?: ScriptEvent[]
  }[]
  status: string
  extractedValue: unknown
  before: unknown
  after: unknown
  applied: boolean
  reason: string
  startedAt: string
  durationMs: number
}
export interface ExtractionResult {
  context: {
    contextRoot: string
    sourceBoundary: string
    paths: string[]
    targets: string[]
  }
  tree: { path: string; parent: string; fileIds: string[] }[]
  documents: Document[]
  files: FileResult[]
  steps: Step[]
  error?: string
}
export type Catalog = Rule
export interface RuleLibrary {
  pipeline: Pipeline
  defaults: Pipeline
  catalog: Rule[]
  targets: Role[]
  sources: Source[]
  scriptLanguage: string
}
export interface Trace {
  id: string
  path: string
  root: string
  status: string
  startedAt: string
  finishedAt?: string
  error?: string
  stepCount: number
}
export const targets: Record<Target, string> = {
  tmdbId: 'TMDB ID',
  title: '片名',
  mediaType: '媒体类型',
  year: '年份',
  season: '季号',
  episodes: '集数',
}
export const roles: Record<Role, string> = { ...targets }
export const stages: Record<Stage, string> = {
  tokenize: '分词',
  annotate: '候选标注',
  resolve: '建议脚本',
}
export const sources: Record<Source, string> = {
  filename: '文件名',
  directories: '父目录',
  all: '全部来源',
}
export const statuses: Record<string, string> = {
  matched: '完成',
  disabled: '禁用',
  skipped: '跳过',
  error: '错误',
}
export function valueText(value: unknown): string {
  return value == null
    ? '未知'
    : typeof value === 'string'
      ? value
      : JSON.stringify(value)
}
export function pipelineRules(p: Pipeline): Rule[] {
  return [
    p.tokenizer,
    ...[...p.annotators].sort(
      (a, b) =>
        Number(a.target === 'title') - Number(b.target === 'title') ||
        a.order - b.order,
    ),
    p.resolver,
  ]
}
export const emptyScript = `function extract(ctx) {\n  trace("片段数量", ctx.tokens.length)\n  return []\n}\n`
