<script setup lang="ts">
import { computed } from 'vue'
import { ChevronDown, ChevronUp, Plus, Trash2 } from '@lucide/vue'
import type { Catalog, Target } from './model'
import { fieldOrder, targets } from './model'
import type { RuleDraft } from './useRuleDraft'

// The rule list: one group per field in execution order. Opening a rule goes
// to its own edit page; this list only manages the chain itself.
const props = defineProps<{
  drafts: RuleDraft[]
  errors: string[]
  catalog: Catalog[]
  disabled: boolean
}>()
const emit = defineEmits<{
  open: [key: string]
  add: [target: Target, kind: 'script' | 'regex', templateId?: string]
  move: [key: string, offset: -1 | 1]
  remove: [key: string]
}>()

const groups = computed(() =>
  fieldOrder.map((target) => ({
    target,
    rules: props.drafts.filter((item) => item.rule.target === target),
    templates: props.catalog.filter((item) => item.target === target),
  })),
)
function close(event: Event) {
  ;(event.target as HTMLElement).closest('details')?.removeAttribute('open')
}
</script>
<template>
  <section class="pane field-table" aria-label="按字段排列的识别规则">
    <div
      v-for="group in groups"
      :key="group.target"
      class="field-group"
    >
      <header class="field-row">
        <strong class="field-name">{{ targets[group.target] }}</strong>
        <span class="field-count muted tiny"
          >{{ group.rules.length }} 条规则</span
        >
        <details class="add-menu">
          <summary
            class="add-button"
            :aria-label="`为${targets[group.target]}新增规则`"
          >
            <Plus :size="13" />新增
          </summary>
          <div class="add-popover" @click="close">
            <button
              type="button"
              :disabled="disabled"
              @click="emit('add', group.target, 'script')"
            >
              空白脚本
            </button>
            <button
              type="button"
              :disabled="disabled"
              @click="emit('add', group.target, 'regex')"
            >
              正则
            </button>
            <button
              v-for="item in group.templates"
              :key="item.id"
              type="button"
              :disabled="disabled"
              @click="emit('add', group.target, 'script', item.id)"
            >
              模板 · {{ item.name }}
            </button>
          </div>
        </details>
      </header>
      <p v-if="!group.rules.length" class="no-rule muted tiny">
        这个字段没有规则，结果始终未知。
      </p>
      <div
        v-for="(draft, index) in group.rules"
        :key="draft.key"
        class="rule-row"
        :class="{
          inactive: !draft.rule.enabled,
          invalid: errors.includes(draft.key),
        }"
      >
        <span class="rule-position">{{ index + 1 }}</span>
        <button
          type="button"
          class="rule-open"
          :disabled="disabled"
          :aria-label="`编辑 ${draft.rule.name}`"
          @click="emit('open', draft.key)"
        >
          <span class="rule-label">{{ draft.rule.name || '未命名规则' }}</span>
          <span v-if="draft.rule.kind === 'regex'" class="kind-tag">正则</span>
          <span v-if="draft.rule.description" class="rule-desc muted">{{
            draft.rule.description
          }}</span>
          <i v-if="errors.includes(draft.key)" class="invalid-label"
            >配置有误</i
          >
        </button>
        <div class="row-actions">
          <button
            type="button"
            class="icon-button"
            :disabled="disabled || index === 0"
            :aria-label="`上移 ${draft.rule.name}`"
            @click="emit('move', draft.key, -1)"
          >
            <ChevronUp :size="14" />
          </button>
          <button
            type="button"
            class="icon-button"
            :disabled="disabled || index === group.rules.length - 1"
            :aria-label="`下移 ${draft.rule.name}`"
            @click="emit('move', draft.key, 1)"
          >
            <ChevronDown :size="14" />
          </button>
          <button
            type="button"
            class="icon-button danger"
            :disabled="disabled"
            :aria-label="`删除 ${draft.rule.name}`"
            @click="emit('remove', draft.key)"
          >
            <Trash2 :size="14" />
          </button>
          <label
            class="switch"
            :title="draft.rule.enabled ? '已启用' : '已停用'"
          >
            <input
              v-model="draft.rule.enabled"
              type="checkbox"
              :disabled="disabled"
              :aria-label="`启用 ${draft.rule.name}`"
            /><span />
          </label>
        </div>
      </div>
    </div>
  </section>
</template>
<style scoped>
.field-table {
  overflow: visible;
}
.field-group {
  border-bottom: 1px solid var(--line);
  padding: 4px 0 8px;
}
.field-group:last-child {
  border-bottom: 0;
}
.field-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 44px;
  padding: 0 18px;
}
.field-name {
  font-size: 13px;
}
.add-menu {
  position: relative;
  margin-left: auto;
}
.add-menu summary {
  list-style: none;
}
.add-menu summary::-webkit-details-marker {
  display: none;
}
.add-button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 5px 9px;
  border-radius: 6px;
  font-size: 12px;
  color: var(--purple);
  cursor: pointer;
}
.add-button:hover,
.add-menu[open] .add-button {
  background: #f2effa;
}
.add-popover {
  position: absolute;
  right: 0;
  top: calc(100% + 4px);
  z-index: 20;
  min-width: 200px;
  padding: 6px;
  background: #fff;
  border: 1px solid var(--line);
  border-radius: 8px;
  box-shadow: 0 12px 32px #29204418;
  display: grid;
}
.add-popover button {
  text-align: left;
  border: 0;
  background: none;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 12px;
}
.add-popover button:hover:not(:disabled) {
  background: #f6f4fc;
}
.no-rule {
  margin: 0;
  padding: 2px 18px 6px 40px;
}
.rule-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 8px;
  padding-left: 14px;
  border-radius: 8px;
}
.rule-row:hover {
  background: #f8f7fc;
}
.rule-row.inactive .rule-label {
  color: var(--muted);
  text-decoration: line-through;
}
.rule-position {
  color: var(--muted);
  font-size: 11px;
  width: 14px;
  font-variant-numeric: tabular-nums;
}
.rule-open {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  border: 0;
  background: none;
  padding: 9px 4px;
  text-align: left;
}
.rule-label {
  font-size: 13px;
  white-space: nowrap;
}
.rule-desc {
  font-size: 11px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.kind-tag,
.invalid-label {
  font-style: normal;
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  white-space: nowrap;
}
.kind-tag {
  background: #eef1f6;
  color: #59617a;
}
.invalid-label {
  margin-left: auto;
  background: #fbecee;
  color: var(--red);
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  padding-right: 10px;
}
.icon-button {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 6px;
  background: none;
  color: var(--muted);
}
.icon-button:hover:not(:disabled) {
  background: #efedf7;
  color: var(--purple);
}
.icon-button.danger:hover:not(:disabled) {
  background: #fbecee;
  color: var(--red);
}
.switch {
  position: relative;
  width: 30px;
  height: 18px;
  margin-left: 8px;
}
.switch input {
  position: absolute;
  inset: 0;
  opacity: 0;
  margin: 0;
  cursor: pointer;
}
.switch span {
  display: block;
  width: 100%;
  height: 100%;
  border-radius: 999px;
  background: #d9d6e2;
  transition: background 0.15s;
  pointer-events: none;
}
.switch span::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #fff;
  transition: transform 0.15s;
}
.switch input:checked + span {
  background: var(--purple);
}
.switch input:checked + span::after {
  transform: translateX(12px);
}
.switch input:focus-visible + span {
  outline: 3px solid #b8adeb;
  outline-offset: 2px;
}
@media (max-width: 760px) {
  .rule-desc {
    display: none;
  }
}
</style>
