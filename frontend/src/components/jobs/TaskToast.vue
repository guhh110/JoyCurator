<script setup lang="ts">
import { LoaderCircle, Clock3, ChevronDown, ChevronUp } from '@lucide/vue'
import { RouterLink } from 'vue-router'
import { useRuntime } from '../../app/runtime'
import { useTaskProgress } from './useTaskProgress'
const { queue, inspect, recordAction, actionBusy } = useRuntime()
const { collapsed, lanes, running, total } = useTaskProgress(queue)
</script>
<template>
  <section v-if="lanes.length" class="task-toast queue-toast" :class="{ 'is-collapsed': collapsed }" aria-label="任务进度">
    <div class="queue-header">
      <template v-if="collapsed">
        <LoaderCircle v-if="running" class="spin purple" :size="16" />
        <Clock3 v-else class="purple" :size="16" />
      </template>
      <strong>{{ collapsed ? `${running ? '任务进行中' : '任务排队中'} · ${total} 项` : '任务进度' }}</strong>
      <button
        class="text-button queue-toggle"
        :aria-expanded="!collapsed"
        aria-controls="task-progress-lanes"
        :aria-label="collapsed ? '展开任务进度' : '收起任务进度'"
        @click="collapsed = !collapsed"
      >
        {{ collapsed ? '展开' : '收起' }}
        <ChevronUp v-if="collapsed" :size="14" /><ChevronDown v-else :size="14" />
      </button>
    </div>
    <div v-if="!collapsed" id="task-progress-lanes" class="queue-lanes">
      <div v-for="lane in lanes" :key="lane.purpose" class="queue-lane">
        <LoaderCircle v-if="lane.current" class="queue-icon spin purple" :size="18" /><Clock3 v-else class="queue-icon purple" :size="18" />
        <div class="queue-copy">
          <strong>{{ lane.name }} · {{ lane.current ? lane.current.undo ? '撤销中' : '进行中' : '等待中' }}</strong>
          <div v-if="lane.current" class="queue-current">
            <span class="queue-title" :title="lane.current.title">{{ lane.current.title }}</span>
            <span class="queue-progress">{{ lane.current.completed }}/{{ lane.current.total }} 项</span>
          </div>
          <span v-if="lane.queued" class="queue-count">等待 {{ lane.queued }} 个任务<template v-if="lane.queuedBatches"> · {{ lane.queuedBatches }} 个批次</template></span>
          <span v-if="lane.waitReason" class="queue-wait-reason">{{ lane.waitReason }}</span>
        </div>
        <div class="queue-actions">
          <button v-if="lane.current" class="text-button" @click="inspect(lane.current)">详情</button>
          <button v-if="lane.current" class="text-button" :disabled="actionBusy" @click="recordAction(lane.current, 'cancel')">取消</button>
          <RouterLink class="text-button" :to="lane.to">记录</RouterLink>
        </div>
      </div>
    </div>
  </section>
</template>
<style scoped>
.queue-toast {
  position: fixed;
  bottom: 101px;
  right: 34px;
  z-index: 50;
  width: 360px;
  max-width: calc(100vw - 30px);
  padding: 14px 16px;
  border: 1px solid #e8e0f3;
  border-radius: 11px;
  background: #fff;
  box-shadow: 0 7px 27px #5a417c12;
}
.queue-header { display: flex; align-items: center; gap: 8px; }
.queue-header strong { flex: 1; min-width: 0; font-size: 12px; font-weight: 650; }
.queue-toggle { display: inline-flex; align-items: center; gap: 3px; flex-shrink: 0; }
.queue-toast .text-button { color: var(--purple); font-size: 11px; white-space: nowrap; }
.queue-lanes { display: grid; gap: 12px; margin-top: 12px; }
.queue-lane { display: grid; grid-template-columns: 18px minmax(0, 1fr); gap: 8px 10px; }
.queue-lane + .queue-lane { padding-top: 12px; border-top: 1px solid var(--line); }
.queue-icon { margin-top: 1px; }
.queue-copy { display: grid; gap: 5px; min-width: 0; }
.queue-copy strong { font-size: 12px; font-weight: 550; }
.queue-copy span { font-size: 11px; color: var(--muted); }
.queue-current { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
.queue-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.queue-progress { flex-shrink: 0; white-space: nowrap; }
.queue-count { white-space: nowrap; }
.queue-wait-reason { overflow-wrap: anywhere; }
.queue-actions { grid-column: 2; display: flex; justify-content: flex-end; align-items: center; gap: 14px; }
.is-collapsed { width: fit-content; }
.is-collapsed .queue-header { gap: 10px; }
@media (max-width: 700px) {
  .queue-toast { left: 15px; right: 15px; bottom: calc(139px + env(safe-area-inset-bottom)); width: auto; padding: 12px 13px; }
  .queue-toast.is-collapsed { left: auto; width: fit-content; }
}
</style>
