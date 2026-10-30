<script setup lang="ts">
import { Check, ChevronDown } from '@lucide/vue'
import {
  SelectRoot, SelectTrigger, SelectValue, SelectPortal, SelectContent,
  SelectViewport, SelectItem, SelectItemText, SelectItemIndicator,
} from 'reka-ui'

defineProps<{
  modelValue?: string
  options: { value: string; label: string }[]
  placeholder?: string
  disabled?: boolean
  label: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <SelectRoot :model-value="modelValue" :disabled="disabled" @update:model-value="(value) => { if (typeof value === 'string') emit('update:modelValue', value) }">
    <SelectTrigger class="ui-select-trigger" :aria-label="label">
      <SelectValue :placeholder="placeholder" />
      <ChevronDown :size="14" />
    </SelectTrigger>
    <SelectPortal>
      <SelectContent class="ui-select-content" position="popper" :side-offset="5" align="end">
        <SelectViewport class="ui-select-viewport">
          <SelectItem v-for="option in options" :key="option.value" :value="option.value" class="ui-select-item">
            <SelectItemText>{{ option.label }}</SelectItemText>
            <SelectItemIndicator class="ui-select-check"><Check :size="14" /></SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

<style>
.ui-select-trigger { display:inline-flex; align-items:center; justify-content:space-between; gap:12px; min-height:32px; padding:6px 10px; border:1px solid #e4e3ed; border-radius:8px; background:#fff; color:#333543; font-size:12px; white-space:nowrap; }
.ui-select-trigger > svg { color:var(--muted); flex-shrink:0; }
.ui-select-trigger:hover:not(:disabled) { border-color:#cfc5ee; background:#fcfbfe; }
.ui-select-trigger[data-state='open'] { border-color:#b2a3e8; box-shadow:0 0 0 3px #7863ca12; }
.ui-select-content { z-index:110; min-width:var(--reka-select-trigger-width); max-height:min(320px, var(--reka-select-content-available-height)); overflow:auto; border:1px solid #e6e3ed; border-radius:9px; background:#fff; color:#333543; box-shadow:0 8px 28px #29204420; }
.ui-select-viewport { padding:4px; }
.ui-select-item { position:relative; display:flex; align-items:center; min-height:34px; padding:7px 32px 7px 10px; border-radius:5px; font-size:12px; outline:none; cursor:default; user-select:none; }
.ui-select-item[data-highlighted] { background:#f1edfb; color:var(--purple-dark); }
.ui-select-check { position:absolute; right:9px; display:flex; color:var(--purple); }
</style>
