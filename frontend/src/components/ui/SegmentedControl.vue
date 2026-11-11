<script setup lang="ts">
import { ToggleGroupRoot, ToggleGroupItem } from 'reka-ui'
defineProps<{ label: string; options: { value: string; label: string }[]; disabled?: boolean }>()
const value = defineModel<string>({ required: true })
function update(next: unknown) { if (typeof next === 'string' && next) value.value = next }
</script>
<template>
  <ToggleGroupRoot class="ui-segmented" type="single" :model-value="value" :disabled="disabled" :aria-label="label" @update:model-value="update">
    <ToggleGroupItem v-for="option in options" :key="option.value" :value="option.value" class="ui-segmented-item">{{ option.label }}</ToggleGroupItem>
  </ToggleGroupRoot>
</template>
<style>
.ui-segmented{display:inline-flex;align-items:center;gap:3px;padding:3px;min-width:0;background:#f2f0f6;border:1px solid #ebe7f1;border-radius:9px}.ui-segmented-item{display:flex;align-items:center;justify-content:center;flex:1;height:28px;min-height:28px;margin:0;padding:0 12px;border:0;border-radius:6px;background:transparent;color:var(--muted);white-space:nowrap;font-size:12px;line-height:1.2}.ui-segmented-item[data-state=on]{background:#fff;color:var(--purple-dark);box-shadow:0 1px 4px #2920440d;font-weight:600}.ui-segmented-item:focus-visible{outline:2px solid #b8adeb;outline-offset:1px}
</style>
