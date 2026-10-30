<script setup lang="ts">
import { computed, ref, toRef } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { name } from '../../api'
import { historyContext, historyStates, historyTime, useHistoryDetail, useTraceRemoval } from './history'
import ExtractionOutput from './ExtractionResult.vue'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import EliText from '../../components/ui/EliText.vue'

const props = defineProps<{ traceId: string }>()
const route = useRoute(), router = useRouter()
const detailQuery = useHistoryDetail(toRef(props, 'traceId'))
const detail = detailQuery.data
const back = computed(() => ({ name: 'extraction-history', query: historyContext(route.query) }))
const { remove, pendingId } = useTraceRemoval(() => router.replace(back.value))
const view = ref<'steps' | 'metadata' | 'json'>('steps')
const tabs = [{ value: 'steps', label: '执行过程' }, { value: 'metadata', label: '提取结果' }, { value: 'json', label: '原始 JSON' }] as const
</script>

<template>
  <section class="history-detail" aria-label="识别历史详情">
    <PageHeader title="识别历史详情" eyebrow="RECOGNITION TRACE" description="本次执行的结果与规则快照。">
      <template #actions>
        <RouterLink class="button" :to="back">返回识别历史</RouterLink>
        <button v-if="detail && !detailQuery.error.value && detail.trace.status !== 'running'" class="button danger" :disabled="!!pendingId" @click="remove(detail.trace)">{{ pendingId ? '处理中…' : '清理本条历史' }}</button>
      </template>
    </PageHeader>
    <QueryState :loading="detailQuery.isPending.value" :error="detailQuery.error.value" @retry="detailQuery.refetch()">
      <template v-if="detail">
        <section class="pane trace-summary">
          <div class="trace-summary-heading"><h2>{{ name(detail.trace.path) }}</h2><span class="state-pill" :class="detail.trace.status">{{ historyStates[detail.trace.status] || detail.trace.status }}</span></div>
          <EliText :text="detail.trace.path" ellipsis="start" class="trace-path" />
          <dl>
            <div><dt>开始时间</dt><dd>{{ historyTime(detail.trace.startedAt) }}</dd></div>
            <div><dt>结束时间</dt><dd>{{ historyTime(detail.trace.finishedAt) }}</dd></div>
            <div><dt>执行步骤</dt><dd>{{ detail.trace.stepCount }} 步</dd></div>
            <div class="trace-root"><dt>解析边界</dt><dd><EliText :text="detail.trace.root || '—'" ellipsis="start" /></dd></div>
          </dl>
          <p v-if="detail.trace.error" class="notice error" role="alert">{{ detail.trace.error }}</p>
        </section>
        <div class="history-tabs" role="group" aria-label="历史详情内容">
          <button v-for="tab in tabs" :key="tab.value" :aria-pressed="view === tab.value" @click="view = tab.value">{{ tab.label }}</button>
        </div>
        <ExtractionOutput :key="traceId" :result="detail" :view="view" />
      </template>
    </QueryState>
  </section>
</template>

<style scoped>
.history-detail { display:grid; gap:18px; min-width:0; }
.trace-summary { padding:20px; display:grid; gap:12px; }
.trace-summary-heading { display:flex; gap:12px; align-items:center; flex-wrap:wrap; }
.trace-summary h2 { font-size:17px; overflow-wrap:anywhere; }
.trace-path { font-size:12px; color:var(--muted); }
.trace-summary dl { display:flex; gap:18px 32px; flex-wrap:wrap; margin:0; }
.trace-summary dl > div { min-width:0; }
.trace-summary dt { font-size:11px; color:var(--muted); margin-bottom:4px; }
.trace-summary dd { margin:0; font-size:12px; }
.trace-root { flex:1 1 240px; }
.history-tabs { display:flex; gap:6px; border-bottom:1px solid var(--line); padding-bottom:8px; }
.history-tabs button { border:0; border-radius:7px; padding:8px 14px; font-size:13px; background:none; color:var(--muted); }
.history-tabs button[aria-pressed='true'] { background:#eee9fc; color:var(--purple-dark); font-weight:600; }
@media(max-width:600px) { .trace-summary { padding:16px; } }
</style>
