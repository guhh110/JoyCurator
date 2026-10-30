<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import ScriptCode from './ScriptCode.vue'
import Select from '../../components/ui/Select.vue'
import EliText from '../../components/ui/EliText.vue'
import type { ExtractionResult, ScriptEvent } from './model'
import { targets, sources, statuses, valueText } from './model'
const props = withDefaults(
  defineProps<{
    result: ExtractionResult
    view?: 'all' | 'metadata' | 'steps' | 'json'
    editable?: boolean
    // Limit the steps view to one rule (the drawer shows only the edited rule).
    ruleId?: string
  }>(),
  { view: 'all' },
)
const emit = defineEmits<{ locate: [ruleId: string, event?: ScriptEvent] }>()
const activeCode = ref<
  Record<string, { phase: string; line: number; request: number }>
>({})
let focusRequest = 0
function locate(ruleId: string, event: ScriptEvent) {
  if (props.editable) emit('locate', ruleId, event)
  activeCode.value[ruleId] = {
    phase: event.phase,
    line: event.line,
    request: ++focusRequest,
  }
}
const status = ref(''),
  rule = ref(''),
  copied = ref('')
const json = computed(() => JSON.stringify(props.result, null, 2))
const visible = computed(() =>
  props.result.steps.filter(
    (step) =>
      (!status.value || step.status === status.value) &&
      (!rule.value || step.ruleId === rule.value) &&
      (!props.ruleId || step.ruleId === props.ruleId),
  ),
)
const details = ref<HTMLDetailsElement>(),
  area = ref<HTMLTextAreaElement>()
async function copy() {
  try {
    await navigator.clipboard.writeText(json.value)
    copied.value = '已复制 JSON'
  } catch {
    if (details.value) details.value.open = true
    await nextTick()
    area.value?.focus()
    area.value?.select()
    copied.value = '已选中 JSON，可手动复制'
  }
}
</script>
<template>
  <div class="extraction-output">
    <section v-if="view === 'all' || view === 'metadata'" class="pane metadata">
      <h2>文件元数据</h2>
      <p v-if="result.root" class="tiny muted extraction-root"><span>解析边界：</span><EliText :text="result.root" ellipsis="start" /></p>
      <dl>
        <template v-for="(label, key) in targets" :key="key"
          ><dt>{{ label }}</dt>
          <dd>{{ valueText(result.metadata[key]) }}</dd></template
        >
      </dl>
    </section>
    <section
      v-if="view === 'all' || view === 'steps'"
      class="pane execution-steps"
    >
      <header v-if="!ruleId" class="pane-head">
        <h2>
          规则执行过程
          <span class="count"
            >{{ visible.length }}/{{ result.steps.length }}</span
          >
        </h2>
        <div class="filters">
          <Select v-model="rule" label="筛选规则" :options="[{ value: '', label: '所有规则' }, ...result.steps.map(step => ({ value: step.ruleId, label: step.ruleName }))]" />
          <Select v-model="status" label="筛选执行结果" :options="[{ value: '', label: '所有结果' }, ...Object.entries(statuses).map(([value, label]) => ({ value, label }))]" />
        </div>
      </header>
      <p v-if="!visible.length" class="muted padding">没有符合条件的步骤。</p>
      <details
        v-for="s in visible"
        :key="s.ruleId"
        class="step"
        :open="!!ruleId || undefined"
      >
        <summary>
          <span class="step-order">{{ s.order }}</span
          ><strong>{{ s.ruleName }}</strong
          ><span class="badge" :class="s.status">{{
            statuses[s.status] || s.status
          }}</span
          ><span class="muted tiny"
            >{{ targets[s.target] }} · {{ s.durationMs }} ms</span
          >
        </summary>
        <div class="step-body">
          <button
            v-if="editable"
            class="button small"
            @click="emit('locate', s.ruleId)"
          >
            编辑此规则
          </button>
          <p>
            {{ s.reason }}
            <strong v-if="s.applied" class="purple">· 已采用</strong>
          </p>
          <dl>
            <dt>来源</dt>
            <dd>{{ sources[s.ruleConfig.source] }}</dd>
            <dt>原值 → 提取值 → 结果</dt>
            <dd>
              {{ valueText(s.before) }} → {{ valueText(s.extractedValue) }} →
              {{ valueText(s.after) }}
            </dd>
            <dt>命中文本</dt>
            <dd>{{ s.matchedText || '—' }}</dd>
            <dt>开始时间</dt>
            <dd>{{ s.startedAt }}</dd>
          </dl>
          <div v-for="(attempt, i) in s.attempts" :key="i" class="attempt">
            <span class="tiny muted"
              >尝试 {{ i + 1 }} ·
              {{ attempt.matched ? '命中' : '未命中' }}</span
            >
            <EliText v-if="['absolutePath', 'relativePath'].includes(s.ruleConfig.source)" :text="attempt.input" ellipsis="start" class="attempt-path" />
            <pre v-else>{{ attempt.input }}</pre>
            <pre v-if="attempt.captures">{{
              JSON.stringify(attempt.captures, null, 2)
            }}</pre>
            <details v-if="attempt.trace?.length" open class="script-log">
              <summary>脚本过程（{{ attempt.trace.length }} 条）</summary>
              <article
                v-for="(event, index) in attempt.trace"
                :key="index"
                class="script-event"
              >
                <header>
                  <button
                    v-if="event.line"
                    class="text-button"
                    @click="locate(s.ruleId, event)"
                  >
                    {{ event.phase === 'source' ? '来源' : '脚本' }} L{{
                      event.line
                    }}:{{ event.column }}</button
                  ><span class="tiny muted"
                    >{{ event.operation }} · {{ event.function }}</span
                  >
                </header>
                <p>{{ event.message }}</p>
                <pre v-if="event.data !== undefined">{{
                  JSON.stringify(event.data, null, 2)
                }}</pre>
              </article>
            </details>
            <p v-if="attempt.error" class="red">{{ attempt.error }}</p>
          </div>
          <details
            v-if="s.ruleConfig.config.code"
            :open="activeCode[s.ruleId]?.phase === 'extract'"
            class="script-snapshot"
          >
            <summary>本次完整脚本（快照）</summary>
            <ScriptCode
              :model-value="s.ruleConfig.config.code"
              :focus-key="activeCode[s.ruleId]?.request"
              readonly
              :active-line="
                activeCode[s.ruleId]?.phase === 'extract'
                  ? activeCode[s.ruleId]?.line
                  : undefined
              "
            />
          </details>
          <details
            v-if="s.ruleConfig.config.sourceCode"
            :open="activeCode[s.ruleId]?.phase === 'source'"
            class="script-snapshot"
          >
            <summary>本次来源清理脚本（快照）</summary>
            <ScriptCode
              :model-value="s.ruleConfig.config.sourceCode"
              :focus-key="activeCode[s.ruleId]?.request"
              readonly
              :active-line="
                activeCode[s.ruleId]?.phase === 'source'
                  ? activeCode[s.ruleId]?.line
                  : undefined
              "
            />
          </details>
          <details>
            <summary>本次规则配置</summary>
            <pre>{{ JSON.stringify(s.ruleConfig, null, 2) }}</pre>
          </details>
        </div>
      </details>
    </section>
    <section v-if="view === 'all' || view === 'json'" class="pane raw">
      <header class="pane-head">
        <h2>完整 JSON</h2>
        <button class="button small" @click="copy">复制 JSON</button>
      </header>
      <p v-if="copied" class="padding tiny" role="status">{{ copied }}</p>
      <details ref="details">
        <summary>展开 JSON</summary>
        <textarea
          ref="area"
          :value="json"
          rows="16"
          readonly
          aria-label="调试结果 JSON"
        />
      </details>
    </section>
  </div>
