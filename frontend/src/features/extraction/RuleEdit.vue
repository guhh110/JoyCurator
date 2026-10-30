<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { useRouter } from 'vue-router'
import { ArrowLeft, CircleHelp, Play, Save } from '@lucide/vue'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import { keys } from '../../app/query'
import Tooltip from '../../components/ui/Tooltip.vue'
import { api } from '../../api'
import { useRuleLibrary } from './queries'
import type { Rule, ScriptEvent, Target } from './model'
import { emptyScript, fieldOrder, targets, valueText } from './model'
import RuleEditor from './RuleEditor.vue'
import ExtractionResult from './ExtractionResult.vue'
import { useExtraction } from './useExtraction'
import { useUnsavedChanges } from './useUnsavedChanges'
import { draftScriptDiagnostics, runtimeDiagnostics, type ScriptDiagnostic } from './scriptDiagnostics'
import { useRuleExamples } from './useRuleExamples'
import RuleExamples from './RuleExamples.vue'
import { useToastFeedback } from '../../app/toast'
import {
  createRuleDraft,
  draftFingerprint,
  draftRule,
  withRule,
  type RuleDraft,
} from './useRuleDraft'

// One rule, edited on its own page. Trial and save both use the saved chain
// with this rule swapped in, so the trial shows exactly what saving will do.
const props = defineProps<{
  ruleId: string
  target?: string
  kind?: string
  template?: string
  line?: string
}>()
const router = useRouter(),
  client = useQueryClient()
const libraryQuery = useRuleLibrary()
const library = libraryQuery.data
const draft = ref<RuleDraft>()
const saved = ref('')
const originalId = ref('')
const error = ref(''),
  notice = ref('')
useToastFeedback(error, notice)
const focus = ref<{ phase: string; line: number; request: number }>()
let focusRequest = 0
const creating = computed(() => props.ruleId === 'new')
let loadedId: string | undefined

function blankRule(): Rule {
  const target = (
    fieldOrder.includes(props.target as Target) ? props.target : 'title'
  ) as Target
  const descriptor = props.template
    ? library.value?.catalog.find((item) => item.id === props.template)
    : undefined
  const kind = props.kind === 'regex' ? 'regex' : 'script'
  return {
    id: `rule-${Date.now().toString(36)}`,
    name: descriptor?.name ?? (kind === 'script' ? '新脚本规则' : '新正则规则'),
    description: descriptor?.description,
    target,
    source: descriptor?.sources[0] ?? 'filename',
    order: 0,
    enabled: true,
    kind,
    overwrite: false,
    config: descriptor
      ? JSON.parse(JSON.stringify(descriptor.config))
      : kind === 'script'
        ? { code: emptyScript, params: {} }
        : { pattern: '(?P<value>.+)', capture: 'value' },
    examples: [],
  }
}
watch(
  [library, () => props.ruleId],
  ([value]) => {
    if (!value || loadedId === props.ruleId) return
    loadedId = props.ruleId
    error.value = ''
    notice.value = ''
    focus.value = undefined
    const existing = creating.value
      ? undefined
      : value.rules.find((item) => item.id === props.ruleId)
    if (!creating.value && !existing) {
      error.value = `规则「${props.ruleId}」不存在，可能已被删除`
      draft.value = undefined
      return
    }
    draft.value = createRuleDraft(existing ?? blankRule(), value.sourceScripts)
    originalId.value = existing?.id ?? ''
    saved.value = creating.value ? '' : draftFingerprint([draft.value])
    const line = Number(props.line)
    if (line > 0) focus.value = { phase: 'extract', line, request: ++focusRequest }
  },
  { immediate: true },
)
const dirty = computed(
  () => !!draft.value && draftFingerprint([draft.value]) !== saved.value,
)
const syntaxProblems = computed(() => draft.value ? draftScriptDiagnostics(draft.value) : [])
const invalid = computed(() => {
  if (!draft.value) return ''
  try {
    draftRule(draft.value)
    if (syntaxProblems.value.length) return syntaxProblems.value[0]!.message
    return ''
  } catch (e) {
    return (e as Error).message
  }
})
// The chain the trial and the save both use: saved rules + this draft.
const chain = computed(() => {
  if (!library.value || !draft.value || invalid.value) return undefined
  return withRule(library.value.rules, draftRule(draft.value), originalId.value)
})

