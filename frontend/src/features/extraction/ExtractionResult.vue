<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import type { ExtractionResult } from './model'
import { targets, sources, statuses, valueText } from './model'
const props = defineProps<{ result: ExtractionResult }>()
const status = ref(''),
  rule = ref(''),
  copied = ref('')
const json = computed(() => JSON.stringify(props.result, null, 2))
const visible = computed(() =>
  props.result.steps.filter(
    (step) =>
      (!status.value || step.status === status.value) &&
      (!rule.value || step.ruleId === rule.value),
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
    <section class="pane metadata">
      <h2>文件元数据</h2>
      <p v-if="result.root" class="tiny muted">解析边界：{{ result.root }}</p>
      <dl>
        <template v-for="(label, key) in targets" :key="key"
          ><dt>{{ label }}</dt>
          <dd>{{ valueText(result.metadata[key]) }}</dd></template
        >
      </dl>
    </section>
    <section class="pane execution-steps">
      <header class="pane-head">
        <h2>
          规则执行过程
          <span class="count"
            >{{ visible.length }}/{{ result.steps.length }}</span
          >
        </h2>
        <div class="filters">
          <select v-model="rule" aria-label="筛选规则">
            <option value="">所有规则</option>
            <option v-for="s in result.steps" :key="s.ruleId" :value="s.ruleId">
              {{ s.ruleName }}
            </option>
          </select>
          <select v-model="status" aria-label="筛选执行结果">
            <option value="">所有结果</option>
            <option v-for="(label, key) in statuses" :key="key" :value="key">
              {{ label }}
            </option>
          </select>
        </div>
      </header>
      <p v-if="!visible.length" class="muted padding">没有符合条件的步骤。</p>
      <details v-for="s in visible" :key="s.ruleId" class="step">
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
            <pre>{{ attempt.input }}</pre>
            <pre v-if="attempt.captures">{{
              JSON.stringify(attempt.captures, null, 2)
            }}</pre>
            <p v-if="attempt.error" class="red">{{ attempt.error }}</p>
          </div>
          <details>
            <summary>本次规则配置</summary>
            <pre>{{ JSON.stringify(s.ruleConfig, null, 2) }}</pre>
          </details>
        </div>
      </details>
    </section>
    <section class="pane raw">
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
.extraction-output {
  display: grid;
  gap: 20px;
  min-width: 0;
}
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
.filters select {
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
  .filters select {
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
