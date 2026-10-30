<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, LoaderCircle } from '@lucide/vue'
import type { Episode, RecognitionItem } from '../../api'
import { name } from '../../api'
import Modal from '../../components/Modal.vue'
import Select from '../../components/ui/Select.vue'
import { episodeComparison } from './episodes'

const props = defineProps<{
  episodes: Episode[]
  files: RecognitionItem[]
  season: number
  loading: boolean
  error: string
  locked: boolean
}>()
const emit = defineEmits<{
  retry: []
  edit: [file: RecognitionItem]
  assign: [file: RecognitionItem, episode: number]
}>()
const rows = computed(() => episodeComparison(props.episodes, props.files, props.season))
const matched = computed(() => rows.value.filter(row => row.files.length).length)
const assigning = ref<Episode | null>(null)
const path = ref('')
function submit() {
  const file = props.files.find(file => file.path === path.value)
  if (!file || !assigning.value || props.locked) return
  emit('assign', file, assigning.value.number)
  assigning.value = null
}
</script>

<template>
  <div class="workspace-comparison">
    <div v-if="loading" class="empty" role="status"><LoaderCircle class="spin" :size="18" />正在获取 TMDB 单集…</div>
    <div v-else-if="error" class="notice error" role="alert">{{ error }} <button class="text-button" @click="emit('retry')">重新获取</button></div>
    <template v-else>
      <p class="comparison-summary">已对应 {{ matched }} / {{ rows.length }} 集 · 缺少文件 {{ rows.length - matched }} 集</p>
      <p v-if="!rows.length" class="empty">当前季暂无单集信息。</p>
      <article v-for="row in rows" :key="row.episode.number" class="comparison-row">
        <span class="auto-episode">{{ String(row.episode.number).padStart(2, '0') }}</span>
        <div class="comparison-main">
          <strong>{{ row.episode.name || `第 ${row.episode.number} 集` }}</strong>
          <template v-if="row.files.length">
            <button v-for="file in row.files" :key="file.path" class="comparison-file" :title="file.path" :disabled="locked" @click="emit('edit', file)">{{ name(file.path) }}</button>
          </template>
          <p v-else class="muted tiny">{{ row.episode.airDate || '暂无播出日期' }} · 缺少文件</p>
        </div>
        <Check v-if="row.files.length" class="green" :size="16" aria-label="已对应" />
        <button v-else class="text-button" :disabled="locked || !files.length" @click="assigning = row.episode; path = ''">关联文件</button>
      </article>
    </template>
  </div>
  <Modal v-if="assigning" title="关联单集文件" @close="assigning = null">
    <p class="muted">将所选文件修正为第 {{ season }} 季第 {{ assigning.number }} 集。原季集对应关系会被替换；合并集请在文件的「修正季集」中填写。</p>
    <Select v-model="path" label="选择要关联的文件" placeholder="选择本作品中的文件" :options="files.map(file => ({ value: file.path, label: name(file.path) }))" :disabled="locked" />
    <template #footer><span /><button class="button primary" :disabled="locked || !path" @click="submit">关联此文件</button></template>
  </Modal>
</template>

<style scoped>
.workspace-comparison { flex:1; min-height:0; overflow:auto; }
.comparison-summary { padding:10px 18px; font-size:12px; color:var(--muted); border-bottom:1px solid var(--line); }
.comparison-row { display:flex; align-items:flex-start; gap:12px; padding:14px 18px; border-bottom:1px solid var(--line); }
.comparison-main { flex:1; min-width:0; display:grid; gap:5px; }
.comparison-main strong { font-size:13px; }
.comparison-file { border:0; background:none; padding:0; text-align:left; color:var(--purple); font-size:12px; overflow-wrap:anywhere; }
@media(max-width:900px) { .workspace-comparison { overflow:visible; } }
</style>
