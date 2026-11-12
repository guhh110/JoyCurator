<script setup lang="ts">
import Button from '../../components/ui/Button.vue'
import { computed, ref, watch } from 'vue'
import EliText from '../../components/ui/EliText.vue'
import Select from '../../components/ui/Select.vue'
import Checkbox from '../../components/ui/Checkbox.vue'
import Disclosure from '../../components/ui/Disclosure.vue'
import LayoutEditor from '../scrape/LayoutEditor.vue'
import { bytes } from '../../api'
import type { Workspace } from './useWorkspace'
const props = defineProps<{ workspace: Workspace }>()
const w = props.workspace
const scraping = w.purpose === 'scrape'
const { busy, locked, workGroups, pendingCount, selected } = w
const { mode, duplicatePolicy, preview, previewRows, previewPage, previewReady } = w.previewState
const { content, onlyMissing, preview: scrapePreview, ready: scrapeReady } = w.scrapePreviewState
const outputPage = ref(1), editingLayout = ref('')
const hasPreview = computed(() => scraping ? !!scrapePreview.value : !!preview.value)
const outputs = computed(() => [...(scrapePreview.value?.artifacts || []).map(path => ({path,action:onlyMissing.value ? '新增' : '生成／覆盖'})), ...(scrapePreview.value?.skipped || []).map(path => ({path,action:'保留'}))])
const visibleOutputs = computed(() => outputs.value.slice((outputPage.value - 1) * 50, outputPage.value * 50))
const layoutProblems = computed(() => scraping ? selected.value.filter(file => !file.layout?.workDir || file.state !== 'ready') : [])
const canPrepare = computed(() => !locked.value && !editingLayout.value && selected.value.length > 0 && selected.value.length <= 1000 && !layoutProblems.value.length)
watch(scrapePreview, () => { outputPage.value = 1 })
async function prepare() { if (scraping) await w.scrapePreviewState.prepare(); else await w.previewState.prepare() }
const unique = (values: string[]) => [...new Set(values)]
</script>
<template>
  <section class="flow-preview" aria-label="执行预览">
    <section class="flow-card">
      <header class="flow-section-head"><h2>已确认作品</h2><div class="flow-inline-actions"><Button variant="ghost" size="sm" :disabled="locked" @click="w.selectReady(true)">全选可执行</Button><Button variant="ghost" size="sm" :disabled="locked || !selected.length" @click="w.selectReady(false)">清空</Button></div></header>
      <article v-for="group in workGroups" :key="group.id" class="flow-preview-work">
        <Disclosure label="选择文件">
          <template #heading><h3>{{ group.entry?.title }} <span class="muted">{{ group.entry?.year || '年份未知' }}</span></h3><span class="flow-work-count muted">{{ group.files.length }} 个文件</span></template>
          <label v-for="file in group.files" :key="file.path" class="flow-preview-file"><Checkbox :label="`执行文件 ${file.path}`" :model-value="file.selected && file.state === 'ready'" :disabled="locked || w.fileLocked(file) || file.state !== 'ready'" @update:model-value="w.selectFile(file)" /><EliText :text="file.path" ellipsis="start" /><small v-if="file.entry?.type === 'tv'">S{{ String(file.season).padStart(2,'0') }} · {{ file.episodes.join(',') }}</small><small v-if="file.state !== 'ready'" class="flow-row-error">{{ file.reason }}</small></label>
        </Disclosure>
        <template v-if="scraping"><div class="flow-layout-summary"><span>作品目录</span><div><EliText v-for="path in unique(group.files.map(file => file.layout?.workDir || '待确认'))" :key="path" :text="path" ellipsis="start" /></div><Button variant="ghost" size="sm" :disabled="locked" :aria-expanded="editingLayout === group.id" @click="editingLayout = editingLayout === group.id ? '' : group.id; w.invalidatePreviews()">{{ group.files.some(file => !file.layout?.workDir || file.state !== 'ready') ? '确认输出目录' : '修改' }}</Button></div><div v-if="group.entry?.type === 'tv'" class="flow-layout-summary"><span>季目录</span><div><EliText v-for="path in unique(group.files.map(file => file.layout?.seasonDir || '平铺'))" :key="path" :text="path" ellipsis="start" /></div></div><p v-for="reason in unique(group.files.filter(file => file.state !== 'ready').map(file => file.reason))" :key="reason" class="flow-row-error">{{ reason }}</p><LayoutEditor v-if="editingLayout === group.id" :key="`${group.id}:${w.task.value?.revision}`" :files="group.files" :busy="busy" @save="async changes => { await w.saveLayouts(changes); if (!w.error.value) editingLayout = '' }" /></template>
      </article>
      <p v-if="!workGroups.length" class="flow-empty">没有可预览的已确认文件</p>
    </section>
    <section v-if="hasPreview" class="flow-card flow-output" aria-label="实际输出路径">
      <header class="flow-section-head"><h2>输出预览</h2><span class="muted">需要 {{ bytes(scraping ? scrapePreview!.required : preview!.required) }}</span></header>
      <template v-if="scraping && scrapePreview"><div v-if="scrapePreview.issues.length" class="flow-output-issues"><p v-for="issue in unique(scrapePreview.issues)" :key="issue">{{ issue }}</p></div><p class="flow-output-summary">{{ scrapePreview.artifacts.length }} 项产物 · 保留 {{ scrapePreview.skipped.length }} 项</p><p v-if="!scrapePreview.artifacts.length && !scrapePreview.issues.length" class="flow-empty">没有需要生成的文件</p><div v-for="item in visibleOutputs" :key="item.path" class="flow-output-row"><EliText :text="item.path" ellipsis="start" /><span>{{ item.action }}</span></div><div v-if="outputs.length > 50" class="flow-pagination"><Button variant="ghost" size="sm" :disabled="outputPage === 1" @click="outputPage--">上一页</Button><span>{{ outputPage }}/{{ Math.ceil(outputs.length / 50) }}</span><Button variant="ghost" size="sm" :disabled="outputPage * 50 >= outputs.length" @click="outputPage++">下一页</Button></div></template>
      <template v-else-if="preview"><div v-if="preview.issues.length" class="flow-output-issues"><p v-for="issue in unique(preview.issues)" :key="issue">{{ issue }}</p></div><p class="flow-output-summary">{{ preview.rows.length }} 个视频 · 跳过 {{ preview.rows.filter(row => row.action === 'skip').length }} 个</p><article v-for="row in previewRows" :key="row.source" class="flow-organize-output"><div><EliText :text="row.source" ellipsis="start" /><span>{{ row.action === 'skip' ? '跳过重复' : row.action === 'overwrite' ? '覆盖' : '新增' }}</span></div><div class="flow-output-target"><span>→</span><EliText :text="row.target" ellipsis="start" /></div><p v-if="row.artifacts.length" class="muted">{{ row.artifacts.join(' · ') }}</p><p v-for="issue in unique(row.issues)" :key="issue" class="flow-row-error">{{ issue }}</p></article><div v-if="preview.rows.length > 30" class="flow-pagination"><Button variant="ghost" size="sm" :disabled="previewPage === 1" @click="previewPage--">上一页</Button><span>{{ previewPage }}/{{ Math.ceil(preview.rows.length / 30) }}</span><Button variant="ghost" size="sm" :disabled="previewPage * 30 >= preview.rows.length" @click="previewPage++">下一页</Button></div></template>
    </section>
    <footer class="flow-footer flow-card flow-preview-footer">
      <div class="flow-preview-footer-main">
        <div class="flow-preview-options" role="group" :aria-label="scraping ? '刮削选项' : '整理选项'">
          <template v-if="!scraping">
            <label>整理方式<Select v-model="mode" label="整理方式" :disabled="locked" :options="Object.entries(w.modes).map(([value,label]) => ({value,label}))" /></label>
            <label>重复文件<Select v-model="duplicatePolicy" label="重复文件处理" :disabled="locked" :options="[{value:'skip',label:'跳过重复'},{value:'overwrite',label:'覆盖重复'}]" /></label>
          </template>
          <template v-else>
            <label>刮削内容<Select v-model="content" label="刮削内容" :disabled="locked" :options="[{value:'all',label:'NFO＋图片'},{value:'nfo',label:'仅 NFO'},{value:'images',label:'仅图片'}]" /></label>
            <label class="flow-checkbox"><Checkbox :model-value="onlyMissing" label="只补缺失" :disabled="locked" @update:model-value="onlyMissing = $event === true" />只补缺失</label>
          </template>
        </div>
        <span class="flow-preview-selection">已选 {{ selected.length }} 个文件<span v-if="pendingCount" class="muted"> · {{ pendingCount }} 个待确认</span></span>
        <Button v-if="!hasPreview" variant="primary" :disabled="!canPrepare" @click="prepare">{{ busy ? '正在生成…' : '生成预览' }}</Button>
        <Button v-else-if="scraping ? scrapeReady : previewReady" variant="primary" @click="w.execute">{{ scraping ? '加入刮削队列' : '加入整理队列' }}</Button>
        <Button v-else :disabled="!canPrepare" @click="prepare">重新生成预览</Button>
      </div>
      <p v-if="selected.length > 1000" class="flow-row-error">单次最多预览 1000 个视频</p>
      <p v-if="!scraping && duplicatePolicy === 'overwrite' || scraping && !onlyMissing" class="flow-overwrite">覆盖后，撤销无法恢复原内容</p>
    </footer>
  </section>
</template>