</template>
<style scoped>
.script-log summary {
  cursor: pointer;
}
.script-event {
  border-top: 1px solid var(--line);
  margin-top: 12px;
  padding-top: 12px;
}
.script-event header {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  align-items: center;
}
.script-event p {
  margin: 8px 0;
}
.script-event pre {
  max-height: 220px;
  overflow: auto;
}
.script-snapshot {
  margin: 16px 0;
}
.script-snapshot > summary {
  margin-bottom: 12px;
}
.extraction-output {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.extraction-root { display:flex; align-items:center; min-width:0; }
.extraction-root > span:first-child { flex-shrink:0; }
.attempt-path { display:flex; margin:12px 0; font-family:monospace; font-size:12px; }
.metadata,
.padding {
  padding: 20px;
}
h2 {
  font-size: 17px;
  margin: 0;
}
dl {
  display: grid;
  grid-template-columns: 150px minmax(0, 1fr);
  gap: 10px 16px;
  font-size: 13px;
}
dt {
  color: var(--muted);
}
dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.pane-head {
  height: auto;
  min-height: 60px;
  gap: 12px;
  flex-wrap: wrap;
}
.filters {
  display: flex;
  gap: 8px;
}
.filters :deep(.ui-select-anchor) {
  max-width: 220px;
  font-size: 12px;
}
.step {
  border-top: 1px solid var(--line);
  padding: 16px 20px;
}
.step > summary {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  cursor: pointer;
  font-size: 13px;
}
.step-order {
  color: var(--muted);
  width: 28px;
}
.badge {
  font-size: 11px;
  padding: 3px 8px;
  background: #f0eff5;
  border-radius: 5px;
}
.matched {
  background: #e5f6ed;
  color: #187946;
}
.error {
  background: #fff0f0;
  color: #b12525;
}
.step-body {
  padding: 8px 0 0;
  font-size: 13px;
}
.step-body > p {
  line-height: 1.7;
}
.attempt {
  padding: 12px;
  background: #f6f7fb;
  border-radius: 6px;
  margin: 12px 0;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
  line-height: 1.6;
}
.raw > details {
  padding: 20px;
}
.raw summary,
.step-body summary {
  cursor: pointer;
  color: var(--purple);
  font-size: 13px;
}
textarea {
  margin-top: 14px;
  font-family: monospace;
  font-size: 12px;
}
@media (max-width: 600px) {
  dl {
    grid-template-columns: 1fr;
    gap: 4px;
  }
  dd {
    margin-bottom: 8px;
  }
  .filters {
    width: 100%;
    flex-wrap: wrap;
  }
  .filters :deep(.ui-select-anchor) {
    flex: 1;
    min-width: 0;
  }
  .step {
    padding: 16px;
  }
  .step > summary strong {
    flex: 1;
    min-width: 120px;
  }
}
</style>
