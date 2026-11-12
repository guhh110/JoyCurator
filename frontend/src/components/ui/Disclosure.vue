<script setup lang="ts">
import { ChevronDown } from '@lucide/vue'
import { CollapsibleRoot, CollapsibleTrigger, CollapsibleContent } from 'reka-ui'
import Button from './Button.vue'
defineProps<{ label: string }>()
const open = defineModel<boolean>('open', { default: false })
</script>
<template>
  <CollapsibleRoot v-model:open="open" :unmount-on-hide="false" class="ui-disclosure"><div class="ui-disclosure-heading"><slot name="heading" /><CollapsibleTrigger as-child><Button variant="ghost" size="sm"><ChevronDown :size="14" :class="{ 'ui-disclosure-open': open }" />{{ label }}</Button></CollapsibleTrigger></div><CollapsibleContent><slot /></CollapsibleContent></CollapsibleRoot>
</template>
<style>.ui-disclosure-open{transform:rotate(180deg)}.ui-disclosure-heading{display:flex;align-items:center;gap:12px;min-width:0}.ui-disclosure-heading>.ui-button{flex:none;margin-left:auto}</style>
