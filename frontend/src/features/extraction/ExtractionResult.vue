<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Select from '../../components/ui/Select.vue'
import ScriptCode from './ScriptCode.vue'
import { notify } from '../../app/toast'
import {
  targets,
  roles,
  stages,
  statuses,
  valueText,
  type ExtractionResult,
  type RecognitionSnapshot,
  pipelineRules,
  type ScriptEvent,
  type Target,
} from './model'
const props = withDefaults(
  defineProps<{
    result: ExtractionResult | RecognitionSnapshot
    view?: 'all' | 'metadata' | 'steps' | 'rules' | 'json'
    editable?: boolean
    initialPath?: string
    ruleId?: string
  }>(),
  { view: 'all' },
)
const emit = defineEmits<{ locate: [ruleId: string, event?: ScriptEvent] }>()
const fileId = ref(''),
  ruleId = ref(''),
  candidateId = ref(''),
  status = ref('')
watch(
  () => [props.result, props.initialPath],
  () => {
    fileId.value =
      props.result.documents.find((d) => d.path === props.initialPath)
        ?.fileId ??
      props.result.files[0]?.fileId ??
      props.result.documents[0]?.fileId ??
      ''
    candidateId.value = ''
  },
  { immediate: true },
)
const doc = computed(() =>
    props.result.documents.find((d) => d.fileId === fileId.value),
  ),
  file = computed(() =>
    props.result.files.find((f) => f.fileId === fileId.value),
  )
const steps = computed(() => 'steps' in props.result ? props.result.steps : [])
const savedRules = computed(() => 'pipeline' in props.result && props.result.digest ? pipelineRules(props.result.pipeline) : [])
const visible = computed(() =>
  steps.value.filter(
    (s) =>
      (!s.fileId || s.fileId === fileId.value) &&
      (!props.ruleId || s.ruleId === props.ruleId) &&
      (!ruleId.value || s.ruleId === ruleId.value) &&
      (!status.value || s.status === status.value),
  ),
)
const snapshots = computed(
  () =>
    new Map(
      steps.value
        .filter((s) => s.ruleConfig.id)
        .map((s) => [s.ruleId, s.ruleConfig]),
    ),
)
const ruleOptions = computed(() =>
  Array.from(
    new Map(
      steps.value.map((s) => [
        s.ruleId,
        { value: s.ruleId, label: s.ruleName },
      ]),
    ).values(),
  ),
)
const activeLine = ref<Record<string, number>>({})
function locate(id: string, event: ScriptEvent) {
  activeLine.value[id] = event.line
  if (props.editable) emit('locate', id, event)
}
function evidence(ref: { fileId: string; candidateId: string }) {
  fileId.value = ref.fileId
  candidateId.value = ref.candidateId
}
const fields = Object.entries(targets) as [Target, string][]
async function copy() {
  try {
    await navigator.clipboard.writeText(JSON.stringify(props.result, null, 2))
    notify('已复制 JSON', 'success')
  } catch {
    notify('复制失败，请手动选择 JSON', 'error')
  }
}
</script>
<template>
  <div class="output">
    <p v-if="result.error" class="notice error">{{ result.error }}</p>
    <template v-if="'diagnostics' in result">
      <p v-for="(error, index) in result.diagnostics" :key="index" class="notice error">{{ error.ruleId }} · {{ error.fileId }}<span v-if="error.line"> · L{{ error.line }}:{{ error.column }}</span> · {{ error.message }}</p>
    </template>
    <label class="file-select"
      >文件<Select
        label="选择文件"
        ellipsis="start"
        v-model="fileId"
        :options="
          result.documents.map((d) => ({
            value: d.fileId,
            label: d.path,
          }))
        "
    /></label>
    <section v-if="view === 'all' || view === 'metadata'" class="pane">
      <h2>识别建议</h2>
      <div v-if="file" class="metadata">
        <div v-for="[key, label] in fields" :key="key" class="field">
          <strong>{{ label }}</strong
          ><span>{{ valueText(file.metadata[key]) }}</span>
          <div v-if="file.decisions[key]">
            <span>{{ file.decisions[key]!.reason }}</span
            ><button
              v-for="ref in file.decisions[key]!.candidates"
              :key="ref.fileId + ref.candidateId"
              class="text-button"
              @click="evidence(ref)"
            >
              {{ ref.fileId }} · {{ ref.candidateId }}
            </button>
          </div>
        </div>
      </div>
      <p v-else class="padding muted">该文件仅提供上下文。</p>
      <template v-if="doc"
        ><details open class="padding">
          <summary>候选（{{ doc.candidates.length }}）</summary>
          <article
            v-for="c in doc.candidates"
            :key="c.id"
            :class="{
              highlight: c.id === candidateId,
              invalid: c.status === 'invalid',
            }"
            class="candidate"
          >
            <strong>{{ roles[c.role] }} · {{ valueText(c.value) }}</strong
            ><span
              >{{ c.status === 'invalid' ? '无效' : c.strength }} ·
              {{ c.reason }}</span
            ><span
              v-for="s in c.spans"
              :key="s.sourceId + s.start"
              class="muted"
              >{{ s.sourceId }} [{{ s.start }}, {{ s.end }}) ·
              {{
                doc.sources
                  .find((source) => source.id === s.sourceId)
                  ?.text.slice(s.start, s.end)
              }}</span
            >
          </article>
        </details>
        <details class="padding">
          <summary>来源原文</summary>
          <p v-for="source in doc.sources" :key="source.id">{{ source.id }} · {{ source.text }}</p>
        </details>
        <details class="padding">
          <summary>分词（{{ doc.tokens.length }}）</summary>
          <div class="tokens">
            <span
              v-for="t in doc.tokens"
              :key="t.id"
              :title="`${t.id} · ${t.sourceId} [${t.start}, ${t.end}) · parent: ${t.parentId ?? '—'}`"
              :class="t.kind"
              >{{ t.kind }} · {{ t.text }}</span
            >
          </div>
        </details></template
      >
    </section>
    <section v-if="'steps' in result && (view === 'all' || view === 'steps')" class="pane">
      <header class="pane-head">
        <h2>执行过程</h2>
        <Select
          label="筛选规则"
          v-model="ruleId"
          :options="[{ value: '', label: '全部规则' }, ...ruleOptions]"
        /><Select
          label="筛选状态"
          v-model="status"
          :options="[
            { value: '', label: '全部状态' },
            ...Object.entries(statuses).map(([value, label]) => ({
              value,
              label,
            })),
          ]"
        />
      </header>
      <details
        v-for="(s, i) in visible"
        :key="s.fileId + s.ruleId + i"
        class="padding"
      >
        <summary>
          {{ stages[s.stage] }} · {{ s.ruleName }} ·
          {{ statuses[s.status] ?? s.status }} · {{ s.durationMs }} ms
        </summary>
        <button
          v-if="editable"
          class="text-button"
          @click="emit('locate', s.ruleId)"
        >
          编辑规则
        </button>
        <p v-if="s.reason" class="red">{{ s.reason }}</p>
        <details>
          <summary>本步骤输出</summary>
          <pre>{{ JSON.stringify(s.extractedValue, null, 2) }}</pre>
        </details>
        <div v-for="(attempt, a) in s.attempts" :key="a">
          <p v-if="attempt.error" class="red">{{ attempt.error }}</p>
          <article v-for="(event, e) in attempt.trace" :key="e" class="log">
            <button
              v-if="event.line"
              class="text-button"
              @click="locate(s.ruleId, event)"
            >
              L{{ event.line }}:{{ event.column }}
            </button>
            {{ event.message }}
            <pre v-if="event.data !== undefined">{{
              JSON.stringify(event.data, null, 2)
            }}</pre>
          </article>
        </div>
        <details v-if="snapshots.get(s.ruleId)?.config.code">
          <summary>脚本快照</summary>
          <ScriptCode
            :model-value="snapshots.get(s.ruleId)!.config.code!"
            readonly
            :active-line="activeLine[s.ruleId]"
          />
        </details>
      </details>
    </section>
    <section v-if="view === 'rules'" class="pane padding">
      <details v-for="rule in savedRules" :key="rule.id">
        <summary>{{ stages[rule.stage] }} · {{ rule.name }}</summary>
        <ScriptCode v-if="rule.config.code" :model-value="rule.config.code" readonly />
        <pre>{{ JSON.stringify(rule, null, 2) }}</pre>
      </details>
    </section>
    <section v-if="view === 'all' || view === 'json'" class="pane padding">
      <button class="button small" @click="copy">复制 JSON</button>
      <details>
        <summary>完整 JSON</summary>
        <pre tabindex="0">{{ JSON.stringify(result, null, 2) }}</pre>
      </details>
    </section>
  </div>
