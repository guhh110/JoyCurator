<script setup lang="ts">
import { computed } from 'vue'
const props = withDefaults(
  defineProps<{
    page: number
    total: number
    pageSize?: number
    disabled?: boolean
  }>(),
  { pageSize: 20 },
)
defineEmits<{ change: [page: number] }>()
const pages = computed(() =>
  Math.max(1, Math.ceil(props.total / props.pageSize)),
)
</script>
<template>
  <nav class="shared-pagination" aria-label="分页">
    <button
      class="button small"
      :disabled="disabled || page <= 1"
      @click="$emit('change', page - 1)"
    >
      上一页</button
    ><span class="tiny muted">{{ page }} / {{ pages }} · 共 {{ total }} 条</span
    ><button
      class="button small"
      :disabled="disabled || page >= pages"
      @click="$emit('change', page + 1)"
    >
      下一页
    </button>
  </nav>
</template>
<style scoped>
.shared-pagination {
  padding: 16px;
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
}
</style>
