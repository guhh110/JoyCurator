<script setup lang="ts">
import { ref } from 'vue'
import { useRuntime } from '../../app/runtime'
import { useQueuedRecords } from './queries'
import { states } from './presentation'
import QueryState from '../../components/ui/QueryState.vue'
import Pagination from '../../components/ui/Pagination.vue'
const props = defineProps<{purpose: 'organize' | 'scrape'}>()
const page=ref(1), runtime=useRuntime()
const pending=useQueuedRecords(page,props.purpose)
</script>
<template>
  <details class="pane queue-records">
    <summary>排队任务 · {{ pending.data.value?.total ?? 0 }}</summary>
    <QueryState :loading="pending.isPending.value" :error="pending.error.value" @retry="pending.refetch()">
      <p v-if="!pending.data.value?.total" class="muted">暂无等待任务</p>
      <div v-for="(run,index) in pending.data.value?.items" :key="run.id" class="queued-record">
        <span>{{ (page-1)*20+index+1 }}. {{ run.title }} · {{ run.undo ? '待撤销' : states[run.state] }}</span>
        <button class="text-button" @click="runtime.inspect(run)">详情</button>
        <button class="text-button" :disabled="runtime.actionBusy.value" @click="runtime.recordAction(run,'cancel')">取消</button>
      </div>
      <Pagination v-if="(pending.data.value?.total ?? 0)>20" :page="page" :total="pending.data.value?.total ?? 0" @change="page=$event" />
    </QueryState>
  </details>
</template>
<style scoped>
.queue-records {padding:16px;margin-bottom:16px;}
summary {cursor:pointer;font-weight:650;}
.queued-record {display:flex;gap:12px;align-items:center;padding:10px 0;border-bottom:1px solid var(--line);}
.queued-record span {flex:1;min-width:0;overflow-wrap:anywhere;}
</style>