</template>
<style scoped>
.output {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  min-width: 0;
}
.file-select {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.file-select > :last-child {
  flex: 1;
  min-width: 0;
}
h2 {
  font-size: 17px;
  padding: 18px;
}
.pane-head h2 {
  padding: 0;
}
.pane-head {
  height: auto;
  gap: 12px;
  flex-wrap: wrap;
  padding: 16px;
}
.padding {
  padding: 16px 20px;
  border-top: 1px solid var(--line);
}
summary {
  cursor: pointer;
  font-size: 13px;
  margin-bottom: 10px;
}
.field {
  display: grid;
  grid-template-columns: 90px minmax(80px, 1fr) minmax(0, 2fr);
  gap: 14px;
  padding: 12px 20px;
  border-top: 1px solid var(--line);
  font-size: 13px;
  overflow-wrap: anywhere;
}
.field > div {
  display: grid;
  gap: 5px;
}
.candidate {
  display: grid;
  gap: 6px;
  padding: 12px;
  border-top: 1px solid var(--line);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.highlight {
  background: #eee9fc;
}
.invalid {
  color: var(--red);
}
.tokens {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.tokens span {
  padding: 6px;
  background: #f2effa;
  font-size: 12px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.tokens .group {
  border: 1px solid var(--purple);
}
pre {
  max-height: 400px;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
}
.log {
  border-top: 1px solid var(--line);
  padding: 8px;
  font-size: 12px;
}
@media (max-width: 600px) {
  .field {
    grid-template-columns: 80px 1fr;
  }
  .field > div,
  .field > .muted {
    grid-column: 1/-1;
  }
}
</style>