const { path, root, busy: testing, result, error: testError, run, cancel } =
  useExtraction()
const trialKey = ref('')
const currentKey = computed(() =>
  JSON.stringify([chain.value, path.value, root.value]),
)
const stale = computed(() => !!result.value && trialKey.value !== currentKey.value)
const ownStep = computed(() =>
  result.value?.steps.find((step) => step.ruleId === draft.value?.rule.id),
)
const runtimeProblems = computed(() => stale.value ? [] : runtimeDiagnostics(ownStep.value))
const problems = computed(() => [...syntaxProblems.value, ...runtimeProblems.value])
const { results: exampleResults, checkedKey, busy: checking, stale: examplesStale, run: checkExamples, reset: resetExamples } = useRuleExamples(() => chain.value)
watch(() => props.ruleId, () => {
  cancel(true)
  result.value = null
  path.value = ''
  root.value = ''
  testError.value = ''
  resetExamples()
})
async function trial() {
  if (!chain.value || testing.value) return
  if (!path.value.trim()) {
    testError.value = ''
    testError.value = '请先填写完整文件路径'
    return
  }
  trialKey.value = currentKey.value
  await run(chain.value)
}

const saveMutation = useMutation({
  mutationFn: (rules: Rule[]) => api('/extraction/rules', { rules }, 'PUT'),
})
const saving = saveMutation.isPending
const submitting = ref(false)
async function save() {
  if (!chain.value || !draft.value || submitting.value || checking.value) return
  submitting.value = true
  error.value = ''
  notice.value = ''
  const rules = JSON.parse(JSON.stringify(chain.value)) as Rule[]
  const fingerprint = draftFingerprint([draft.value])
  try {
    if (!await checkExamples(rules)) return
    if (!draft.value || draftFingerprint([draft.value]) !== fingerprint) {
      error.value = '验证期间规则已修改，请重新保存。'
      return
    }
    await saveMutation.mutateAsync(rules)
    client.setQueryData(keys.ruleLibrary, { ...library.value!, rules })
    void client.invalidateQueries({ queryKey: keys.ruleLibrary })
    const id = draft.value.rule.id
    saved.value = fingerprint
    originalId.value = id
    notice.value = '已保存并生效'
    if (creating.value || id !== props.ruleId)
      await router.replace({ name: 'recognition-rule-edit', params: { ruleId: id } })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    submitting.value = false
  }
}
function locate(_id: string, event?: ScriptEvent) {
  focus.value = {
    phase: event?.phase ?? 'extract',
    line: event?.line ?? 1,
    request: ++focusRequest,
  }
}
function locateProblem(problem: ScriptDiagnostic) {
  focus.value = { phase: problem.phase, line: problem.line, request: ++focusRequest }
}
useUnsavedChanges(dirty, '这条规则')
</script>
<template>
  <section class="rule-edit-page" aria-label="编辑识别规则">
    <PageHeader
      :title="draft?.rule.name || (creating ? '新增规则' : '编辑规则')"
      :eyebrow="draft ? `${targets[draft.rule.target]} · 识别规则` : '识别规则'"
      description="改代码、试跑、保存，都在这一页完成。"
      ><template #actions
        ><button
          class="button"
          type="button"
          @click="router.push({ name: 'recognition-rules' })"
        >
          <ArrowLeft :size="14" />返回规则列表</button
        ><span class="save-status" :class="{ dirty }" role="status"
          ><i />{{
            checking ? '正在验证样例…' : saving ? '正在保存…' : dirty ? '有未保存修改' : '已保存'
          }}</span
        ><button
          class="button primary"
          type="button"
          :disabled="!chain || !dirty || submitting || checking"
          @click="save"
        >
          <Save :size="14" />{{ submitting ? checking ? '验证中…' : '保存中…' : '保存并生效' }}
        </button></template
      ></PageHeader
    >
    <QueryState
      :loading="libraryQuery.isPending.value"
      :error="libraryQuery.error.value"
      @retry="libraryQuery.refetch()"
    >
      <div v-if="!draft && error" class="empty">规则不可用，请返回规则列表。</div>
      <div v-if="draft" class="edit-layout">
        <section class="pane editor-pane">
          <fieldset class="editor-fields" :disabled="submitting">
            <RuleEditor v-model="draft" :focus="focus" :diagnostics="problems" :disabled="submitting" />
          </fieldset>
        </section>
        <aside class="pane trial-pane" aria-label="试跑这条规则">
          <form class="trial-form" @submit.prevent="trial">
            <div class="trial-heading">
              <label for="rule-trial-path">用这条规则试跑</label>
              <Tooltip content="使用已生效的规则链，并把这条规则换成当前草稿，结果与保存后一致。">
                <button class="help-button" type="button" aria-label="试跑说明"><CircleHelp :size="15" /></button>
              </Tooltip>
            </div>
            <input
              id="rule-trial-path"
              v-model="path"
              placeholder="/source/Show/Season 2/03.mkv"
              autocomplete="off"
              spellcheck="false"
            />
            <input
              v-model="root"
              placeholder="解析边界（可选，留空按源目录）"
              aria-label="解析边界"
              autocomplete="off"
            />
            <div class="trial-actions">
              <button v-if="testing" class="button" type="button" @click="cancel()">
                取消
              </button>
              <button class="button" type="button" :disabled="!chain || checking || submitting" @click="chain && checkExamples(chain)">验证全部样例</button>
              <button class="button primary" :disabled="!chain || testing || submitting">
                <Play :size="13" />{{ testing ? '试跑中…' : '试跑' }}
              </button>
            </div>
          </form>
          <RuleExamples :results="exampleResults" :checked="!!checkedKey" :busy="checking" :stale="examplesStale" />
          <div v-if="problems.length" class="script-problems" role="alert">
            <button v-for="(problem, index) in problems" :key="index" class="text-button" type="button" @click="locateProblem(problem)">
              {{ problem.phase === 'source' ? '来源脚本' : '脚本' }} L{{ problem.line }}:{{ problem.column }} · {{ problem.message }}
            </button>
          </div>
          <template v-if="result">
            <p v-if="stale" class="notice warning" role="status">
              代码或路径改过，下方是上次的结果。
            </p>
            <dl class="trial-summary">
              <dt>{{ targets[draft.rule.target] }}</dt>
              <dd>{{ valueText(result.metadata[draft.rule.target]) }}</dd>
              <dt>这条规则</dt>
              <dd>
                {{
                  ownStep
                    ? ownStep.status === 'error'
                      ? '执行失败'
                      : ownStep.applied
                      ? '命中并采用'
                      : ownStep.reason
                    : '未运行'
                }}
              </dd>
            </dl>
            <details :key="trialKey" class="trial-details">
              <summary>查看执行过程</summary>
              <ExtractionResult :result="result" view="steps" :rule-id="draft.rule.id" :editable="!stale" @locate="locate" />
            </details>
          </template>
        </aside>
      </div>
    </QueryState>
  </section>
