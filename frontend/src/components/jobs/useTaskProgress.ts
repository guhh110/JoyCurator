import { computed, ref, watch, type Ref } from 'vue'
import type { QueueStatus } from '../../model'

export function useTaskProgress(queue: Ref<QueueStatus | null>) {
  const collapsed = ref(false)
  const lanes = computed(() => queue.value ? [
    { purpose: 'organize', name: '文件整理', to: '/records', ...queue.value.organize },
    { purpose: 'scrape', name: '媒体刮削', to: '/scrape/records', ...queue.value.scrape },
  ].filter(lane => lane.current || lane.queued) : [])
  const running = computed(() => lanes.value.filter(lane => lane.current).length)
  const total = computed(() => running.value + lanes.value.reduce((count, lane) => count + lane.queued, 0))
  watch(() => lanes.value.length, count => {
    if (!count) collapsed.value = false
  })
  return { collapsed, lanes, running, total }
}
