<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useRecords } from './queries'
import {
  ChevronRight,
  Folder,
  FileImage,
  History,
  Undo2,
} from '@lucide/vue'
import { pageNumber } from '../../app/routing'
import { useRuntime } from '../../app/runtime'
import { date, modeNames, states, recordStateOptions, recordState } from './presentation'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import Select from '../../components/ui/Select.vue'
import Pagination from '../../components/ui/Pagination.vue'
const props = defineProps<{ purpose: 'organize' | 'scrape' }>()
const pageCopy = computed(() => props.purpose === 'scrape'
  ? {
      title: '刮削记录', eyebrow: 'SCRAPE JOURNAL',
      description: '查看、重试和撤销 NFO/图片任务。',
      emptyDescription: '提交刮削后，任务进度和结果会保存在这里。',
      destination: 'scrape', destinationLabel: '前往媒体刮削',
      detailsLabel: '查看刮削详情', route: 'scrape-records',
    }
  : {
      title: '整理记录', eyebrow: 'COLLECTION JOURNAL',
      description: '查看结果、恢复中断，或把文件放回原处。',
      emptyDescription: '提交整理后，任务进度和结果会保存在这里。',
      destination: 'automatic-recognition', destinationLabel: '前往工作台',
      detailsLabel: '查看整理详情', route: 'records',
    })
const route = useRoute(),
  router = useRouter(),
  runtime = useRuntime()
const page = computed(() => pageNumber(route.query.page))
const appliedSearch = computed(() => typeof route.query.search === 'string' ? route.query.search : '')
const state = computed(() => recordState(route.query.state))
const search = ref(appliedSearch.value)
watch(appliedSearch, value => { search.value = value })
const filtered = computed(() => !!appliedSearch.value || !!state.value)
const recordsQuery = useRecords(page, props.purpose, appliedSearch, state)
const records = computed(() => recordsQuery.data.value?.items ?? [])
function changePage(value: number) {
  void router.push({ name: pageCopy.value.route, query: { ...route.query, page: value } })
}
function applyFilter(value = state.value) {
  void router.push({ name: pageCopy.value.route, query: {
    ...(search.value.trim() ? { search: search.value.trim() } : {}),
    ...(value ? { state: value } : {}),
  } })
}
watch(() => recordsQuery.data.value, value => {
  const last = Math.max(1, Math.ceil((value?.total ?? 0) / 20))
  if (value && page.value > last) void router.replace({ query: { ...route.query, page: last } })
})
</script>
<template>
  <section :aria-label="pageCopy.title">
    <PageHeader
      :title="pageCopy.title"
      :eyebrow="pageCopy.eyebrow"
      :description="pageCopy.description"
      ><template #actions
        ><button class="button small" @click="recordsQuery.refetch()">
          刷新记录
        </button></template
      ></PageHeader
    >
    <form class="record-filters" @submit.prevent="applyFilter()">
      <input v-model="search" type="search" placeholder="搜索作品名称" aria-label="记录作品搜索" />
      <Select :model-value="state" :options="recordStateOptions" label="记录状态筛选" @update:model-value="applyFilter" />
      <button class="button small" type="submit">查询</button>
    </form>
    <QueryState
      :loading="recordsQuery.isPending.value"
      :error="recordsQuery.error.value"
      @retry="recordsQuery.refetch()"
      ><div v-if="!records.length" class="pane empty record-empty">
        <History :size="38" />
        <h3>{{ filtered ? '没有符合条件的记录' : `还没有${pageCopy.title}` }}</h3>
        <p>{{ filtered ? '试试其他作品名称或任务状态。' : pageCopy.emptyDescription }}</p>
        <button v-if="filtered" class="button" @click="router.push({ name: pageCopy.route })">清除筛选</button>
        <RouterLink v-else class="button" :to="{ name: pageCopy.destination }"
          >{{ pageCopy.destinationLabel }}</RouterLink
        >
      </div>
      <div v-else class="records-list">
        <article v-for="run in records" :key="run.id" class="record-card">
          <span class="record-symbol"
            ><Undo2 v-if="run.state === 'undone'" :size="21" /><FileImage
              v-else-if="purpose === 'scrape'"
              :size="21" /><Folder
              v-else
              :size="21" /></span
          ><button class="record-main" @click="runtime.inspect(run)">
            <h3>{{ run.title }}</h3>
            <p>
              {{ date(run.created) }}<span>·</span>{{ modeNames[run.mode]
              }}<span>·</span>{{ run.completed }}/{{ run.total }} 项
            </p></button
          ><span class="state-pill" :class="run.state">{{
            run.state === 'queued'
              ? run.undo ? '撤销排队中' : '排队中'
              : run.undo && run.state === 'running' ? '撤销中' : states[run.state]
          }}</span
          ><button
            class="icon-button"
            :aria-label="pageCopy.detailsLabel"
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

<style scoped>
.record-filters { display:flex; align-items:center; gap:10px; margin-bottom:18px; }
.record-filters input { flex:1; min-width:0; }
.record-filters .button, .record-filters :deep(.ui-select-anchor) { flex-shrink:0; }
@media(max-width:520px) { .record-filters { flex-wrap:wrap; } .record-filters input { flex-basis:100%; } }
</style>