</template>
<style scoped>
.rule-edit-page {
  display: grid;
  gap: 16px;
  min-width: 0;
}
.edit-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(320px, 1fr);
  gap: 16px;
  align-items: start;
}
.editor-pane {
  min-width: 0;
}
.editor-fields { margin:0; padding:0; border:0; min-width:0; }
.trial-heading { display:flex; align-items:center; justify-content:space-between; gap:8px; }
.help-button { display:inline-flex; padding:4px; border:0; background:transparent; color:var(--muted); cursor:help; }
.script-problems { display:grid; gap:8px; color:#b12525; font-size:12px; }
.script-problems button { text-align:left; overflow-wrap:anywhere; color:inherit; }
.trial-details > summary { cursor:pointer; font-size:12px; color:var(--purple); }
.trial-details > .extraction-output { margin-top:12px; }
.trial-pane {
  position: sticky;
  top: 0;
  display: grid;
  gap: 12px;
  padding: 16px;
  min-width: 0;
}
.trial-form {
  display: grid;
  gap: 8px;
}
.trial-form label {
  font-weight: 600;
  font-size: 13px;
}
.trial-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}
.trial-summary {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 6px 14px;
  margin: 0;
  font-size: 13px;
}
.trial-summary dt {
  color: var(--muted);
}
.trial-summary dd {
  margin: 0;
  font-family: 'SFMono-Regular', Consolas, monospace;
  overflow-wrap: anywhere;
}
.save-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--muted);
}
.save-status i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--green);
}
.save-status.dirty i {
  background: #d8973a;
}
@media (max-width: 960px) {
  .edit-layout {
    grid-template-columns: 1fr;
  }
  .trial-pane {
    position: static;
  }
}
</style>
