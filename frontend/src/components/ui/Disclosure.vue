<script setup lang="ts">
import { ChevronDown } from '@lucide/vue'
import { CollapsibleRoot, CollapsibleTrigger, CollapsibleContent } from 'reka-ui'
import Button from './Button.vue'
defineProps<{ label: string }>()
const open = defineModel<boolean>('open', { default: false })
</script>
<template>
  <CollapsibleRoot v-model:open="open" :unmount-on-hide="false" class="ui-disclosure"><CollapsibleTrigger as-child><Button variant="ghost" size="sm"><ChevronDown :size="14" :class="{ 'ui-disclosure-open': open }" />{{ label }}</Button></CollapsibleTrigger><CollapsibleContent><slot /></CollapsibleContent></CollapsibleRoot>
</template>
<style>.ui-disclosure-open{transform:rotate(180deg)}.ui-disclosure>button{margin-left:-9px}</style>
