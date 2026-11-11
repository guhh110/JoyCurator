<script setup lang="ts">
defineProps<{ label: string; for?: string }>()
</script>
<template><div class="ui-field"><label v-if="$props.for" class="ui-field-label" :for="$props.for">{{ label }}</label><span v-else class="ui-field-label">{{ label }}</span><slot /></div></template>
<style>
.ui-field{display:grid;grid-template-columns:minmax(0,1fr);align-content:start;gap:8px;min-width:0}.ui-field>.ui-field-label{display:block;margin:0;color:var(--muted);font-size:11px;font-weight:500;line-height:16px}.ui-field>.ui-select-anchor{width:100%}
</style>
