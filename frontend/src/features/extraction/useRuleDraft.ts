import { computed, ref } from 'vue'
import { fieldMajor, type Rule, type Source } from './model'

// Raw JSON stays with each rule so an unfinished edit survives selection and sorting.
export interface RuleDraft {
  key: string
  rule: Rule
  paramsText: string
  examplesText: string
  constantText: string
  constant: boolean
  sourceCode: string
}
let nextKey = 0
export function createRuleDraft(
  rule: Rule,
  sourceScripts: Partial<Record<Source, string>> = {},
): RuleDraft {
  return {
    key: `draft-${++nextKey}`,
    rule: JSON.parse(JSON.stringify(rule)),
    paramsText: JSON.stringify(rule.config.params ?? {}, null, 2),
    examplesText: JSON.stringify(rule.examples, null, 2),
    constantText: JSON.stringify(rule.config.value ?? 'tv'),
    constant: Object.hasOwn(rule.config, 'value'),
    sourceCode: rule.config.sourceCode ?? sourceScripts.cleanFilename ?? '',
  }
}
export function draftRule(draft: RuleDraft): Rule {
  const rule: Rule = JSON.parse(JSON.stringify(draft.rule))
  if (!rule.id.trim() || !rule.name.trim())
    throw new Error('规则 ID 和名称不能为空')
  rule.examples = JSON.parse(draft.examplesText)
  if (!Array.isArray(rule.examples)) throw new Error('测试样例必须是 JSON 数组')
  for (const item of rule.examples) {
    if (
      !item ||
      typeof item.path !== 'string' ||
      !Object.hasOwn(item, 'expected')
    )
      throw new Error('每个样例需填写 path 和 expected，未命中请用 null')
  }
  if (rule.kind === 'script') {
    const params: unknown = JSON.parse(draft.paramsText)
    if (!params || typeof params !== 'object' || Array.isArray(params))
      throw new Error('脚本参数必须是 JSON 对象')
    if (!rule.config.code?.trim()) throw new Error('脚本不能为空')
    rule.config = {
      code: rule.config.code,
      params: params as Record<string, unknown>,
    }
  } else {
    if (!rule.config.pattern?.trim()) throw new Error('正则表达式不能为空')
    if (!draft.constant && !rule.config.capture?.trim())
      throw new Error('请填写命名捕获组')
    rule.config = draft.constant
      ? { pattern: rule.config.pattern, value: JSON.parse(draft.constantText) }
      : { pattern: rule.config.pattern, capture: rule.config.capture }
  }
  if (rule.source === 'cleanFilename') {
    if (!draft.sourceCode.trim()) throw new Error('来源清理脚本不能为空')
    rule.config.sourceCode = draft.sourceCode
  }
  return rule
}
// Puts one edited rule into a saved chain (replacing the rule that had
// `originalId`, or appending a new one) and renumbers the chain field by field.
export function withRule(chain: Rule[], rule: Rule, originalId = ''): Rule[] {
  const copy: Rule[] = JSON.parse(JSON.stringify(chain))
  const index = originalId ? copy.findIndex((item) => item.id === originalId) : -1
  if (index >= 0) copy.splice(index, 1, rule)
  else copy.push(rule)
  const ordered = fieldMajor(
    copy.sort((a, b) => a.order - b.order),
    (item) => item.target,
  )
  ordered.forEach((item, position) => {
    item.order = (position + 1) * 10
  })
  return ordered
}
export function draftFingerprint(drafts: RuleDraft[]): string {
  return JSON.stringify(drafts.map(({ key: _, ...draft }) => draft))
}
export function trialFingerprint(
  chain: string,
  path: string,
  root: string,
): string {
  return JSON.stringify([chain, path, root])
}
export function useRuleDraft() {
  const drafts = ref<RuleDraft[]>([]),
    selectedKey = ref(''),
    saved = ref('')
  const editing = computed(() =>
    drafts.value.find((item) => item.key === selectedKey.value),
  )
  const fingerprint = computed(() => draftFingerprint(drafts.value))
  const dirty = computed(() => fingerprint.value !== saved.value)
  const validation = computed(() => {
    const ids = new Set<string>(),
      orders = new Set<number>()
    const rules: Rule[] = [],
      errors: { key: string; name: string; message: string }[] = []
    for (const draft of drafts.value) {
      try {
        const rule = draftRule(draft)
        if (ids.has(rule.id)) throw new Error('规则 ID 重复')
        ids.add(rule.id)
        if (
          !Number.isInteger(rule.order) ||
          rule.order < 0 ||
          orders.has(rule.order)
        )
          throw new Error('执行顺序必须是非负且不重复的整数')
        orders.add(rule.order)
        rules.push(rule)
      } catch (e) {
        errors.push({
          key: draft.key,
          name: draft.rule.name || '未命名规则',
          message: (e as Error).message,
        })
      }
    }
    return { rules, errors }
  })
  // Returns true when the stored chain was not field-major and had to be reordered.
  function load(
    rules: Rule[],
    scripts: Partial<Record<Source, string>>,
    markSaved = true,
  ) {
    const byOrder = [...rules].sort((a, b) => a.order - b.order)
    drafts.value = byOrder.map((rule) => createRuleDraft(rule, scripts))
    const layout = () =>
      JSON.stringify(drafts.value.map((item) => [item.key, item.rule.order]))
    const before = layout()
    normalize()
    selectedKey.value = ''
    if (markSaved) saved.value = fingerprint.value
    return before !== layout()
  }
  // Group drafts by field and number them sequentially; call after any change
  // to a rule's target or to the list itself.
  function normalize() {
    drafts.value = fieldMajor(drafts.value, (item) => item.rule.target)
    drafts.value.forEach((draft, index) => {
      draft.rule.order = (index + 1) * 10
    })
  }
  // Moves a rule one step within its own field; crossing fields is not allowed.
  function move(key: string, offset: -1 | 1) {
    const index = drafts.value.findIndex((item) => item.key === key)
    const next = index + offset
    const draft = drafts.value[index],
      other = drafts.value[next]
    if (!draft || !other || other.rule.target !== draft.rule.target) return
    drafts.value.splice(index, 1)
    drafts.value.splice(next, 0, draft)
    normalize()
  }
  function siblings(key: string) {
    const draft = drafts.value.find((item) => item.key === key)
    const group = drafts.value.filter(
      (item) => item.rule.target === draft?.rule.target,
    )
    const position = group.findIndex((item) => item.key === key)
    return { first: position === 0, last: position === group.length - 1 }
  }
  function add(draft: RuleDraft) {
    drafts.value.push(draft)
    normalize()
    selectedKey.value = draft.key
  }
  function remove(key: string) {
    const index = drafts.value.findIndex((item) => item.key === key)
    if (index < 0) return
    drafts.value.splice(index, 1)
    if (selectedKey.value === key) selectedKey.value = ''
    normalize()
  }
  function markSaved() {
    saved.value = fingerprint.value
  }
  return {
    drafts,
    selectedKey,
    editing,
    fingerprint,
    dirty,
    validation,
    load,
    normalize,
    move,
    siblings,
    add,
    remove,
    markSaved,
  }
}
