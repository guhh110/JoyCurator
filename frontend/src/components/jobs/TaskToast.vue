<script setup lang="ts">
import { Clock3, CircleCheck, LoaderCircle, X } from '@lucide/vue'
import { useRuntime } from '../../app/runtime'
import { states } from '../../features/records/presentation'
const { active, activeId, inspect, recordAction } = useRuntime()
</script>
<template>
  <div
    v-if="active"
    class="task-toast"
    :class="{ finished: active.state !== 'running' }"
  >
    <LoaderCircle
      v-if="active.state === 'running'"
      class="spin purple"
      :size="19"
    /><CircleCheck
      v-else-if="active.state === 'completed' || active.state === 'undone'"
      :size="19"
      class="green"
    /><Clock3 v-else :size="19" />
    <div>
      <strong
        >{{ active.undo ? '撤销' : active.purpose === 'scrape' ? '刮削' : '整理' }}{{ states[active.state] }}</strong
      ><span
        >{{ active.title }} · {{ active.completed }}/{{ active.total }} 项</span
      >
    </div>
    <button
      v-if="active.state === 'running'"
      class="text-button"
      @click="recordAction(active, 'cancel')"
    >
      取消</button
    ><button v-else class="text-button" @click="inspect(active)">查看</button
    ><button
      v-if="active.state !== 'running'"
      class="icon-button"
      aria-label="关闭任务提示"
      @click="activeId = ''"
    >
      <X :size="15" />
    </button>
  </div>
</template>
