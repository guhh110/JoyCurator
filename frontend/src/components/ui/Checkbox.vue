<script setup lang="ts">
import { Check, Minus } from '@lucide/vue'
import { CheckboxRoot, CheckboxIndicator } from 'reka-ui'
defineProps<{ disabled?: boolean; label?: string; id?: string }>()
const checked = defineModel<boolean | 'indeterminate'>({ default: false })
</script>
<template>
  <CheckboxRoot v-model="checked" class="ui-checkbox" :disabled="disabled" :aria-label="label" :id="id">
    <CheckboxIndicator class="ui-checkbox-indicator"><Minus v-if="checked === 'indeterminate'" :size="12" :stroke-width="3" /><Check v-else :size="12" :stroke-width="3" /></CheckboxIndicator>
  </CheckboxRoot>
</template>
<style>
.ui-checkbox{display:inline-flex;align-items:center;justify-content:center;flex:none;width:17px;height:17px;min-height:0;margin:0;padding:0;border:1px solid #d7d1e4;border-radius:4px;background:#fff;color:#fff;cursor:pointer;vertical-align:middle}.ui-checkbox[data-state=checked],.ui-checkbox[data-state=indeterminate]{background:var(--purple);border-color:var(--purple)}.ui-checkbox-indicator{display:flex;align-items:center;justify-content:center}.ui-checkbox:focus-visible{outline:3px solid #cfc5ee;outline-offset:3px}.ui-checkbox[data-disabled]{opacity:.4;cursor:not-allowed}
</style>
