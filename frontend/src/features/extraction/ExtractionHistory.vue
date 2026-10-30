<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { api } from '../../api'
import { pageNumber } from '../../app/routing'
import { keys } from '../../app/query'
import type { ExtractionResult, Trace } from './model'
import ExtractionOutput from './ExtractionResult.vue'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import Pagination from '../../components/ui/Pagination.vue'
const props = defineProps<{ traceId?: string }>()
const route = useRoute(),
  router = useRouter(),
  client = useQueryClient()
const page = computed(() => pageNumber(route.query.page))
const appliedPath = computed(() => String(route.query.path || '')),
  path = ref(appliedPath.value),
  error = ref(''),
  deleting = ref('')
watch(appliedPath, (value) => {
  path.value = value
})
const stateNames: Record<string, string> = {
  completed: '已完成',
  running: '运行中',
  cancelled: '已取消',
  interrupted: '已中断',
  failed: '失败',
}
const listQuery = useQuery({
  queryKey: computed(() => [...keys.traces, page.value, appliedPath.value]),
  queryFn: ({ signal }) =>
    api<{ items: Trace[]; total: number }>(
      `/extraction/traces?page=${page.value}&path=${encodeURIComponent(appliedPath.value)}`,
      undefined,
      'GET',
      { signal },
    ),
  staleTime: 0,
  refetchInterval: (query) =>
    query.state.data?.items.some((item) => item.status === 'running')
      ? 1500
      : false,
})
const detailQuery = useQuery({
  queryKey: computed(() => keys.trace(props.traceId || '')),
  enabled: computed(() => !!props.traceId),
  queryFn: ({ signal }) =>
    api<ExtractionResult & { trace: Trace }>(
      `/extraction/traces/${encodeURIComponent(props.traceId!)}`,
      undefined,
      'GET',
      { signal },
    ),
  staleTime: 0,
  refetchInterval: (query) =>
    query.state.data?.trace.status === 'running' ? 1200 : false,
})
const items = computed(() => listQuery.data.value?.items ?? []),
  total = computed(() => listQuery.data.value?.total ?? 0),
  detail = detailQuery.data
const removeMutation = useMutation({
  mutationFn: (id: string) =>
    api(`/extraction/traces/${encodeURIComponent(id)}`, undefined, 'DELETE'),
})
function search() {
  void router.push({
    name: 'extraction-history',
    params: { traceId: undefined },
    query: { ...(path.value ? { path: path.value } : {}) },
  })
}
function changePage(value: number) {
  void router.push({
    name: 'extraction-history',
    params: { traceId: props.traceId },
    query: {
      page: value,
      ...(appliedPath.value ? { path: appliedPath.value } : {}),
    },
  })
}
function open(id: string) {
  deleting.value = ''
  void router.push({
    name: 'extraction-history',
    params: { traceId: id },
    query: route.query,
  })
}
async function remove(id: string) {
  error.value = ''
  try {
    await removeMutation.mutateAsync(id)
    client.removeQueries({ queryKey: keys.trace(id) })
    deleting.value = ''
    if (props.traceId === id)
      await router.replace({
        name: 'extraction-history',
        params: { traceId: undefined },
        query: route.query,
      })
    await client.invalidateQueries({ queryKey: keys.traces })
  } catch (cause) {
    error.value = (cause as Error).message
  }
}
</script>
<template>
  <section class="history-page" aria-label="规则执行历史">
    <PageHeader
      title="执行历史"
      eyebrow="EXTRACTION HISTORY"
      description="长期保存正式提取过程；历史规则配置随步骤留存。"
      ><template #actions
        ><RouterLink class="button" :to="{ name: 'automatic-recognition' }"
          >返回整理工作区</RouterLink
        ></template
      ></PageHeader
    >
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <form class="history-search" @submit.prevent="search">
      <input
        v-model="path"
        placeholder="按完整文件路径筛选"
        aria-label="历史路径筛选"
      /><button class="button" :disabled="listQuery.isFetching.value">
        查询
      </button>
    </form>
    <QueryState
      :loading="listQuery.isPending.value"
      :error="listQuery.error.value"
      @retry="listQuery.refetch()"
      ><section class="pane">
        <p v-if="!items.length" class="history-padding muted">
          暂无规则执行轨迹。
        </p>
        <article v-for="item in items" :key="item.id" class="trace-row">
          <div>
            <strong>{{ item.path }}</strong>
            <p class="tiny muted">
              {{ new Date(item.startedAt).toLocaleString() }} ·
              {{ stateNames[item.status] || item.status }} ·
              {{ item.stepCount }} 步
            </p>
          </div>
          <button class="text-button" @click="open(item.id)">查看过程</button
          ><button
            v-if="item.status !== 'running'"
            class="text-button red"
            @click="deleting = item.id"
          >
            清理
          </button>
          <div v-if="deleting === item.id" class="history-confirm">
            <p>清理这条路径的历史执行轨迹？工作区识别结果会保留。</p>
            <button class="button small" @click="deleting = ''">保留</button
            ><button
              class="button small danger"
              :disabled="removeMutation.isPending.value"
              @click="remove(item.id)"
            >
              确认清理
            </button>
          </div>
        </article>
        <Pagination
          :page="page"
          :total="total"
          :disabled="listQuery.isFetching.value"
          @change="changePage"
        /></section></QueryState
    ><QueryState
      v-if="traceId"
      :loading="detailQuery.isPending.value"
      :error="detailQuery.error.value"
      @retry="detailQuery.refetch()"
      ><template v-if="detail"
        ><section class="pane history-padding">
          <h2>本次执行</h2>
          <p class="trace-path">{{ detail.trace.path }}</p>
          <p class="tiny muted">
            边界 {{ detail.trace.root }} ·
            {{ stateNames[detail.trace.status] || detail.trace.status }}
          </p>
          <p v-if="detail.trace.error" class="red">{{ detail.trace.error }}</p>
        </section>
        <ExtractionOutput :result="detail" /></template
    ></QueryState>
  </section>
</template>
<style scoped>
.history-page {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.history-search {
  display: flex;
  align-items: center;
  gap: 12px;
}
.history-search input {
  flex: 1;
  min-width: 0;
}
.history-padding {
  padding: 20px;
}
.trace-row {
  display: flex;
  gap: 16px;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--line);
  flex-wrap: wrap;
}
.trace-row > div:first-child {
  flex: 1;
  min-width: 180px;
}
.trace-row strong,
.trace-path {
  overflow-wrap: anywhere;
  font-size: 13px;
}
.trace-row button {
  flex-shrink: 0;
}
.history-confirm {
  flex-basis: 100%;
  font-size: 13px;
}
.history-confirm button {
  margin-right: 8px;
}
h2 {
  font-size: 17px;
  margin: 0;
}
@media (max-width: 600px) {
  .trace-row {
    padding: 16px;
  }
}
</style>
