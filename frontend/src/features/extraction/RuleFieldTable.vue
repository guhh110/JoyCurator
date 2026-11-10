<script setup lang="ts">
import type { RuleDraft } from './useRuleDraft'
import { canMoveRule } from './useRuleDraft'
import { roles, stages } from './model'
defineProps<{ drafts: RuleDraft[]; errors: string[]; disabled?: boolean }>()
defineEmits<{
  open: [key: string]
  move: [key: string, offset: -1 | 1]
  remove: [key: string]
}>()
</script>
<template>
  <div class="pipeline-list">
    <section
      v-for="stage in ['tokenize', 'annotate', 'resolve'] as const"
      :key="stage"
      class="pane"
    >
      <h2 v-if="stage === 'annotate'">{{ stages[stage] }}</h2>
      <h2 v-else>
        <button
          class="text-button"
          :disabled="disabled"
          @click="
            drafts.find((d) => d.rule.stage === stage) &&
            $emit('open', drafts.find((d) => d.rule.stage === stage)!.key)
          "
        >
          {{ stages[stage] }}
        </button>
      </h2>
      <div
        v-for="d in drafts.filter(
          (d) => stage === 'annotate' && d.rule.stage === stage,
        )"
        :key="d.key"
        class="rule-row"
        :class="{ invalid: errors.includes(d.key) }"
      >
        <input
          v-if="stage === 'annotate'"
          v-model="d.rule.enabled"
          type="checkbox"
          :disabled="disabled"
          :aria-label="`启用${d.rule.name}`"
        /><button class="text-button" @click="$emit('open', d.key)">
          {{ d.rule.name }}</button
        ><span
          v-if="d.rule.target && roles[d.rule.target] !== d.rule.name"
          class="muted"
          >{{ roles[d.rule.target] }}</span
        ><template v-if="stage === 'annotate'"
          ><button
            class="button small"
            :disabled="disabled || !canMoveRule(drafts, d.key, -1)"
            :aria-label="`上移${d.rule.name}`"
            @click="$emit('move', d.key, -1)"
          >
            ↑</button
          ><button
            class="button small"
            :disabled="disabled || !canMoveRule(drafts, d.key, 1)"
            :aria-label="`下移${d.rule.name}`"
            @click="$emit('move', d.key, 1)"
          >
            ↓</button
          ><button
            class="text-button danger"
            :disabled="disabled"
            @click="$emit('remove', d.key)"
          >
            删除
          </button></template
        >
      </div>
    </section>
  </div>
</template>
<style scoped>
.pipeline-list {
  display: grid;
  gap: 16px;
}
.pane h2 {
  font-size: 17px;
  padding: 18px;
}
.rule-row {
  display: flex;
  gap: 14px;
  align-items: center;
  padding: 12px 18px;
  border-top: 1px solid var(--line);
  font-size: 13px;
}
.rule-row > .text-button:first-of-type {
  margin-right: auto;
}
.rule-row input {
  width: auto;
}
.invalid {
  background: #fff0f0;
}
</style>
