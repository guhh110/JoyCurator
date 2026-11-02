<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { episodeCode, name } from '../../api'
import Modal from '../../components/Modal.vue'
import Select from '../../components/ui/Select.vue'
import EliText from '../../components/ui/EliText.vue'
import { Film, FolderOpen } from '@lucide/vue'
import type { useEpisodeOffset } from './useEpisodeOffset'

const props = defineProps<{ editor: ReturnType<typeof useEpisodeOffset>; locked: boolean }>()
const { context, source, sources, season, offsetInput, offset, rows, chosen, loading, saving,
  error, stale, allowOverlap, overlaps, canApply } = props.editor
const posterFailed = ref(false)
const eligible = computed(() => rows.value.filter(row => row.eligible))
const allSelected = computed(() => eligible.value.length > 0 && eligible.value.every(row => row.selected))
const disabled = computed(() => props.locked || saving.value || stale.value)
watch(context, () => { posterFailed.value = false })
const originalCode = (numbers: number[]) => numbers.length ? numbers.map(number => `E${String(number).padStart(2, '0')}`).join(', ') : '未识别'
</script>

<template>
  <Modal v-if="context" class="offset-modal" title="批量修正季集" wide :locked="saving" @close="editor.close">
    <div class="offset-editor">
      <div class="offset-setup">
      <section class="offset-scope" aria-label="修正范围">
        <span class="offset-poster" aria-hidden="true">
          <img v-if="context.posterPath && !posterFailed" :src="`https://image.tmdb.org/t/p/w154${context.posterPath}`" alt="" width="52" height="78" @error="posterFailed = true" />
          <Film v-else :size="22" class="offset-poster-placeholder" />
        </span>
        <div class="offset-scope-info">
        <div class="offset-context"><strong :title="context.title">{{ context.title }}</strong><span>{{ rows.length }} 个文件</span></div>
        <div v-if="sources.length > 1" class="offset-source-picker">
          <label for="offset-source">来源</label>
          <Select id="offset-source" v-model="source" label="修改来源" ellipsis="start" :disabled="disabled"
            :options="sources.map(path => ({ value: path, label: path || '未归属来源' }))" />
        </div>
        <details v-else class="offset-source-detail">
          <summary><FolderOpen :size="14" aria-hidden="true" /><span>来源：{{ name(source) || '未归属来源' }}</span><span class="offset-path-hint">查看路径</span></summary>
          <p><EliText :text="source || '未归属来源'" ellipsis="start" /></p>
        </details>
        </div>
      </section>
      <section class="offset-rule" aria-label="季集修正规则">
        <div class="offset-fields">
          <div class="offset-field offset-season">
            <label for="offset-season">目标季度</label>
            <Select id="offset-season" :model-value="String(season)" label="目标季度" :disabled="disabled"
              :options="context.seasons.map(item => ({ value: String(item.number), label: `${item.name} · ${item.count} 集` }))"
              @update:model-value="value => season = Number(value)" />
          </div>
          <div class="offset-field">
            <label for="offset-input">集数偏移</label>
            <div class="offset-input-wrap"><input id="offset-input" v-model="offsetInput" type="text" placeholder="例如 +9、-1 或 0" autocomplete="off" :spellcheck="false" aria-label="集数偏移" :aria-invalid="!Number.isSafeInteger(offset)" :aria-describedby="Number.isSafeInteger(offset) ? undefined : 'offset-error'" :disabled="disabled" /><span aria-hidden="true">集</span></div>
          </div>
        </div>
        <p v-if="!Number.isSafeInteger(offset)" id="offset-error" class="offset-input-error danger">请输入整数偏移量，例如 +9、-1 或 0</p>
      </section>
      </div>
      <section class="offset-preview" aria-label="文件修正预览">
      <div class="offset-toolbar">
        <h3>文件预览</h3><span role="status">已勾选 {{ chosen.length }} / {{ rows.length }}{{ loading ? ' · 校验中…' : '' }}</span>
      </div>
      <div class="offset-table" role="table" aria-label="季集修正预览">
        <div class="offset-row offset-head" role="row"><span role="columnheader"><label class="offset-check"><input type="checkbox" :checked="allSelected" :indeterminate="chosen.length > 0 && !allSelected" :disabled="disabled || !eligible.length" aria-label="选择本来源全部可修正文件" @change="editor.selectAll(($event.target as HTMLInputElement).checked)" /><span>文件</span></label></span><span role="columnheader">原始识别</span><span role="columnheader">当前映射</span><span role="columnheader">修正后</span></div>
        <div v-for="row in rows" :key="row.file.path" class="offset-row" :class="{ 'is-selected': row.selected }" role="row">
          <div role="cell" class="offset-file">
            <input type="checkbox" class="offset-checkbox" :checked="row.selected" :aria-label="`修正 ${row.file.path}`"
              :disabled="disabled || !row.eligible" @change="editor.select(row.file.path, ($event.target as HTMLInputElement).checked)" />
            <div class="offset-file-text">
              <strong>{{ name(row.file.path) }}</strong>
              <EliText :text="row.file.path" ellipsis="start" class="offset-file-path" />
            </div>
          </div>
          <span role="cell" class="offset-code" data-label="原始识别">{{ originalCode(row.file.metadata.episodes) }}</span>
          <div role="cell" data-label="当前映射">
            <span class="offset-result"><span class="offset-code">{{ row.file.season < 0 ? '季号未知' : episodeCode(row.file.season, row.file.episodes) }}</span><span v-if="row.file.season >= 0 && row.currentOffset !== null" class="offset-delta-badge" :class="{ neutral: row.currentOffset === 0 }" :title="`当前映射相对原始识别偏移 ${row.currentOffset > 0 ? '+' : ''}${row.currentOffset} 集`">{{ row.currentOffset > 0 ? '+' : '' }}{{ row.currentOffset }} 集</span></span>
          </div>
          <div role="cell" data-label="修正后" :class="{ danger: row.selected && row.error }">
            <span v-if="!row.eligible" class="muted tiny">{{ row.error }}</span>
            <span v-else-if="!row.selected" class="muted">不修改</span>
            <span v-else-if="row.error">{{ row.error }}</span>
            <template v-else>
              <span class="offset-result"><span class="offset-code offset-delta">{{ episodeCode(season, row.episodes) }}</span><span class="offset-delta-badge" :class="{ neutral: offset === 0 }" :title="offset === 0 ? '相对原始识别偏移 0 集，恢复原始集号' : `相对原始识别偏移 ${offset > 0 ? '+' : ''}${offset} 集`">{{ offset > 0 ? '+' : '' }}{{ offset }} 集</span></span>
              <p v-if="row.overlaps.length" class="tiny">同集映射：{{ row.overlaps.join('、') }}</p>
            </template>
          </div>
        </div>
      </div>
      <p class="offset-selection-note">仅修正勾选文件，不改变“参与整理”状态。</p>
      </section>
      <p v-if="stale" class="notice error" role="alert">工作区已更新，请关闭弹窗后重新打开，避免覆盖新的修正。</p>
      <label v-if="overlaps" class="offset-check offset-warning"><input v-model="allowOverlap" type="checkbox" :disabled="disabled" />已确认同集映射，允许保留多版本文件；目标路径冲突仍需在整理预览中处理。</label>
    </div>
    <template #footer><span class="offset-footer-note">仅保存映射，不执行文件整理</span><div class="offset-actions"><button class="button" :disabled="saving" @click="editor.close">取消</button><button class="button primary" :disabled="error && !stale ? saving || loading || locked : !canApply" @click="error && !stale ? editor.retry() : editor.apply()">{{ saving ? '保存并校验…' : error && !stale ? '重新校验' : chosen.length ? `应用到 ${chosen.length} 个文件` : '应用修正' }}</button></div></template>
  </Modal>
