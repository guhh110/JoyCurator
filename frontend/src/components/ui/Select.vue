<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import EliText from './EliText.vue'
import { Check, ChevronDown } from '@lucide/vue'
import {
  SelectRoot, SelectTrigger, SelectValue, SelectPortal, SelectContent,
  SelectViewport, SelectItem, SelectItemText, SelectItemIndicator,
} from 'reka-ui'

const props = defineProps<{
  modelValue?: string
  options: { value: string; label: string }[]
  placeholder?: string
  disabled?: boolean
  label: string
  id?: string
  ellipsis?: 'start' | 'end'
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const anchor = ref<HTMLElement>()
const selectedOption = computed(() => props.options.find(option => option.value === props.modelValue))
const portalTarget = ref<HTMLElement | string>('body')
onMounted(() => { portalTarget.value = anchor.value?.closest('dialog') || 'body' })
// Reka reserves the empty string for placeholders. Encode every option so an
// empty application value (for example "all") remains an ordinary choice.
const optionKey = (value: string) => `value:${value}`
function update(value: unknown) {
  const option = props.options.find(option => optionKey(option.value) === value)
  if (option) emit('update:modelValue', option.value)
}
</script>

<template>
  <span ref="anchor" class="ui-select-anchor">
  <SelectRoot :model-value="modelValue === undefined ? undefined : optionKey(modelValue)" :disabled="disabled" @update:model-value="update">
    <SelectTrigger :id="id" class="ui-select-trigger" :aria-label="label">
      <SelectValue v-if="ellipsis" :placeholder="placeholder"><EliText v-if="selectedOption" :text="selectedOption.label" :ellipsis="ellipsis" :focusable="false" /><template v-else>{{ placeholder }}</template></SelectValue>
      <SelectValue v-else :placeholder="placeholder" />
      <ChevronDown :size="14" />
    </SelectTrigger>
    <SelectPortal :to="portalTarget">
      <SelectContent class="ui-select-content" position="popper" :side-offset="5" align="end">
        <SelectViewport class="ui-select-viewport">
          <SelectItem v-for="option in options" :key="option.value" :value="optionKey(option.value)" :text-value="option.label" class="ui-select-item">
            <SelectItemText><EliText v-if="ellipsis" :text="option.label" :ellipsis="ellipsis" :focusable="false" /><template v-else>{{ option.label }}</template></SelectItemText>
            <SelectItemIndicator class="ui-select-check"><Check :size="14" /></SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
  </span>
</template>

<style>
.ui-select-anchor { display:inline-flex; max-width:100%; }
.ui-select-trigger { display:inline-flex; align-items:center; justify-content:space-between; gap:12px; min-height:32px; padding:6px 10px; border:1px solid #e4e3ed; border-radius:8px; background:#fff; color:#333543; font-size:12px; white-space:nowrap; }
.ui-select-trigger { width:100%; min-width:0; }
.ui-select-trigger > span { min-width:0; overflow:hidden; text-overflow:ellipsis; }
.ui-select-trigger > svg { color:var(--muted); flex-shrink:0; }
.ui-select-trigger .eli-text { display:flex; width:100%; pointer-events:auto; }
.ui-select-item > span:first-child { min-width:0; max-width:100%; }
.ui-select-trigger:hover:not(:disabled) { border-color:#cfc5ee; background:#fcfbfe; }
.ui-select-trigger[data-state='open'] { border-color:#b2a3e8; box-shadow:0 0 0 3px #7863ca12; }
.ui-select-content { z-index:110; min-width:var(--reka-select-trigger-width); max-height:min(320px, var(--reka-select-content-available-height)); overflow:auto; border:1px solid #e6e3ed; border-radius:9px; background:#fff; color:#333543; box-shadow:0 8px 28px #29204420; }
.ui-select-content { max-width:calc(100vw - 24px); }
.ui-select-viewport { padding:4px; }
.ui-select-item { position:relative; display:flex; align-items:center; min-height:34px; padding:7px 32px 7px 10px; border-radius:5px; font-size:12px; outline:none; cursor:default; user-select:none; }
.ui-select-item[data-highlighted] { background:#f1edfb; color:var(--purple-dark); }
.ui-select-check { position:absolute; right:9px; display:flex; color:var(--purple); }
</style>
