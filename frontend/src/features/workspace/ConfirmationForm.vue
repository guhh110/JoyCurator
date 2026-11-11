<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { Check, Search, SlidersHorizontal, BookOpen, RotateCcw, MoreHorizontal, FolderOpen, CircleAlert } from '@lucide/vue'
import { RouterLink } from 'vue-router'
import EliText from '../../components/ui/EliText.vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Checkbox from '../../components/ui/Checkbox.vue'
import Field from '../../components/ui/Field.vue'
import Select from '../../components/ui/Select.vue'
import SegmentedControl from '../../components/ui/SegmentedControl.vue'
import Pagination from '../../components/ui/Pagination.vue'
import ActionMenu from '../../components/ui/ActionMenu.vue'
import ActionMenuItem from '../../components/ui/ActionMenuItem.vue'
import WorkPicker from './WorkPicker.vue'
import EpisodeMappingPanel from './EpisodeMappingPanel.vue'
import EpisodeCatalog from './EpisodeCatalog.vue'
import type { Workspace } from './useWorkspace'
const props = defineProps<{ workspace: Workspace }>()
const w = props.workspace, s = w.confirmation
const { busy, locked, currentFiles, current, status } = w
const { entry, seasons, season, rows, chosen, loading, fetchingSeasons, stale } = s
const panel = ref<'mapping' | 'catalog' | ''>(''), targetPath = ref('')
const page = ref(1), batchView = ref(false), problemsOnly = ref(false)
const problems = computed(() => chosen.value.filter(row => s.problem(row)))
const displayFiles = computed(() => problemsOnly.value ? problems.value.map(row => row.file) : batchView.value ? chosen.value.map(row => row.file).filter(file => file.path.toLowerCase().includes(w.query.value.toLowerCase())) : currentFiles.value)
const visibleFiles = computed(() => displayFiles.value.slice((page.value - 1) * 40, page.value * 40))
const rowFor = (path: string) => rows.value.find(row => row.file.path === path)
const visibleRows = computed(() => visibleFiles.value.map(file => ({ file, row: rowFor(file.path) })))
const availableFiles = computed(() => displayFiles.value.filter(file => !w.fileLocked(file)))
const selectedCount = computed(() => availableFiles.value.filter(file => rowFor(file.path)?.included).length)
const allSelected = computed(() => selectedCount.value === 0 ? false : selectedCount.value === availableFiles.value.length ? true : 'indeterminate')
const outsideCount = computed(() => chosen.value.filter(row => !current.value?.files.some(file => file.path === row.file.path)).length)
const seasonOptions = computed(() => [{value:'-1',label:'待定'}, ...seasons.value.map(value => ({value:String(value.number),label:`第 ${value.number} 季`}))])
const formLocked = computed(() => locked.value || stale.value)
watch([() => current.value?.id, status], () => { page.value = 1; batchView.value = false; problemsOnly.value = false })
watch([() => w.query.value, batchView, problemsOnly], () => { page.value = 1 })
watch(() => displayFiles.value.length, () => { page.value = Math.max(1, Math.min(page.value, Math.ceil(displayFiles.value.length / 40))) })
watch(() => `${entry.value?.type}:${entry.value?.tmdbId}`, () => { panel.value = '' })
function selectVisible(value: boolean) { for (const file of availableFiles.value) { const row = rowFor(file.path); if (row) row.included = value } }
function openMapping(path: string) { s.samplePath.value = path; s.mode.value = 'digits'; s.mappingKey.value = ''; panel.value = 'mapping' }
function openCatalog(path?: string) {
  targetPath.value = path || chosen.value[0]?.file.path || ''
  panel.value = path ? 'catalog' : panel.value === 'catalog' ? '' : 'catalog'
}
</script>
<template>
  <section class="flow-card confirm-main" aria-label="确认作品与季集">
    <header class="flow-section-head"><div class="flow-current-path"><FolderOpen :size="16" /><EliText :text="current?.id || '选择目录'" ellipsis="start" /></div><ActionMenu align="end" :disabled="locked"><template #trigger><Button variant="ghost" size="sm" icon aria-label="目录更多操作"><MoreHorizontal :size="18" /></Button></template><ActionMenuItem @select="w.reidentification.open(current?.files || [])">重新识别此目录</ActionMenuItem><ActionMenuItem @select="w.reloadDraft">重载本批</ActionMenuItem></ActionMenu></header>
    <div class="confirm-scope-toolbar">
      <SegmentedControl :model-value="status" label="确认状态" :options="[{value:'pending',label:'待确认'},{value:'confirmed',label:'已确认'}]" :disabled="busy" @update:model-value="status = $event === 'confirmed' ? 'confirmed' : 'pending'" />
      <Button v-if="outsideCount" variant="ghost" size="sm" :aria-pressed="batchView" @click="batchView = !batchView">{{ batchView ? '当前目录' : `本批 ${chosen.length} 个文件` }}</Button>
      <Input v-model="w.query.value" type="search" aria-label="搜索文件" placeholder="搜索文件" :disabled="busy"><template #icon><Search :size="14" /></template></Input>
    </div>
    <div v-if="stale && s.open.value" class="confirm-stale"><CircleAlert :size="15" /><span>工作区已更新</span><Button size="sm" :disabled="busy" @click="w.reloadDraft">重新载入</Button></div>
    <div v-if="loading" class="flow-loading">正在读取文件与候选…</div>
    <template v-else>
      <WorkPicker v-if="chosen.length" :state="s" :disabled="formLocked" />
      <template v-if="entry?.type === 'tv' && chosen.length">
        <div class="confirm-batch-toolbar">
          <Field label="批量季号"><Select :model-value="String(season)" label="统一季号" placeholder="选择季度" :disabled="formLocked || fetchingSeasons" :options="seasonOptions" @update:model-value="value => { season = Number(value); s.applySeason() }" /></Field>
          <Button :disabled="formLocked" :aria-expanded="panel === 'mapping'" @click="panel = panel === 'mapping' ? '' : 'mapping'"><SlidersHorizontal :size="14" />调整集数</Button>
          <Button variant="ghost" :disabled="formLocked" :aria-expanded="panel === 'catalog'" @click="openCatalog()"><BookOpen :size="15" />查看本季集目</Button>
          <Button v-if="s.seasonsError.value || Object.keys(s.episodeErrors.value).length" variant="ghost" :disabled="busy" @click="s.retry"><RotateCcw :size="14" />重试季集校验</Button>
        </div>
        <EpisodeMappingPanel v-if="panel === 'mapping'" :state="s" :disabled="formLocked" @close="panel = ''" />
        <EpisodeCatalog v-if="panel === 'catalog'" :key="targetPath" :state="s" :disabled="formLocked" :path="targetPath" @close="panel = ''" />
      </template>
      <div class="confirm-selection-bar"><span>已选 <strong>{{ chosen.length }}</strong> 个文件<span v-if="outsideCount" class="muted"> · 其他目录 {{ outsideCount }} 个</span></span><Button v-if="problems.length" variant="ghost" size="sm" :aria-pressed="problemsOnly" @click="problemsOnly = !problemsOnly"><CircleAlert :size="13" />{{ problemsOnly ? '显示全部' : `${problems.length} 个需修正` }}</Button></div>
      <div class="confirm-file-table" role="table" aria-label="文件季集对应关系">
        <div class="confirm-file-head" :class="{'has-seasons':entry?.type === 'tv'}" role="row"><Checkbox label="选择当前目录显示的文件" :model-value="allSelected" :disabled="formLocked || !availableFiles.length" @update:model-value="selectVisible($event === true)" /><span role="columnheader">文件</span><template v-if="entry?.type === 'tv'"><span role="columnheader">季号</span><span role="columnheader">集数</span></template><span /></div>
        <article v-for="{file, row} in visibleRows" :key="file.path" class="confirm-file-row" :class="{'is-selected':row?.included,'has-seasons':entry?.type === 'tv','has-problem':row && s.problem(row)}" role="row">
          <Checkbox :label="`确认文件 ${file.path}`" :model-value="row?.included || false" :disabled="formLocked || !row || w.fileLocked(file)" @update:model-value="row && (row.included = $event === true)" />
          <div class="confirm-file-name" role="cell"><EliText :text="file.path.slice(file.path.lastIndexOf('/')+1)" ellipsis="start" /><EliText class="confirm-file-path" :text="file.path" ellipsis="start" /><small v-if="w.fileLocked(file)">{{ file.execution?.completed ? '已完成' : '已提交' }}</small><small v-else-if="w.isConfirmed(file)" class="flow-confirmed"><Check :size="11" />{{ file.entry?.title }}{{ file.entry?.type === 'tv' ? ` · 第 ${file.season} 季 · ${file.episodes.join(',')} 集` : '' }}</small></div>
          <template v-if="entry?.type === 'tv'">
            <Field class="confirm-row-season" label="季号"><Select v-if="row" size="sm" :model-value="String(row.season)" :label="`文件季度 ${file.path}`" :disabled="formLocked || !row.included" :options="seasonOptions" @update:model-value="row.season = Number($event)" /><span v-else>—</span></Field>
            <Field class="confirm-row-episodes" label="集数"><Input v-if="row" v-model="row.text" size="sm" :aria-label="`文件集数 ${file.path}`" placeholder="如 1 或 1,2" :disabled="formLocked || !row.included" :invalid="!!s.problem(row)" @update:model-value="s.manual(row)" /><span v-else>—</span></Field>
          </template>
          <ActionMenu align="end"><template #trigger><Button variant="ghost" size="sm" icon :aria-label="`文件更多操作 ${file.path}`"><MoreHorizontal :size="17" /></Button></template><ActionMenuItem v-if="entry?.type === 'tv' && row?.included" :disabled="formLocked" @select="openMapping(file.path)">从此文件取数</ActionMenuItem><ActionMenuItem v-if="entry?.type === 'tv' && row?.included" :disabled="formLocked" @select="openCatalog(file.path)">对照集目</ActionMenuItem><ActionMenuItem v-if="file.traceId" as-child><RouterLink :to="{name:'extraction-history-detail',params:{traceId:file.traceId},query:{file:file.path}}">识别依据</RouterLink></ActionMenuItem><ActionMenuItem :disabled="locked || w.fileLocked(file)" @select="w.reidentification.open([file])">重新识别</ActionMenuItem></ActionMenu>
          <p v-if="row && s.problem(row)" class="confirm-file-problem"><CircleAlert :size="12" />{{ s.problem(row) }}</p><p v-else-if="file.state === 'failed'" class="confirm-file-problem">{{ file.reason }}</p>
        </article>
      </div>
      <p v-if="!displayFiles.length" class="flow-empty">{{ status === 'pending' ? '此目录没有待确认文件' : '此目录没有已确认文件' }}</p>
      <Pagination v-if="displayFiles.length > 40" :page="page" :total="displayFiles.length" :page-size="40" @change="page = $event" />
    </template>
    <footer class="flow-footer confirm-footer"><Button :disabled="locked || !w.workGroups.value.length" @click="w.goStep(3)">前往预览（{{ w.ready.value.length }}）</Button><Button v-if="chosen.length" variant="primary" :disabled="!s.canConfirm.value" :loading="s.saving.value" @click="w.confirmBatch"><Check :size="14" />确认 {{ chosen.length }} 个文件</Button></footer>
  </section>
</template>

<style src="./confirmation.css"></style>
