<script setup lang="ts">
import { computed } from 'vue'
import { LoaderCircle, Clock3 } from '@lucide/vue'
import { RouterLink } from 'vue-router'
import { useRuntime } from '../../app/runtime'
const {queue, inspect, recordAction, actionBusy} = useRuntime()
const lanes = computed(() => queue.value ? [
  {purpose:'organize',name:'文件整理',to:'/records',...queue.value.organize},
  {purpose:'scrape',name:'媒体刮削',to:'/scrape/records',...queue.value.scrape},
].filter(lane => lane.current || lane.queued) : [])
</script>
<template>
  <div v-if="lanes.length" class="task-toast queue-toast">
    <div v-for="lane in lanes" :key="lane.purpose" class="queue-lane">
      <LoaderCircle v-if="lane.current" class="spin purple" :size="18" /><Clock3 v-else :size="18" />
      <div class="queue-copy"><strong>{{ lane.name }} · {{ lane.current ? lane.current.undo ? '撤销中' : '进行中' : '等待中' }}</strong>
        <span v-if="lane.current">{{ lane.current.title }} · {{ lane.current.completed }}/{{ lane.current.total }} 项</span>
        <span v-if="lane.queued">等待 {{ lane.queued }} 个任务<span v-if="lane.queuedBatches"> · {{ lane.queuedBatches }} 个批次</span></span>
        <span v-if="lane.waitReason">{{ lane.waitReason }}</span>
      </div>
      <button v-if="lane.current" class="text-button" @click="inspect(lane.current)">详情</button>
      <button v-if="lane.current" class="text-button" :disabled="actionBusy" @click="recordAction(lane.current,'cancel')">取消</button>
      <RouterLink class="text-button" :to="lane.to">记录</RouterLink>
    </div>
  </div>
</template>
<style scoped>
.queue-toast {display:grid;gap:12px;max-width:min(620px,calc(100vw - 24px));}
.queue-lane {display:flex;align-items:center;gap:10px;min-width:0;}
.queue-copy {display:grid;gap:3px;flex:1;min-width:0;}
.queue-copy strong {font-size:12px;}
.queue-copy span {font-size:11px;color:var(--muted);overflow-wrap:anywhere;}
</style>
