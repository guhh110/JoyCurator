<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { Check, SlidersHorizontal, BookOpen, RotateCcw, MoreHorizontal, CircleAlert } from '@lucide/vue'
import { RouterLink } from 'vue-router'
import EliText from '../../components/ui/EliText.vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Checkbox from '../../components/ui/Checkbox.vue'
import Field from '../../components/ui/Field.vue'
import Select from '../../components/ui/Select.vue'
import Pagination from '../../components/ui/Pagination.vue'
import ActionMenu from '../../components/ui/ActionMenu.vue'
import ActionMenuItem from '../../components/ui/ActionMenuItem.vue'
import WorkPicker from './WorkPicker.vue'
import EpisodeMappingPanel from './EpisodeMappingPanel.vue'
import EpisodeCatalog from './EpisodeCatalog.vue'
import type { Workspace } from './useWorkspace'
const props = defineProps<{ workspace: Workspace; groupKey: string }>()
const w = props.workspace, s = w.confirmation.group(props.groupKey)
const { busy, locked } = w
const { entry, files, rows, seasons, season, chosen, fetchingSeasons } = s
const loading = w.confirmation.loading
const panel = ref<'mapping' | 'catalog' | ''>(''), targetPath = ref(''), page = ref(1)
const rowByPath = computed(() => new Map(rows.value.map(row => [row.file.path, row])))
const visibleRows = computed(() => files.value.slice((page.value - 1) * 40, page.value * 40).map(file => ({ file, row: rowByPath.value.get(file.path) })))
const availableFiles = computed(() => files.value.filter(file => !w.fileLocked(file) && rowByPath.value.has(file.path)))
const selectedCount = computed(() => availableFiles.value.filter(file => rowByPath.value.get(file.path)?.included).length)
const allSelected = computed(() => selectedCount.value === 0 ? false : selectedCount.value === availableFiles.value.length ? true : 'indeterminate')
const seasonOptions = computed(() => [{value:'-1',label:'待定'}, ...seasons.value.map(value => ({value:String(value.number),label:`第 ${value.number} 季`}))])
const formLocked = computed(() => locked.value || w.confirmation.stale.value)
watch([w.directory, w.status, w.query], () => { page.value = 1; panel.value = '' })
watch(() => files.value.length, () => { page.value = Math.max(1, Math.min(page.value, Math.ceil(files.value.length / 40))) })
function selectVisible(value: boolean) { for (const file of availableFiles.value) { const row = rowByPath.value.get(file.path); if (row) row.included = value } }
function openMapping(path: string) { s.samplePath.value = path; s.mode.value = 'digits'; s.mappingKey.value = ''; panel.value = 'mapping' }
function openCatalog(path?: string) {
  targetPath.value = path || chosen.value[0]?.file.path || ''
  panel.value = path ? 'catalog' : panel.value === 'catalog' ? '' : 'catalog'
}
</script>
<template>
  <section class="confirm-work-group" :aria-label="entry?.title || '待指定作品'" :data-work-key="groupKey">
    <WorkPicker :state="s" :file-count="files.length" :disabled="formLocked || loading || !chosen.length">
      <template v-if="entry?.type === 'tv'" #actions>
        <div class="confirm-batch-toolbar" role="group" :aria-label="`${entry.title}批量季集调整`">
          <Select :model-value="String(season)" label="统一季号" placeholder="选择季度" :disabled="formLocked || loading || !chosen.length || fetchingSeasons" :options="seasonOptions" @update:model-value="value => { season = Number(value); s.applySeason() }" />
          <Button :disabled="formLocked || loading || !chosen.length" :aria-expanded="panel === 'mapping'" @click="panel = panel === 'mapping' ? '' : 'mapping'"><SlidersHorizontal :size="14" />调整集数</Button>
          <Button variant="ghost" :disabled="formLocked || loading || !chosen.length" :aria-expanded="panel === 'catalog'" @click="openCatalog()"><BookOpen :size="15" />本季集目</Button>
          <Button v-if="s.seasonsError.value || Object.keys(s.episodeErrors.value).length" variant="ghost" :disabled="busy" @click="s.retry"><RotateCcw :size="14" />重试季集校验</Button>
        </div>
      </template>
    </WorkPicker>
    <template v-if="entry?.type === 'tv' && chosen.length">
      <EpisodeMappingPanel v-if="panel === 'mapping'" :state="s" :disabled="formLocked" @close="panel = ''" />
      <EpisodeCatalog v-if="panel === 'catalog'" :key="targetPath" :state="s" :disabled="formLocked" :path="targetPath" @close="panel = ''" />
    </template>
    <div v-if="loading" class="flow-loading">正在读取文件与候选…</div>
    <template v-else>
      <div class="confirm-file-table" role="table" :aria-label="`${entry?.title || '待指定作品'}文件季集对应关系`">
        <div class="confirm-file-head" :class="{'has-seasons':entry?.type === 'tv'}" role="row"><Checkbox :label="`选择${entry?.title || '待指定作品'}组内筛选结果中的文件`" :model-value="allSelected" :disabled="formLocked || !availableFiles.length" @update:model-value="selectVisible($event === true)" /><span role="columnheader">文件</span><template v-if="entry?.type === 'tv'"><span role="columnheader">季号</span><span role="columnheader">集数</span></template><span /></div>
        <article v-for="{file, row} in visibleRows" :key="file.path" class="confirm-file-row" :class="{'is-selected':row?.included,'has-seasons':entry?.type === 'tv','has-problem':row && s.problem(row)}" role="row">
          <Checkbox :label="`确认文件 ${file.path}`" :model-value="row?.included || false" :disabled="formLocked || !row || w.fileLocked(file)" @update:model-value="row && (row.included = $event === true)" />
          <div class="confirm-file-name" role="cell"><EliText :text="file.path.slice(file.path.lastIndexOf('/')+1)" ellipsis="start" /><EliText class="confirm-file-path" :text="file.path" ellipsis="start" /><small v-if="w.fileLocked(file)">{{ file.execution?.completed ? '已完成' : '已提交' }}</small><small v-else-if="w.isConfirmed(file)" class="flow-confirmed"><Check :size="11" />{{ file.entry?.title }}{{ file.entry?.type === 'tv' ? ` · 第 ${file.season} 季 · ${file.episodes.join(',')} 集` : '' }}</small></div>
          <template v-if="entry?.type === 'tv'">
            <Field class="confirm-row-season" label="季号"><Select v-if="row" size="sm" :model-value="String(row.season)" :label="`文件季度 ${file.path}`" :disabled="formLocked || !row.included || w.fileLocked(file)" :options="seasonOptions" @update:model-value="row.season = Number($event)" /><span v-else>—</span></Field>
            <Field class="confirm-row-episodes" label="集数"><Input v-if="row" v-model="row.text" size="sm" :aria-label="`文件集数 ${file.path}`" placeholder="如 1 或 1,2" :disabled="formLocked || !row.included || w.fileLocked(file)" :invalid="!!s.problem(row)" @update:model-value="s.manual(row)" /><span v-else>—</span></Field>
          </template>
          <ActionMenu align="end"><template #trigger><Button variant="ghost" size="sm" icon :aria-label="`文件更多操作 ${file.path}`"><MoreHorizontal :size="17" /></Button></template><ActionMenuItem v-if="entry?.type === 'tv' && row?.included" :disabled="formLocked || w.fileLocked(file)" @select="openMapping(file.path)">从此文件取数</ActionMenuItem><ActionMenuItem v-if="entry?.type === 'tv' && row?.included" :disabled="formLocked || w.fileLocked(file)" @select="openCatalog(file.path)">对照集目</ActionMenuItem><ActionMenuItem v-if="file.traceId" as-child><RouterLink :to="{name:'extraction-history-detail',params:{traceId:file.traceId},query:{file:file.path}}">识别依据</RouterLink></ActionMenuItem><ActionMenuItem :disabled="locked || w.fileLocked(file)" @select="w.reidentification.open([file])">重新识别</ActionMenuItem></ActionMenu>
          <p v-if="row && s.problem(row)" class="confirm-file-problem"><CircleAlert :size="12" />{{ s.problem(row) }}</p><p v-else-if="file.state === 'failed'" class="confirm-file-problem">{{ file.reason }}</p>
        </article>
      </div>
      <Pagination v-if="files.length > 40" :page="page" :total="files.length" :page-size="40" @change="page = $event" />
      <footer v-if="entry && rows.length" class="confirm-group-footer">
        <span v-if="chosen.length > 2000" class="flow-row-error">一次最多确认 2000 个文件，请减少勾选</span>
        <Button variant="primary" :disabled="!s.canConfirm.value" :loading="s.saving.value || s.validating.value" @click="w.confirmGroup(groupKey)"><Check v-if="!s.saving.value && !s.validating.value" :size="14" />确认 {{ chosen.length }} 个文件</Button>
      </footer>
    </template>
  </section>
</template>
