<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { ArrowRight, Clock3, FileImage, RotateCcw, Layers3 } from '@lucide/vue'
import { api, type Run } from '../api'
import { useRuntime } from '../app/runtime'
import PageHeader from '../components/ui/PageHeader.vue'
import QueryState from '../components/ui/QueryState.vue'
import { date, states } from '../features/records/presentation'
const runtime = useRuntime()
const page = ref(1)
const records = useQuery({ queryKey: computed(() => ['scrape','runs',page.value]), queryFn: ({ signal }) => api<{items:Run[];total:number}>(`/scrape/runs?page=${page.value}`,undefined,'GET',{signal}),refetchInterval:1500 })
const error = ref('')
const busy = ref(false)
async function action(run: Run, kind: string) {
  busy.value = true; error.value = ''
  try {
    await api(`/scrape/runs/${run.id}/${kind}`,{})
    await records.refetch()
  } catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
</script>
<template>
  <section aria-label="刮削记录">
    <PageHeader title="刮削记录" eyebrow="SCRAPE JOURNAL" description="单独查看、重试和撤销 NFO/图片任务。" />
    <QueryState :loading="records.isPending.value" :error="records.error.value" @retry="records.refetch()">
      <div v-if="!records.data.value?.items.length" class="pane records-empty"><span class="empty-icon"><FileImage :size="32" :stroke-width="1.5" /></span><span class="eyebrow">THE JOURNAL IS CLEAR</span><h2>还没有刮削记录</h2><p>开始刮削后，每次任务的进度、失败项与撤销入口都会保留在这里。</p><RouterLink class="button primary" to="/scrape">前往媒体刮削 <ArrowRight :size="15" /></RouterLink></div>
      <template v-else>
        <div class="record-heading"><div><span class="eyebrow">SCRAPE ACTIVITY</span><h2>最近的刮削任务</h2><p>图片和元数据的每次写入都有独立记录。</p></div><span class="record-count"><Layers3 :size="16" /> {{ records.data.value?.total }} 次任务</span></div>
        <article v-for="run in records.data.value?.items" :key="run.id" class="pane scrape-record">
          <div class="record-emblem"><FileImage :size="22" :stroke-width="1.6" /></div>
          <div class="record-body"><div class="record-title"><strong>{{ run.title }}</strong><span class="record-state" :class="run.state">{{ states[run.state] ?? run.state }}</span></div><div class="record-meta"><span><Clock3 :size="13" />{{ date(run.created) }}</span><span>{{ run.completed }} / {{ run.total }} 项已处理</span><span v-if="run.undo">撤销任务</span></div><p v-if="run.error" class="record-error">{{ run.error }}</p></div>
          <div class="scrape-record-actions"><button class="text-button" @click="runtime.inspect(run)">查看详情 <ArrowRight :size="14" /></button><button v-if="run.state === 'running'" class="button small" :disabled="busy" @click="action(run,'cancel')">取消</button><button v-if="['failed','cancelled','interrupted'].includes(run.state) && !run.undo" class="button small" :disabled="busy" @click="action(run,'retry')"><RotateCcw :size="13" />重试</button><button v-if="!['running','queued','undone'].includes(run.state)" class="button small" :disabled="busy" @click="runtime.inspect(run)">撤销…</button></div>
        </article>
      </template>
      <div v-if="(records.data.value?.total ?? 0)>20"><button class="button small" :disabled="page===1" @click="page--">上一页</button><button class="button small" :disabled="page*20>=(records.data.value?.total ?? 0)" @click="page++">下一页</button></div>
      <p v-if="error" class="notice error">{{ error }}</p>
    </QueryState>
  </section>
</template>
<style scoped>
.eyebrow { color:var(--purple-dark);font-size:10px;font-weight:750;letter-spacing:.15em }
.records-empty { display:flex;flex-direction:column;align-items:center;justify-content:center;gap:12px;min-height:320px;padding:32px;text-align:center;background:radial-gradient(circle at 50% 0,#f4f0fc,#fff 68%) }
.empty-icon { display:grid;place-items:center;width:70px;height:70px;border-radius:18px;background:#eee9fa;color:var(--purple);box-shadow:0 12px 27px #6554ad16 }
.records-empty h2 { font-size:20px }.records-empty p { max-width:380px;color:var(--muted);font-size:12px }
.record-heading { display:flex;justify-content:space-between;align-items:center;gap:16px;margin:16px 0 }
.record-heading h2 { margin:5px 0 2px;font-size:19px }.record-heading p { color:var(--muted);font-size:11px }
.record-count { display:inline-flex;align-items:center;gap:7px;padding:8px 11px;border:1px solid #e7e2f0;border-radius:8px;background:#fff;color:var(--purple);font-size:11px;font-weight:700;white-space:nowrap }
.scrape-record { display:flex;align-items:center;gap:14px;margin-bottom:10px;padding:17px }
.record-emblem { display:grid;place-items:center;flex:none;width:48px;height:48px;border-radius:10px;background:linear-gradient(145deg,#eee9fa,#faf8ff);color:var(--purple) }
.record-body { flex:1;min-width:0;display:grid;gap:6px }.record-title { display:flex;align-items:center;gap:10px;flex-wrap:wrap }.record-title strong { font-size:14px }
.record-state { padding:4px 7px;border-radius:5px;background:#f1eef9;color:#705bae;font-size:10px;font-weight:700 }.record-state.failed,.record-state.cancelled { background:#fdf0f0;color:var(--red) }.record-state.completed { background:#edf7f2;color:var(--green) }
.record-meta { display:flex;align-items:center;gap:13px;flex-wrap:wrap;color:var(--muted);font-size:10px }.record-meta span { display:inline-flex;align-items:center;gap:4px }
.record-error { color:var(--red);font-size:10px;white-space:pre-line;overflow-wrap:anywhere }
.scrape-record-actions { display:flex;align-items:center;justify-content:flex-end;gap:7px;flex-wrap:wrap }.scrape-record-actions .text-button { display:inline-flex;align-items:center;gap:4px;color:var(--purple);font-size:11px;font-weight:700 }
@media(max-width:700px) { .scrape-record { align-items:flex-start;flex-wrap:wrap }.scrape-record-actions { width:100%;justify-content:flex-start;margin-left:62px }.record-heading { align-items:flex-start;flex-direction:column } }
</style>
