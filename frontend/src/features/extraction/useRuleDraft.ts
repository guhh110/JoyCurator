import { computed, ref } from 'vue'
import { pipelineRules, type Rule, type Pipeline } from './model'
export interface RuleDraft {
  key: string
  rule: Rule
  paramsText: string
  examplesText: string
  constantText: string
  constant: boolean
}
let nextKey = 0
export function createRuleDraft(rule: Rule): RuleDraft {
  return {
    key: `draft-${++nextKey}`,
    rule: JSON.parse(JSON.stringify(rule)),
    paramsText: JSON.stringify(rule.config.params ?? {}, null, 2),
    examplesText: JSON.stringify(rule.examples, null, 2),
    constantText: JSON.stringify(rule.config.value ?? 'tv'),
    constant: Object.hasOwn(rule.config, 'value'),
  }
}
export function draftRule(d: RuleDraft): Rule {
  const r: Rule = JSON.parse(JSON.stringify(d.rule))
  if (!r.id.trim() || !r.name.trim()) throw Error('规则 ID 和名称不能为空')
  r.examples = JSON.parse(d.examplesText)
  if (
    !Array.isArray(r.examples) ||
    r.examples.some(
      (e) => !Array.isArray(e.paths) || !Object.hasOwn(e, 'expected'),
    )
  )
    throw Error('样例需包含 paths 和 expected')
  if (r.kind === 'script') {
    const params = JSON.parse(d.paramsText)
    if (!params || Array.isArray(params) || typeof params !== 'object')
      throw Error('参数必须为 JSON 对象')
    if (!r.config.code?.trim()) throw Error('脚本不能为空')
    r.config = { code: r.config.code, params }
  } else {
    if (!r.config.pattern?.trim()) throw Error('正则不能为空')
    if (!d.constant && !r.config.capture?.trim())
      throw Error('请填写命名捕获组')
    r.config = d.constant
      ? { pattern: r.config.pattern, value: JSON.parse(d.constantText) }
      : { pattern: r.config.pattern, capture: r.config.capture }
  }
  return r
}
export function asPipeline(rules: Rule[]): Pipeline {
  const tokenizer = rules.find((r) => r.stage === 'tokenize'),
    resolver = rules.find((r) => r.stage === 'resolve')
  if (!tokenizer || !resolver) throw Error('分词和裁决规则不可缺少')
  return {
    tokenizer,
    annotators: rules.filter((r) => r.stage === 'annotate'),
    resolver,
  }
}
export function withRule(p: Pipeline, rule: Rule, originalId = ''): Pipeline {
  const rules: Rule[] = JSON.parse(JSON.stringify(pipelineRules(p))),
    i = rules.findIndex((r) => r.id === originalId)
  if (i >= 0 && (rules[i]!.target === 'title') === (rule.target === 'title'))
    rules[i] = rule
  else {
    if (i >= 0) rules.splice(i, 1)
    const beforeTitle = rules.findIndex(
      (r) => r.stage === 'annotate' && r.target === 'title',
    )
    const at =
      rule.target !== 'title' && beforeTitle >= 0
        ? beforeTitle
        : rules.length - 1
    rules.splice(at, 0, rule)
  }
  rules
    .filter((r) => r.stage === 'annotate')
    .forEach((r, i) => (r.order = (i + 1) * 10))
  return asPipeline(rules)
}
export function draftFingerprint(drafts: RuleDraft[]): string {
  return JSON.stringify(drafts.map(({ key: _, ...d }) => d))
}
export function trialFingerprint(
  chain: string,
  path: string,
  root: string,
): string {
  return JSON.stringify([chain, path, root])
}
export function canMoveRule(
  drafts: RuleDraft[],
  key: string,
  offset: -1 | 1,
): boolean {
  const i = drafts.findIndex((d) => d.key === key),
    a = drafts[i]?.rule,
    b = drafts[i + offset]?.rule
  return (
    a?.stage === 'annotate' &&
    b?.stage === 'annotate' &&
    (a.target === 'title') === (b.target === 'title')
  )
}
export function useRuleDraft() {
  const drafts = ref<RuleDraft[]>([]),
    selectedKey = ref(''),
    saved = ref('')
  const fingerprint = computed(() => draftFingerprint(drafts.value)),
    dirty = computed(() => fingerprint.value !== saved.value)
  const validation = computed(() => {
    const rules: Rule[] = [],
      errors: { key: string; name: string; message: string }[] = [],
      ids = new Set<string>()
    for (const d of drafts.value) {
      try {
        const r = draftRule(d)
        if (ids.has(r.id)) throw Error('规则 ID 重复')
        ids.add(r.id)
        rules.push(r)
      } catch (e) {
        errors.push({
          key: d.key,
          name: d.rule.name,
          message: (e as Error).message,
        })
      }
    }
    return {
      rules,
      errors,
      pipeline: errors.length || !rules.length ? undefined : asPipeline(rules),
    }
  })
  function load(p: Pipeline, markSaved = true) {
    drafts.value = pipelineRules(p).map(createRuleDraft)
    if (markSaved) saved.value = fingerprint.value
  }
  function normalize() {
    drafts.value
      .filter((d) => d.rule.stage === 'annotate')
      .forEach((d, i) => (d.rule.order = (i + 1) * 10))
  }
  function move(key: string, offset: -1 | 1) {
    const i = drafts.value.findIndex((d) => d.key === key),
      j = i + offset
    if (!canMoveRule(drafts.value, key, offset)) return
    ;[drafts.value[i], drafts.value[j]] = [drafts.value[j]!, drafts.value[i]!]
    normalize()
  }
  function remove(key: string) {
    drafts.value = drafts.value.filter(
      (d) => d.key !== key || d.rule.stage !== 'annotate',
    )
    normalize()
  }
  return {
    drafts,
    selectedKey,
    editing: computed(() =>
      drafts.value.find((d) => d.key === selectedKey.value),
    ),
    normalize,
    dirty,
    fingerprint,
    validation,
    load,
    move,
    remove,
    markSaved: () => (saved.value = fingerprint.value),
  }
}
