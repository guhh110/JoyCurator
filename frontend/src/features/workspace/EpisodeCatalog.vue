<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Search, Check } from '@lucide/vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Field from '../../components/ui/Field.vue'
import Select from '../../components/ui/Select.vue'
import Pagination from '../../components/ui/Pagination.vue'
import SidePanel from '../../components/ui/SidePanel.vue'
import { notify } from '../../app/toast'
import { parseEpisodes } from '../../api'
import type { ConfirmationGroup } from './useConfirmationGroup'
const props = defineProps<{ state: ConfirmationGroup; disabled: boolean; path?: string }>()
const emit = defineEmits<{ close: [] }>()
const s = props.state
const target = ref(props.path || s.chosen.value[0]?.file.path || '')
const targetRow = computed(() => s.chosen.value.find(row => row.file.path === target.value))
const season = ref(targetRow.value?.season ?? s.season.value)
const query = ref(''), page = ref(1)
const episodes = computed(() => (s.episodeLists.value[season.value] || []).filter(item => `${item.number} ${item.name}`.includes(query.value)))
const visible = computed(() => episodes.value.slice((page.value - 1) * 24, page.value * 24))
const selectedNumbers = computed(() => { try { return targetRow.value?.season === season.value ? parseEpisodes(targetRow.value.text) : [] } catch { return [] } })
watch(season, value => { s.viewSeason(value); page.value = 1 }, { immediate: true })
watch(query, () => { page.value = 1 })
watch(target, () => { if (targetRow.value) season.value = targetRow.value.season })
function choose(number: number) {
  if (!targetRow.value) return
  targetRow.value.season = season.value; targetRow.value.text = String(number); s.manual(targetRow.value)
  notify(`已填入第 ${number} 集`, 'success')
}
</script>
<template>
  <SidePanel :open="true" title="本季集目" @update:open="value => { if (!value) emit('close') }">
    <section class="confirm-tool-panel" aria-label="TMDB 集目">
    <div class="confirm-catalog-fields"><Field label="填入文件"><Select v-model="target" label="填入文件" ellipsis="start" :disabled="disabled" :options="s.chosen.value.map(row => ({value:row.file.path,label:row.file.path}))" /></Field><Field label="季度"><Select :model-value="String(season)" @update:model-value="season = Number($event)" label="集目季度" :disabled="disabled" :options="s.seasons.value.map(value => ({value:String(value.number),label:value.name}))" /></Field></div>
    <Input v-model="query" type="search" aria-label="搜索集目" placeholder="搜索集号或名称"><template #icon><Search :size="14" /></template></Input>
    <p v-if="!episodes.length" class="muted">{{ s.episodeErrors.value[season] || (s.episodeLists.value[season] ? '没有匹配的集目' : '正在读取集目…') }}</p>
    <div class="confirm-catalog-grid"><Button v-for="episode in visible" :key="episode.number" :class="{'is-mapped':selectedNumbers.includes(episode.number)}" :disabled="disabled || !targetRow" @click="choose(episode.number)"><span>{{ String(episode.number).padStart(2,'0') }}</span><span>{{ episode.name }}</span><Check v-if="selectedNumbers.includes(episode.number)" :size="14" /></Button></div>
    <Pagination v-if="episodes.length > 24" :page="page" :total="episodes.length" :page-size="24" @change="page = $event" />
    </section>
    <template #footer><span>点击集目填入所选文件</span><Button @click="emit('close')">返回表格</Button></template>
  </SidePanel>
</template>
