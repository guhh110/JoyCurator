<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { episodeCode, name } from '../../api'
import Modal from '../../components/Modal.vue'
import Select from '../../components/ui/Select.vue'
import EliText from '../../components/ui/EliText.vue'
import { ArrowRight, FolderOpen, Info } from '@lucide/vue'
import { originalEpisodesValid } from './episodeOffset'
import type { useEpisodeOffset } from './useEpisodeOffset'

const props = defineProps<{ editor: ReturnType<typeof useEpisodeOffset>; locked: boolean }>()
const { context, source, sources, season, original, target, offset, rows, chosen, loading, saving,
  error, stale, allowOverlap, overlaps, canApply } = props.editor
const page = ref(1)
const visible = computed(() => rows.value.slice((page.value - 1) * 40, page.value * 40))
const eligible = computed(() => rows.value.filter(row => originalEpisodesValid(row.file)))
const allSelected = computed(() => eligible.value.length > 0 && eligible.value.every(row => row.selected))
const disabled = computed(() => props.locked || saving.value || stale.value)
watch(source, () => { page.value = 1 })
const originalCode = (numbers: number[]) => numbers.length ? numbers.map(number => `E${String(number).padStart(2, '0')}`).join(', ') : '未识别'
</script>

<template>
  <Modal v-if="context" class="offset-modal" title="批量修正季集" wide :locked="saving" @close="editor.close">
    <div class="offset-editor">
      <section class="offset-scope" aria-label="修正范围">
        <div class="offset-context"><strong>{{ context.title }}</strong><span>{{ rows.length }} 个文件</span></div>
        <div v-if="sources.length > 1" class="offset-source-picker">
          <label for="offset-source">来源</label>
          <Select id="offset-source" v-model="source" label="修改来源" :disabled="disabled"
            :options="sources.map(path => ({ value: path, label: path || '未归属来源' }))" />
        </div>
        <details v-else class="offset-source-detail">
          <summary><FolderOpen :size="14" aria-hidden="true" /><span>来源：{{ name(source) || '未归属来源' }}</span><span class="offset-path-hint">查看路径</span></summary>
          <p>{{ source || '未归属来源' }}</p>
        </details>
      </section>
      <section class="offset-rule" aria-label="季集修正规则">
        <div class="offset-fields">
          <div class="offset-field offset-season">
            <label for="offset-season">目标季度</label>
            <Select id="offset-season" :model-value="String(season)" label="目标季度" :disabled="disabled"
              :options="context.seasons.map(item => ({ value: String(item.number), label: `${item.name} · ${item.count} 集` }))"
              @update:model-value="value => season = Number(value)" />
          </div>
          <div class="offset-field"><label for="offset-original">原始集号</label><input id="offset-original" v-model.number="original" type="number" min="1" step="1" aria-label="原始识别参照集号" :disabled="disabled" /></div>
          <ArrowRight class="offset-arrow" :size="17" aria-hidden="true" />
          <div class="offset-field"><label for="offset-target">实际集号</label><input id="offset-target" v-model.number="target" type="number" min="1" step="1" aria-label="实际对应集号" :disabled="disabled" /></div>
          <span v-if="Number.isSafeInteger(offset)" class="offset-delta-badge" :class="{ neutral: offset === 0 }">{{ offset === 0 ? '不偏移' : `偏移 ${offset > 0 ? '+' : ''}${offset} 集` }}</span>
          <span v-else class="offset-input-error danger">请输入正整数</span>
        </div>
        <p class="offset-rule-note"><Info :size="13" aria-hidden="true" />以原始识别为准，不累加；合并集整体平移，不跨季。</p>
      </section>
      <section class="offset-preview" aria-label="文件修正预览">
      <div class="offset-toolbar"><h3>文件预览</h3><span role="status">已勾选 {{ chosen.length }} / {{ rows.length }}{{ loading ? ' · 校验中…' : '' }}</span></div>
      <div class="offset-table" role="table" aria-label="季集修正预览">
        <div class="offset-row offset-head" role="row"><span role="columnheader"><label class="offset-check"><input type="checkbox" :checked="allSelected" :indeterminate="chosen.length > 0 && !allSelected" :disabled="disabled || !eligible.length" aria-label="选择本来源全部可修正文件" @change="editor.selectAll(($event.target as HTMLInputElement).checked)" /><span>文件</span></label></span><span role="columnheader">原始识别</span><span role="columnheader">当前映射</span><span role="columnheader">修正后</span></div>
        <div v-for="row in visible" :key="row.file.path" class="offset-row" :class="{ 'is-selected': row.selected }" role="row">
          <div role="cell" class="offset-file">
            <input type="checkbox" class="offset-checkbox" :checked="row.selected" :aria-label="`修正 ${row.file.path}`"
              :disabled="disabled || !originalEpisodesValid(row.file)" @change="editor.select(row.file.path, ($event.target as HTMLInputElement).checked)" />
            <div class="offset-file-text">
              <strong>{{ name(row.file.path) }}</strong>
              <EliText :text="row.file.path" class="offset-file-path" />
            </div>
          </div>
          <span role="cell" class="offset-code" data-label="原始识别">{{ originalCode(row.file.metadata.episodes) }}</span>
          <span role="cell" class="offset-code" data-label="当前映射">{{ row.file.season < 0 ? '季号未知' : episodeCode(row.file.season, row.file.episodes) }}</span>
          <div role="cell" data-label="修正后" :class="{ danger: row.selected && row.error }">
            <span v-if="!originalEpisodesValid(row.file)" class="muted tiny">缺少原始集号，请单独修正</span>
            <span v-else-if="!row.selected" class="muted">不修改</span>
            <span v-else-if="row.error">{{ row.error }}</span>
            <template v-else><span class="offset-code offset-delta">{{ episodeCode(season, row.episodes) }}</span><span v-if="loading" class="tiny muted"> · 校验中</span><p v-if="row.overlaps.length" class="tiny">同集映射：{{ row.overlaps.join('、') }}</p></template>
          </div>
        </div>
      </div>
      <div v-if="rows.length > 40" class="offset-pagination"><button class="text-button" :disabled="page === 1" @click="page--">上一页</button><span>{{ page }} / {{ Math.ceil(rows.length / 40) }}</span><button class="text-button" :disabled="page * 40 >= rows.length" @click="page++">下一页</button></div>
      <p class="offset-selection-note">仅修正勾选文件，不改变“参与整理”状态。</p>
      </section>
      <p v-if="stale" class="notice error" role="alert">工作区已更新，请关闭弹窗后重新打开，避免覆盖新的修正。</p>
      <p v-if="error" class="notice error" role="alert">{{ error }} <button v-if="!stale && !saving" class="text-button" @click="editor.retry">重新校验</button></p>
      <label v-if="overlaps" class="offset-check offset-warning"><input v-model="allowOverlap" type="checkbox" :disabled="disabled" />已确认同集映射，允许保留多版本文件；目标路径冲突仍需在整理预览中处理。</label>
    </div>
    <template #footer><span class="offset-footer-note">仅保存映射，不执行文件整理</span><div class="offset-actions"><button class="button" :disabled="saving" @click="editor.close">取消</button><button class="button primary" :disabled="!canApply" @click="editor.apply">{{ saving ? '保存并校验…' : chosen.length ? `应用到 ${chosen.length} 个文件` : '应用修正' }}</button></div></template>
  </Modal>
