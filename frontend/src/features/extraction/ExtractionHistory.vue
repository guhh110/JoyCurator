<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { name } from '../../api'
import { pageNumber } from '../../app/routing'
import { historyContext, historyStates, historyTime, lastHistoryPage, useHistoryList, useTraceRemoval } from './history'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import Pagination from '../../components/ui/Pagination.vue'
import EliText from '../../components/ui/EliText.vue'

const route = useRoute(), router = useRouter()
const page = computed(() => pageNumber(route.query.page))
const appliedPath = computed(() => typeof route.query.path === 'string' ? route.query.path : '')
const path = ref(appliedPath.value)
watch(appliedPath, value => { path.value = value })
const listQuery = useHistoryList(page, appliedPath)
const items = computed(() => listQuery.data.value?.items || [])
const total = computed(() => listQuery.data.value?.total || 0)
const { remove, pendingId, error } = useTraceRemoval(() => listQuery.refetch())
watch(() => listQuery.data.value, value => {
  if (value && page.value > lastHistoryPage(value.total)) {
    void router.replace({ name: 'extraction-history', query: { ...historyContext(route.query), page: lastHistoryPage(value.total) } })
  }
})
function search() {
  void router.push({ name: 'extraction-history', query: path.value ? { path: path.value } : {} })
}
function changePage(value: number) {
  void router.push({ name: 'extraction-history', query: { ...historyContext(route.query), page: value } })
}
</script>

<template>
  <section class="history-page" aria-label="识别历史">
    <PageHeader title="识别历史" eyebrow="RECOGNITION HISTORY" description="查看文件的识别结果与规则执行轨迹。">
      <template #actions><RouterLink class="button" :to="{ name: 'automatic-recognition' }">返回整理工作区</RouterLink></template>
    </PageHeader>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <form class="history-search" @submit.prevent="search">
      <input v-model="path" placeholder="输入完整文件路径精确筛选" aria-label="历史路径筛选" />
      <button class="button" :disabled="listQuery.isFetching.value">查询</button>
    </form>
    <QueryState :loading="listQuery.isPending.value" :error="listQuery.error.value" @retry="listQuery.refetch()">
      <section class="pane">
        <p v-if="!items.length" class="history-empty muted">{{ appliedPath ? '没有符合此路径的识别历史。' : '暂无识别历史。' }}</p>
        <article v-for="item in items" :key="item.id" class="trace-row">
          <div class="trace-info">
            <RouterLink class="trace-name" :to="{ name: 'extraction-history-detail', params: { traceId: item.id }, query: historyContext(route.query) }">{{ name(item.path) }}</RouterLink>
            <EliText :text="item.path" class="trace-path" />
            <p class="trace-meta"><span class="state-pill" :class="item.status">{{ historyStates[item.status] || item.status }}</span><span>{{ historyTime(item.startedAt) }}</span><span>{{ item.stepCount }} 步</span></p>
          </div>
          <div class="trace-actions">
            <RouterLink class="text-button" :to="{ name: 'extraction-history-detail', params: { traceId: item.id }, query: historyContext(route.query) }">查看过程</RouterLink>
            <button v-if="item.status !== 'running'" class="text-button danger" :disabled="!!pendingId" @click="remove(item)">{{ pendingId === item.id ? '处理中…' : '清理' }}</button>
          </div>
        </article>
        <Pagination :page="page" :total="total" :disabled="listQuery.isFetching.value || !!pendingId" @change="changePage" />
      </section>
    </QueryState>
  </section>
</template>

<style scoped>
.history-page { display:grid; gap:18px; min-width:0; }
.history-search { display:flex; align-items:center; gap:12px; }
.history-search input { flex:1; min-width:0; }
.history-empty { padding:28px 20px; font-size:13px; }
.trace-row { display:flex; align-items:center; gap:20px; padding:18px 20px; border-bottom:1px solid var(--line); }
.trace-info { flex:1; min-width:0; display:grid; gap:6px; }
.trace-name { font-size:14px; font-weight:600; overflow-wrap:anywhere; }
.trace-name:hover { color:var(--purple); }
.trace-path { color:var(--muted); font-size:12px; }
.trace-meta { display:flex; align-items:center; flex-wrap:wrap; gap:8px 12px; color:var(--muted); font-size:12px; }
.trace-actions { display:flex; gap:18px; flex-shrink:0; }
.trace-actions .danger { color:var(--red); }
@media(max-width:600px) {
  .trace-row { align-items:stretch; flex-direction:column; gap:10px; padding:16px; }
  .trace-actions { justify-content:flex-end; }
}
</style>
