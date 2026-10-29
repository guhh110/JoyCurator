export type Target =
  | 'tmdbId'
  | 'title'
  | 'mediaType'
  | 'year'
  | 'season'
  | 'episodes'
  | 'releaseTag'
export type Source =
  | 'filename'
  | 'cleanFilename'
  | 'parentDirectory'
  | 'directories'
  | 'relativePath'
  | 'absolutePath'
export interface Rule {
  id: string
  name: string
  description?: string
  target: Target
  source: Source
  order: number
  enabled: boolean
  kind: 'script' | 'regex'
  overwrite: boolean
  config: {
    code?: string
    sourceCode?: string
    params?: Record<string, unknown>
    pattern?: string
    capture?: string
    value?: unknown
  }
  examples: { path: string; root?: string; expected: unknown }[]
}
export interface Metadata {
  tmdbId: number | null
  title: string | null
  mediaType: 'movie' | 'tv' | null
  year: number | null
  season: number | null
  episodes: number[]
  releaseTag: string | null
}
export interface Step {
  ruleId: string
  ruleName: string
  order: number
  ruleConfig: Rule
  target: Target
  inputs: string[]
  attempts: {
    input: string
    matched: boolean
    matchedText?: string
    captures?: Record<string, string>
    error?: string
    trace?: ScriptEvent[]
  }[]
  status: string
  matchedText: string
  captures: Record<string, string>
  extractedValue: unknown
  before: unknown
  after: unknown
  applied: boolean
  reason: string
  startedAt: string
  durationMs: number
}
export interface ExtractionResult {
  path?: string
  root?: string
  metadata: Metadata
  steps: Step[]
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
export interface Catalog {
  id: string
  name: string
  target: Target
  description: string
  sources: Source[]
  config: Rule['config']
}
export interface RuleLibrary {
  rules: Rule[]
  defaults: Rule[]
  catalog: Catalog[]
  targets: Target[]
  sources: Source[]
  sourceScripts: Partial<Record<Source, string>>
  scriptLanguage: string
}
export interface Trace {
  id: string
  path: string
  root: string
  status: string
  startedAt: string
  finishedAt?: string
  metadata: Metadata
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
  releaseTag: '发布标签',
}
export const sources: Record<Source, string> = {
  filename: '文件名',
  cleanFilename: '清理后文件名',
  parentDirectory: '父目录',
  directories: '逐级父目录（近到远）',
  relativePath: '边界内相对路径',
  absolutePath: '绝对路径',
}
export const statuses: Record<string, string> = {
  matched: '命中',
  not_matched: '未命中',
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
export function moveRule(rules: Rule[], from: number, to: number) {
  if (from < 0 || to < 0 || from >= rules.length || to >= rules.length) return
  const [item] = rules.splice(from, 1)
  if (item) rules.splice(to, 0, item)
  rules.forEach((rule, index) => {
    rule.order = (index + 1) * 10
  })
}

export const emptyScript = `function extract(ctx) {
  trace("输入文本", ctx.text)
  // 返回本规则的目标值；未命中时返回 null。
  return null
}
`
