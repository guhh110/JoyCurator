<script setup lang="ts">
import ExtractionHistoryDetail from '../../features/extraction/ExtractionHistoryDetail.vue'
defineProps<{ traceId: string }>()
</script>
<template><ExtractionHistoryDetail :key="traceId" :trace-id="traceId" /></template>
