<script setup lang="ts">
import ExtractionHistory from '../../features/extraction/ExtractionHistory.vue'
defineProps<{ traceId?: string }>()
</script>
<template><ExtractionHistory :trace-id="traceId" /></template>
