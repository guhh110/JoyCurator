<script setup lang="ts">
import { useDataMaintenance } from './useDataMaintenance'
const { before, maxDate, traces, cache, preview, count, busy, inspect, cleanup } = useDataMaintenance()
</script>

<template>
  <section class="data-maintenance" aria-label="数据清理">
    <h2>清理历史与缓存</h2>
    <p class="muted">清理已结束的识别轨迹及其步骤，或已过期的 TMDB 缓存。工作区结果、整理记录和媒体文件会保留。</p>
    <form @submit.prevent="inspect">
      <label class="maintenance-option"><input v-model="traces" type="checkbox" :disabled="busy" />识别历史</label>
      <label>清理此日期之前结束的历史
        <input v-model="before" type="date" :max="maxDate" :required="traces" :disabled="busy || !traces" />
      </label>
      <label class="maintenance-option"><input v-model="cache" type="checkbox" :disabled="busy" />过期 TMDB 缓存</label>
      <p class="muted tiny">运行中的识别记录不会被清理。清理后数据库空间可供后续写入复用，文件体积不一定立即缩小。</p>
      <div class="maintenance-actions"><button class="button" :disabled="busy || (!traces && !cache)">{{ busy ? '处理中…' : '预览清理数量' }}</button></div>
    </form>
    <div v-if="preview" class="maintenance-preview">
      <p>预计清理 <strong>{{ preview.traces }}</strong> 条识别历史、<strong>{{ preview.cache }}</strong> 条过期缓存。</p>
      <div class="maintenance-actions"><button class="button danger" :disabled="busy || !count" @click="cleanup">确认清理…</button></div>
    </div>
  </section>
</template>

<style scoped>
.data-maintenance, .data-maintenance form { display:grid; gap:16px; min-width:0; }
.data-maintenance h2, .data-maintenance p { margin:0; }
.maintenance-option { display:flex; align-items:center; gap:8px; }
.maintenance-option input { width:auto; }
.data-maintenance input[type='date'] { max-width:240px; }
.maintenance-actions { display:flex; justify-content:flex-end; }
.maintenance-preview { display:grid; gap:16px; padding-top:16px; border-top:1px solid var(--line); }
</style>
