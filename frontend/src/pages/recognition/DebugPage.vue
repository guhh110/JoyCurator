<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useExtraction } from '../../features/extraction/useExtraction'
import ExtractionResult from '../../features/extraction/ExtractionResult.vue'
import PageHeader from '../../components/ui/PageHeader.vue'
import type { ScriptEvent } from '../../features/extraction/model'
import {
  fieldOrder,
  targets,
  valueText,
} from '../../features/extraction/model'
const { path, root, busy, result, run, cancel } = useExtraction()
const router = useRouter()
// For each field: its value and the last applied step, which is what decided it.
const fields = computed(() =>
  fieldOrder.map((target) => {
    const decided = [...(result.value?.steps ?? [])]
      .reverse()
      .find((step) => step.target === target && step.applied)
    return {
      target,
      value: result.value?.metadata[target],
      ruleId: decided?.ruleId,
      ruleName: decided?.ruleName,
      text: decided?.matchedText,
    }
  }),
)
function edit(ruleId: string, event?: ScriptEvent) {
  void router.push({
    name: 'recognition-rule-edit',
    params: { ruleId },
    query: event?.line ? { line: String(event.line) } : {},
  })
}
</script>
<template>
  <section class="debug-page" aria-label="识别调试页面">
    <PageHeader
      title="识别调试"
      eyebrow="PATH METADATA"
      description="绝对路径 → 当前规则链 → 文件元数据"
      ><template #actions
        ><RouterLink class="button" :to="{ name: 'automatic-recognition' }"
          >返回整理工作区</RouterLink
        ></template
      ></PageHeader
    >
    <form class="pane debug-form" @submit.prevent="run()">
      <label for="debug-path">完整文件路径</label
      ><input
        id="debug-path"
        v-model="path"
        required
        placeholder="/media/Show/Season 02/03.mkv"
        autocomplete="off"
        spellcheck="false"
        :disabled="busy"
      /><label for="debug-root">解析边界目录（可选）</label
      ><input
        id="debug-root"
        v-model="root"
        placeholder="源目录内默认源目录，否则使用 /"
        autocomplete="off"
        :disabled="busy"
      />
      <p class="muted tiny">
        支持不存在的模拟路径；只解析文本，不读取文件、不访问
        TMDB，不保存执行轨迹。
      </p>
      <div class="actions">
        <button v-if="busy" type="button" class="button" @click="cancel()">
          取消请求</button
        ><button class="button primary" :disabled="busy">
          {{ busy ? '执行中…' : '开始调试' }}
        </button>
      </div>
    </form>
    <template v-if="result">
      <section class="pane field-summary" aria-label="字段识别结果">
        <header class="pane-head"><h2>字段结果</h2></header>
        <div v-for="field in fields" :key="field.target" class="summary-row">
          <strong>{{ targets[field.target] }}</strong>
          <span class="summary-value">{{ valueText(field.value) }}</span>
          <span class="summary-rule">
            <template v-if="field.ruleId"
              ><button class="text-button" @click="edit(field.ruleId)">
                {{ field.ruleName }}</button
              ><code v-if="field.text">{{ field.text }}</code></template
            >
            <span v-else class="muted">没有规则命中</span>
          </span>
        </div>
      </section>
      <ExtractionResult
        :result="result"
        view="steps"
        editable
        @locate="edit"
      />
    </template>
  </section>
</template>
<style scoped>
.debug-page {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.heading h1 {
  margin: 8px 0;
}
.debug-form {
  padding: 24px;
}
.debug-form label:not(:first-child) {
  margin-top: 18px;
}
.debug-form p {
  line-height: 1.8;
}
.actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 20px;
}
.field-summary {
  overflow: hidden;
}
.summary-row {
  display: grid;
  grid-template-columns: 96px minmax(80px, 0.7fr) minmax(0, 1.6fr);
  gap: 16px;
  align-items: center;
  padding: 11px 18px;
  border-top: 1px solid var(--line);
  font-size: 13px;
}
.summary-value {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.summary-rule {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.summary-rule code {
  margin-left: 8px;
  padding: 2px 6px;
  border-radius: 4px;
  background: #f2effa;
  color: var(--purple-dark);
  font-size: 11px;
}
@media (max-width: 600px) {
  .summary-row {
    grid-template-columns: 72px 1fr;
  }
  .summary-rule {
    grid-column: 1 / -1;
  }
  .heading {
    align-items: flex-start;
    flex-direction: column;
  }
  .debug-form {
    padding: 18px;
  }
}
</style>