</template>

<style scoped>
.offset-modal.modal.wide { width:min(820px, calc(100% - 32px)); }
.offset-modal :deep(.modal-head) { padding:18px 22px 16px; }
.offset-modal :deep(.modal-head h2) { font-size:18px; margin-top:4px; }
.offset-modal :deep(.modal-body) { padding:20px 22px; }
.offset-modal :deep(.modal-foot) { padding:14px 22px; gap:12px; flex-wrap:wrap; }
.offset-editor { display:grid; gap:22px; }
.offset-scope { display:grid; gap:8px; }
.offset-context { display:flex; align-items:baseline; gap:10px; }
.offset-context strong { font-size:15px; font-weight:600; }
.offset-context > span { font-size:12px; color:var(--muted); }
.offset-source-detail summary { display:flex; align-items:center; gap:7px; font-size:12px; color:#676473; }
.offset-source-detail summary > span:first-of-type { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.offset-path-hint { font-size:11px; color:var(--muted); flex-shrink:0; }
.offset-source-detail[open] .offset-path-hint { display:none; }
.offset-source-detail p { font-size:11px; color:var(--muted); overflow-wrap:anywhere; line-height:1.6; padding-top:6px; }
.offset-source-picker { display:grid; grid-template-columns:auto minmax(0,1fr); align-items:center; gap:12px; }
.offset-rule { padding:16px; background:#f8f7fb; border:1px solid var(--line); border-radius:10px; }
.offset-fields { display:grid; grid-template-columns:minmax(160px,1.65fr) minmax(80px,1fr) 18px minmax(80px,1fr) auto; align-items:end; gap:12px; }
.offset-field { display:flex; flex-direction:column; gap:8px; min-width:0; }
.offset-field label { font-size:12px; font-weight:500; line-height:18px; }
.offset-field input { margin:0; height:36px; min-height:36px; padding:7px 10px; font-size:13px; font-variant-numeric:tabular-nums; }
.offset-field :deep(.ui-select-anchor) { display:flex; width:100%; }
.offset-field :deep(.ui-select-trigger) { height:36px; min-height:36px; font-size:13px; padding:7px 10px; }
.offset-arrow { align-self:end; margin-bottom:9px; color:var(--muted); }
.offset-delta-badge { display:flex; align-items:center; align-self:end; height:36px; padding:0 10px; border-radius:6px; background:#eeebfa; color:var(--purple-dark); font-size:12px; white-space:nowrap; }
.offset-delta-badge.neutral { background:#eeedf2; color:#777181; }
.offset-input-error { font-size:12px; align-self:center; }
.offset-rule-note { display:flex; align-items:center; gap:6px; margin-top:12px; color:var(--muted); font-size:11px; line-height:1.6; }
.offset-rule-note svg { flex-shrink:0; }
.offset-toolbar,.offset-pagination { display:flex; justify-content:space-between; align-items:center; gap:12px; flex-wrap:wrap; }
.offset-toolbar { margin-bottom:12px; }
.offset-toolbar h3 { font-size:13px; font-weight:600; }
.offset-toolbar > span { font-size:12px; color:var(--muted); }
.offset-check { display:flex; align-items:center; gap:10px; font-size:12px; font-weight:400; }
.offset-checkbox,.offset-check input { width:15px; height:15px; margin:0; flex-shrink:0; accent-color:var(--purple); }
.offset-delta { color:var(--purple-dark); }
.offset-row { display:grid; grid-template-columns:minmax(0,2fr) minmax(0,1fr) minmax(0,1.15fr) minmax(0,1.25fr); gap:12px; padding:13px 10px; min-height:48px; border-bottom:1px solid var(--line); align-items:center; font-size:12px; }
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
.offset-pagination { padding-top:12px; font-size:12px; }
.offset-warning { align-items:flex-start; padding:12px 0; }
.offset-footer-note { font-size:11px; color:var(--muted); }
.offset-actions { display:flex; gap:8px; flex-wrap:wrap; }
.offset-actions .button { min-height:36px; font-size:12px; padding:8px 14px; }
@media(max-width:620px) {
  .offset-modal :deep(.modal-head),.offset-modal :deep(.modal-body) { padding:16px; }
  .offset-modal :deep(.modal-foot) { padding:12px 16px; }
  .offset-editor { gap:18px; }
  .offset-rule { padding:12px; }
  .offset-fields { grid-template-columns:minmax(0,1fr) 18px minmax(0,1fr); gap:10px; }
  .offset-season { grid-column:1 / -1; }
  .offset-delta-badge,.offset-input-error { grid-column:1 / -1; justify-self:start; height:26px; }
  .offset-row { grid-template-columns:minmax(0,1.25fr) repeat(3,minmax(0,1fr)); gap:8px; padding-inline:6px; }
  .offset-file { gap:7px; }
  .offset-footer-note { flex-basis:100%; }
  .offset-actions { margin-left:auto; }
}
@media(max-width:420px) {
  .offset-row:not(.offset-head) { grid-template-columns:repeat(3,minmax(0,1fr)); gap:10px; }
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
