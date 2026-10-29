<script setup lang="ts">
import { LoaderCircle } from '@lucide/vue'
defineProps<{
  loading?: boolean
  error?: unknown
  empty?: boolean
  emptyText?: string
}>()
defineEmits<{ retry: [] }>()
</script>
<template>
  <div v-if="loading" class="query-state muted" role="status">
    <LoaderCircle :size="18" class="spin purple" />正在加载…
  </div>
  <div v-else-if="error" class="notice error query-failure" role="alert">
    <span>{{ error instanceof Error ? error.message : String(error) }}</span
    ><button class="button small" @click="$emit('retry')">重新加载</button>
  </div>
  <div v-else-if="empty" class="pane query-state muted">
    {{ emptyText || '暂无内容' }}<slot name="empty" />
  </div>
  <slot v-else />
</template>
<style scoped>
.query-state {
  padding: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  flex-wrap: wrap;
  font-size: 13px;
  min-height: 100px;
}
.query-failure {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
