<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import {
  ChevronRight,
  Folder,
  History,
  Undo2,
} from '@lucide/vue'
import { api, type Run } from '../api'
import { pageNumber } from '../app/routing'
import { keys } from '../app/query'
import { useRuntime } from '../app/runtime'
import { date, modeNames, states } from '../features/records/presentation'
import PageHeader from '../components/ui/PageHeader.vue'
import QueryState from '../components/ui/QueryState.vue'
import Pagination from '../components/ui/Pagination.vue'
const route = useRoute(),
  router = useRouter(),
  runtime = useRuntime()
const page = computed(() => pageNumber(route.query.page))
const recordsQuery = useQuery({
  queryKey: computed(() => keys.runs(page.value)),
  queryFn: ({ signal }) =>
    api<{ items: Run[]; total: number }>(
      `/runs?page=${page.value}`,
      undefined,
      'GET',
      { signal },
    ),
  staleTime: 0,
  refetchInterval: (query) =>
    runtime.executing.value ||
    query.state.data?.items.some((item) =>
      ['running', 'queued'].includes(item.state),
    )
      ? 1500
      : false,
})
const records = computed(() => recordsQuery.data.value?.items ?? [])
function changePage(value: number) {
  void router.push({ name: 'records', query: { page: value } })
}
</script>
<template>
  <section aria-label="整理记录">
    <PageHeader
      title="整理记录"
      eyebrow="COLLECTION JOURNAL"
      description="查看结果、恢复中断，或把文件放回原处。"
      ><template #actions
        ><button class="button small" @click="recordsQuery.refetch()">
          刷新记录
        </button></template
      ></PageHeader
    ><QueryState
      :loading="recordsQuery.isPending.value"
      :error="recordsQuery.error.value"
      @retry="recordsQuery.refetch()"
      ><div v-if="!records.length" class="pane empty record-empty">
        <History :size="38" />
        <h3>还没有整理记录</h3>
        <p>完成第一批整理后，结果会保存在这里。</p>
        <RouterLink class="button" :to="{ name: 'automatic-recognition' }"
          >前往工作台</RouterLink
        >
      </div>
      <div v-else class="records-list">
        <article v-for="run in records" :key="run.id" class="record-card">
          <span class="record-symbol"
            ><Undo2 v-if="run.state === 'undone'" :size="21" /><Folder
              v-else
              :size="21" /></span
          ><button class="record-main" @click="runtime.inspect(run)">
            <h3>{{ run.title }}</h3>
            <p>
              {{ date(run.created) }}<span>·</span>{{ modeNames[run.mode]
              }}<span>·</span>{{ run.completed }}/{{ run.total }} 项
            </p></button
          ><span class="state-pill" :class="run.state">{{
            run.undo && run.state === 'running' ? '撤销中' : states[run.state]
          }}</span
          ><button
            class="icon-button"
            aria-label="查看整理详情"
            @click="runtime.inspect(run)"
          >
            <ChevronRight :size="18" />
          </button>
        </article>
      </div>
      <Pagination
        v-if="(recordsQuery.data.value?.total ?? 0) > 20"
        :page="page"
        :total="recordsQuery.data.value?.total ?? 0"
        :disabled="recordsQuery.isFetching.value"
        @change="changePage"
    /></QueryState>
  </section>
</template>
