<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../../api'
import type { ExtractionResult, Trace } from './model'
import ExtractionOutput from './ExtractionResult.vue'
const props = defineProps<{ traceId?: string }>()
defineEmits<{ back: [] }>()
const page = ref(1),
  total = ref(0),
  items = ref<Trace[]>([]),
  error = ref(''),
  busy = ref(false),
  path = ref('')
const detail = ref<ExtractionResult & { trace: Trace }>(),
  deleting = ref('')
const stateNames: Record<string, string> = {
  completed: '已完成',
  running: '运行中',
  cancelled: '已取消',
  interrupted: '已中断',
  failed: '失败',
}
function search() {
  page.value = 1
  void load()
}
function changePage(delta: number) {
  page.value += delta
  void load()
}
async function load() {
  busy.value = true
  error.value = ''
  try {
    const data = await api<{ items: Trace[]; total: number }>(
      `/extraction/traces?page=${page.value}&path=${encodeURIComponent(path.value)}`,
    )
    items.value = data.items
    total.value = data.total
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
async function open(id: string) {
  error.value = ''
  deleting.value = ''
  try {
    detail.value = await api(`/extraction/traces/${encodeURIComponent(id)}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}
async function remove(id: string) {
  error.value = ''
  try {
    await api(
      `/extraction/traces/${encodeURIComponent(id)}`,
      undefined,
      'DELETE',
    )
    if (detail.value?.trace.id === id) detail.value = undefined
    deleting.value = ''
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}
onMounted(async () => {
  await load()
  if (props.traceId) await open(props.traceId)
})
watch(
  () => props.traceId,
  (id) => {
    if (id) void open(id)
  },
)
</script>
<template>
  <section class="history-page" aria-label="规则执行历史">
    <header class="heading">
      <div>
        <span class="eyebrow">EXTRACTION HISTORY</span>
        <h1>规则执行历史<span class="heading-dot">.</span></h1>
        <p class="muted">长期保存正式提取过程；历史规则配置随步骤留存。</p>
      </div>
      <button class="button" @click="$emit('back')">返回自动识别</button>
    </header>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <form class="search" @submit.prevent="search">
      <input
        v-model="path"
        placeholder="按完整文件路径筛选"
        aria-label="历史路径筛选"
      /><button class="button" :disabled="busy">查询</button>
    </form>
    <section class="pane">
      <p v-if="!items.length" class="padding muted">
        暂无规则执行轨迹。旧识别结果不会自动补写轨迹。
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
        <div v-if="deleting === item.id" class="confirm">
          <p>清理这条路径的历史执行轨迹？工作区识别结果会保留。</p>
          <button class="button small" @click="deleting = ''">保留</button
          ><button class="button small danger" @click="remove(item.id)">
            确认清理
          </button>
        </div>
      </article>
      <footer class="pagination">
        <button
          class="button small"
          :disabled="page <= 1 || busy"
          @click="changePage(-1)"
        >
          上一页</button
        ><span class="tiny muted"
          >{{ page }} / {{ Math.max(1, Math.ceil(total / 20)) }} · 共
          {{ total }} 条</span
        ><button
          class="button small"
          :disabled="page * 20 >= total || busy"
          @click="changePage(1)"
        >
          下一页
        </button>
      </footer>
    </section>
    <template v-if="detail"
      ><section class="pane padding">
        <h2>本次执行</h2>
        <p class="trace-path">{{ detail.trace.path }}</p>
        <p class="tiny muted">
          边界 {{ detail.trace.root }} ·
          {{ stateNames[detail.trace.status] || detail.trace.status }}
        </p>
        <p v-if="detail.trace.error" class="red">{{ detail.trace.error }}</p>
      </section>
      <ExtractionOutput :result="detail"
    /></template>
  </section>
</template>
<style scoped>
.history-page {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.heading,
.search,
.pagination {
  display: flex;
  align-items: center;
  gap: 12px;
}
.heading {
  justify-content: space-between;
}
.heading h1 {
  margin: 8px 0;
}
.search input {
  flex: 1;
  min-width: 0;
}
.padding {
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
.pagination {
  padding: 16px;
  justify-content: center;
}
.confirm {
  flex-basis: 100%;
  font-size: 13px;
}
.confirm button {
  margin-right: 8px;
}
h2 {
  font-size: 17px;
  margin: 0;
}
@media (max-width: 600px) {
  .heading {
    align-items: flex-start;
    flex-direction: column;
  }
  .trace-row {
    padding: 16px;
  }
}
</style>
