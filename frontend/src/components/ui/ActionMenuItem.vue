<script setup lang="ts">
import { DropdownMenuItem } from 'reka-ui'

defineProps<{ disabled?: boolean; asChild?: boolean }>()
defineEmits<{ select: [event: Event] }>()
</script>

<template>
  <DropdownMenuItem class="ui-action-menu-item" :disabled="disabled" :as-child="asChild" @select="$emit('select', $event)">
    <slot />
  </DropdownMenuItem>
</template>
