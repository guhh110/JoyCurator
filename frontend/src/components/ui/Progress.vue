<script setup lang="ts">
import { computed } from 'vue'
import { ProgressRoot, ProgressIndicator } from 'reka-ui'
const props = defineProps<{ value: number; max: number }>()
const percent = computed(() => Math.min(100,Math.max(0,props.value / Math.max(1,props.max) * 100)))
</script>
<template><ProgressRoot class="ui-progress" :model-value="value" :max="max" aria-label="识别进度"><ProgressIndicator class="ui-progress-indicator" :style="{transform:`translateX(-${100-percent}%)`}" /></ProgressRoot></template>
<style>.ui-progress{height:5px;border-radius:8px;background:#ece8f4;overflow:hidden;min-width:0}.ui-progress-indicator{height:100%;width:100%;background:var(--purple);transition:transform .2s}</style>
