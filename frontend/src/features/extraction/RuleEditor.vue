<script setup lang="ts">
import { computed } from 'vue'
import ScriptCode from './ScriptCode.vue'
import ScriptReference from './ScriptReference.vue'
import Select from '../../components/ui/Select.vue'
import { roles, sources, emptyScript, type Role, type Source } from './model'
import type { RuleDraft } from './useRuleDraft'
import type { ScriptDiagnostic } from './scriptDiagnostics'
const draft = defineModel<RuleDraft>({ required: true })
defineProps<{
  focus?: { phase: string; line: number; request: number }
  diagnostics?: ScriptDiagnostic[]
  disabled?: boolean
}>()
const parameterNames = computed(() => {
  try {
    return Object.keys(JSON.parse(draft.value.paramsText))
  } catch {
    return []
  }
})
function kindChanged() {
  draft.value.rule.config =
    draft.value.rule.kind === 'regex'
      ? { pattern: '(?P<value>.+)', capture: 'value' }
      : { code: emptyScript }
  draft.value.paramsText = '{}'
}
</script>
<template>
  <div class="editor">
    <label>规则名称<input v-model="draft.rule.name" /></label
    ><template v-if="draft.rule.stage === 'annotate'"
      ><label
        >候选身份<Select
          label="候选身份"
          :model-value="draft.rule.target"
          :options="
            Object.entries(roles).map(([value, label]) => ({ value, label }))
          "
          @update:model-value="
            (value) => (draft.rule.target = value as Role)
          " /></label
      ><label
        >来源<Select
          label="来源"
          :model-value="draft.rule.source"
          :options="
            Object.entries(sources).map(([value, label]) => ({ value, label }))
          "
          @update:model-value="
            (value) => (draft.rule.source = value as Source)
          " /></label
      ><label
        >标注方式<Select
          label="标注方式"
          :model-value="draft.rule.kind"
          :options="[
            { value: 'script', label: 'JavaScript' },
            { value: 'regex', label: '正则' },
          ]"
          @update:model-value="
            (value) => {
              draft.rule.kind = value as 'script' | 'regex'
              kindChanged()
            }
          " /></label
    ></template>
    <template v-if="draft.rule.kind === 'script'"
      ><ScriptReference
        label="脚本"
        editor-id="rule-code"
        :stage="draft.rule.stage"
        ><ScriptCode
          id="rule-code"
          v-model="draft.rule.config.code!"
          :parameter-names="parameterNames"
          :disabled="disabled"
          :diagnostics="diagnostics"
          :active-line="focus?.line"
          :focus-key="focus?.request" /></ScriptReference
      ><label
        >参数 JSON<textarea
          v-model="draft.paramsText"
          rows="3"
        /></label></template
    ><template v-else
      ><label
        >Go 正则（逐词匹配）<input
          v-model="draft.rule.config.pattern"
          spellcheck="false" /></label
      ><label class="check"
        ><input v-model="draft.constant" type="checkbox" />固定输出值</label
      ><label v-if="draft.constant"
        >固定值 JSON<input v-model="draft.constantText" /></label
      ><label v-else
        >命名捕获组<input v-model="draft.rule.config.capture" /></label
    ></template>
    <details>
      <summary>样例</summary>
      <p class="tiny muted">
        paths 为完整目录快照，targets 为待裁决文件。expected
        比较本规则所有文件的输出数组。
      </p>
      <textarea
        v-model="draft.examplesText"
        rows="8"
        spellcheck="false"
        aria-label="样例 JSON"
      />
    </details>
    <details>
      <summary>规则 ID</summary>
      <input v-model="draft.rule.id" aria-label="规则 ID" />
    </details>
  </div>
</template>
<style scoped>
.editor {
  padding: 20px;
  display: grid;
  gap: 18px;
  min-width: 0;
}
.editor > label {
  display: grid;
  gap: 6px;
}
summary {
  cursor: pointer;
  margin-bottom: 10px;
}
textarea {
  font-family: monospace;
}
.check {
  display: flex !important;
  align-items: center;
}
.check input {
  width: auto;
}
</style>