</template>

<style scoped>
.offset-modal.modal.wide { width:min(820px, calc(100% - 32px)); }
.offset-modal :deep(.modal-head) { padding:18px 22px 16px; }
.offset-modal :deep(.modal-head h2) { font-size:18px; margin-top:4px; }
.offset-modal :deep(.modal-body) { padding:20px 22px; }
.offset-modal :deep(.modal-foot) { padding:14px 22px; gap:12px; flex-wrap:wrap; }
.offset-editor { display:grid; gap:22px; }
.offset-setup { display:grid; grid-template-columns:minmax(0,1.1fr) minmax(0,1.4fr); align-items:center; gap:20px; padding:16px; background:#f8f7fb; border:1px solid var(--line); border-radius:10px; }
.offset-scope { display:grid; grid-template-columns:52px minmax(0,1fr); align-items:center; gap:12px; min-width:0; }
.offset-poster { display:flex; align-items:center; justify-content:center; width:52px; height:78px; overflow:hidden; border-radius:5px; background:#eeedf2; color:#a19ba9; box-shadow:inset 0 0 0 1px #302b3c08; }
.offset-poster img { display:block; width:100%; height:100%; object-fit:cover; }
.offset-scope-info { display:grid; gap:8px; min-width:0; }
.offset-context { display:flex; align-items:baseline; gap:10px; min-width:0; }
.offset-context strong { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:15px; font-weight:600; }
.offset-context > span { flex-shrink:0; font-size:12px; color:var(--muted); }
.offset-source-detail summary { display:flex; align-items:center; gap:7px; font-size:12px; color:#676473; }
.offset-source-detail summary > span:first-of-type { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.offset-source-detail summary > svg { flex-shrink:0; }
.offset-path-hint { font-size:11px; color:var(--muted); flex-shrink:0; }
.offset-source-detail[open] .offset-path-hint { display:none; }
.offset-source-detail p { font-size:11px; color:var(--muted); overflow-wrap:anywhere; line-height:1.6; padding-top:6px; }
.offset-source-picker { display:grid; grid-template-columns:auto minmax(0,1fr); align-items:center; gap:12px; }
.offset-rule { min-width:0; padding-left:20px; border-left:1px solid var(--line); }
.offset-fields { display:grid; grid-template-columns:minmax(0,1.4fr) minmax(0,1fr); align-items:end; gap:16px; }
.offset-field { display:flex; flex-direction:column; gap:8px; min-width:0; }
.offset-field label { font-size:12px; font-weight:500; line-height:18px; }
.offset-field input { margin:0; height:36px; min-height:36px; padding:7px 10px; font-size:13px; font-variant-numeric:tabular-nums; }
.offset-field :deep(.ui-select-anchor) { display:flex; width:100%; }
.offset-field :deep(.ui-select-trigger) { height:36px; min-height:36px; font-size:13px; padding:7px 10px; }
.offset-input-wrap { position:relative; }
.offset-input-wrap input { padding-right:32px; }
.offset-input-wrap > span { position:absolute; right:11px; top:50%; transform:translateY(-50%); color:var(--muted); font-size:12px; pointer-events:none; }
.offset-result { display:flex; align-items:center; gap:6px; flex-wrap:wrap; }
.offset-delta-badge { display:inline-flex; align-items:center; padding:1px 5px; border-radius:4px; background:#eeebfa; color:var(--purple-dark); font-size:10px; line-height:18px; white-space:nowrap; font-variant-numeric:tabular-nums; }
.offset-delta-badge.neutral { background:#eeedf2; color:#777181; }
.offset-input-error { margin-top:8px; font-size:12px; }
.offset-toolbar { position:sticky; top:-20px; z-index:2; display:flex; justify-content:space-between; align-items:center; gap:10px 16px; flex-wrap:wrap; padding:10px 0; background:#fff; }
.offset-toolbar h3 { font-size:13px; font-weight:600; }
.offset-toolbar > span { font-size:12px; color:var(--muted); }
.offset-check { display:flex; align-items:center; gap:10px; font-size:12px; font-weight:400; }
.offset-checkbox,.offset-check input { width:15px; height:15px; margin:0; flex-shrink:0; accent-color:var(--purple); }
.offset-delta { color:var(--purple-dark); }
.offset-row { display:grid; grid-template-columns:minmax(0,1.8fr) minmax(0,.8fr) repeat(2,minmax(0,1.4fr)); gap:12px; padding:13px 10px; min-height:48px; border-bottom:1px solid var(--line); align-items:center; font-size:12px; }
.offset-row > * { min-width:0; overflow-wrap:anywhere; }
.offset-row.is-selected { background:#faf9fe; }
.offset-head { min-height:34px; padding-block:8px; background:#f7f7fa; color:var(--muted); }
.offset-file { display:flex; align-items:center; gap:10px; }
.offset-file-text { display:grid; gap:4px; min-width:0; flex:1; }
.offset-file-text strong { display:block; font-weight:500; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.offset-file-path { display:flex; width:100%; font-size:11px; line-height:1.5; color:var(--muted); }
.offset-source-detail summary::marker { content:''; }
.offset-source-detail summary::-webkit-details-marker { display:none; }
.offset-code { font-variant-numeric:tabular-nums; }
.offset-selection-note { margin-top:10px; font-size:11px; color:var(--muted); }
.offset-warning { align-items:flex-start; padding:12px 0; }
.offset-footer-note { font-size:11px; color:var(--muted); }
.offset-actions { display:flex; gap:8px; flex-wrap:wrap; }
.offset-actions .button { min-height:36px; font-size:12px; padding:8px 14px; }
@media(max-width:620px) {
  .offset-modal :deep(.modal-head),.offset-modal :deep(.modal-body) { padding:16px; }
  .offset-modal :deep(.modal-foot) { padding:12px 16px; }
  .offset-editor { gap:18px; }
  .offset-toolbar { top:-16px; }
  .offset-setup { grid-template-columns:minmax(0,1fr); gap:14px; padding:12px; }
  .offset-scope { grid-template-columns:44px minmax(0,1fr); }
  .offset-poster { width:44px; height:66px; }
  .offset-rule { padding:14px 0 0; border-left:0; border-top:1px solid var(--line); }
  .offset-fields { grid-template-columns:repeat(2,minmax(0,1fr)); gap:10px; }
  .offset-row { grid-template-columns:minmax(0,1.25fr) minmax(0,.85fr) repeat(2,minmax(0,1.4fr)); gap:8px; padding-inline:6px; }
  .offset-file { gap:7px; }
  .offset-footer-note { flex-basis:100%; }
  .offset-actions { margin-left:auto; }
}
@media(max-width:420px) {
  .offset-row:not(.offset-head) { grid-template-columns:minmax(0,.7fr) repeat(2,minmax(0,1.3fr)); gap:10px; }
  .offset-row:not(.offset-head) .offset-file { grid-column:1 / -1; }
  .offset-row:not(.offset-head) [data-label]::before { content:attr(data-label); display:block; margin-bottom:4px; color:var(--muted); font-size:11px; }
  .offset-head { grid-template-columns:1fr; }
  .offset-head > span:not(:first-child) { display:none; }
}
@media(pointer:coarse) {
  .offset-check { min-height:32px; }
  .offset-field input { font-size:16px; }
}
</style>
