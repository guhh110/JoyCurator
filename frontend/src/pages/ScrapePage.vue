<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { ArrowRight, Check, CircleAlert, Film, FolderOpen, Image as ImageIcon, Layers3, FileText, RefreshCw } from '@lucide/vue'
import { api, bytes, type MediaMapping, type Run, type ScrapePreview } from '../api'
import { useRuntime } from '../app/runtime'
import PageHeader from '../components/ui/PageHeader.vue'
import QueryState from '../components/ui/QueryState.vue'
const runtime = useRuntime()
const queue = useQuery({ queryKey: ['scrape', 'queue'], queryFn: ({ signal }) => api<MediaMapping[]>('/scrape/queue', undefined, 'GET', { signal }), refetchInterval: 5000 })
const selected = ref<string[]>([])
const preview = ref<ScrapePreview | null>(null)
const busy = ref(false)
const error = ref('')
const activeGroup = ref('')
const groups = computed(() => {
  const map = new Map<string, MediaMapping[]>()
  for (const item of queue.data.value ?? []) {
    const key = `${item.type}:${item.tmdbId}`
    map.set(key, [...(map.get(key) ?? []), item])
  }
  return [...map.entries()].map(([key, items]) => ({ key, items, type: items[0]!.type, id: items[0]!.tmdbId,
    ready: items.filter(item => item.state !== 'unavailable').length,
    title: items[0]!.path.split(/[\\/]/).slice(items[0]!.type === 'tv' ? -3 : -2, items[0]!.type === 'tv' ? -2 : -1)[0] || `TMDB ${items[0]!.tmdbId}` }))
})
const currentGroup = computed(() => groups.value.find(group => group.key === activeGroup.value) ?? groups.value[0])
const current = computed(() => currentGroup.value?.items ?? [])
const seasons = computed(() => {
  const map = new Map<number, Map<string, MediaMapping[]>>()
  for (const item of current.value) {
    const episodes = map.get(item.season) ?? new Map<string, MediaMapping[]>()
    const key = item.episodes.join(',')
    episodes.set(key, [...(episodes.get(key) ?? []), item])
    map.set(item.season, episodes)
  }
  return [...map.entries()].sort(([a], [b]) => a - b).map(([number, episodes]) => ({
    number, episodes: [...episodes.entries()].sort(([a], [b]) => Number(a.split(',')[0]) - Number(b.split(',')[0]))
      .map(([key, items]) => ({ key, items })),
    items: [...episodes.values()].flat(),
  }))
})
const stateNames: Record<MediaMapping['state'], string> = { pending: '待刮削', completed: '已完成', failed: '失败', unavailable: '视频不可用' }
const counts = computed(() => current.value.reduce((result, item) => {
  result[item.state] = (result[item.state] ?? 0) + 1
  return result
}, {} as Partial<Record<MediaMapping['state'], number>>))
const selectedCount = computed(() => selected.value.length)
const availableCount = computed(() => current.value.filter(item => item.state !== 'unavailable').length)
const artifactGroups = computed(() => {
  const artifacts = preview.value?.artifacts ?? []
  return [
    { key: 'nfo', title: '元数据文件', description: '作品、季度及单集的独立资料', icon: FileText, files: artifacts.filter(path => path.toLowerCase().endsWith('.nfo')) },
    { key: 'image', title: '图片文件', description: '作品主图、季海报与单集封面', icon: ImageIcon, files: artifacts.filter(path => !path.toLowerCase().endsWith('.nfo')) },
  ].filter(group => group.files.length)
})
const selectedGroup = computed(() => {
  const path = selected.value[0]
  const item = queue.data.value?.find(entry => entry.path === path)
  return item ? `${item.type}:${item.tmdbId}` : ''
})
function selectItems(items: MediaMapping[]) {
  const paths = items.filter(item => item.state !== 'unavailable').map(item => item.path)
  const group = items[0] ? `${items[0].type}:${items[0].tmdbId}` : ''
  selected.value = [...new Set([...(selectedGroup.value === group ? selected.value : []), ...paths])]
  preview.value = null
}
function switchGroup(key: string) {
  if (activeGroup.value === key) return
  activeGroup.value = key
  selected.value = []
  preview.value = null
  error.value = ''
}
function fileName(path: string) { return path.split(/[\\/]/).pop() ?? path }
function parentName(path: string) { return path.split(/[\\/]/).slice(-2, -1)[0] ?? '' }
function episodeLabel(episodes: number[]) { return episodes.length ? episodes.map(number => `E${String(number).padStart(2, '0')}`).join(' + ') : '影片' }
function toggle(item: MediaMapping) {
  if (item.state === 'unavailable') return
  selected.value = selected.value.includes(item.path) ? selected.value.filter(path => path !== item.path) : [...(selectedGroup.value === `${item.type}:${item.tmdbId}` ? selected.value : []), item.path]
  preview.value = null
}
async function prepare() {
  if (!selected.value.length) return
  busy.value = true
  error.value = ''
  try { preview.value = await api<ScrapePreview>('/scrape/preview', { paths: selected.value }) }
  catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
async function execute() {
  if (!preview.value || preview.value.issues.length) return
  busy.value = true
  error.value = ''
  try {
    const run = await api<Run>('/scrape/execute', { id: preview.value.id })
    runtime.startRun(run)
    selected.value = []
    preview.value = null
    void queue.refetch()
  } catch (e) { error.value = (e as Error).message }
  finally { busy.value = false }
}
</script>
<template>
  <section class="scrape-page" aria-label="媒体刮削">
    <PageHeader title="媒体刮削" eyebrow="ARTWORK & METADATA" description="从已经整理的媒体中，挑选作品与集数，单独补齐资料和图片。" />
    <QueryState :loading="queue.isPending.value" :error="queue.error.value" @retry="queue.refetch()">
      <div v-if="!groups.length" class="pane scrape-empty">
        <div class="empty-art"><Layers3 :size="34" :stroke-width="1.5" /></div>
        <span class="eyebrow">YOUR LIBRARY, NEXT CHAPTER</span>
        <h2>还没有等待刮削的媒体</h2>
        <p>先整理视频与字幕。目标视频登记后，就能在这里为它们补齐 NFO、海报和剧照。</p>
        <RouterLink class="button primary" to="/recognition">前往整理工作区 <ArrowRight :size="15" /></RouterLink>
      </div>
      <template v-else>
        <div class="scrape-overview">
          <div class="overview-heading"><span class="eyebrow">01 / CHOOSE A TITLE</span><h2>从片库中挑选作品</h2><p>每次刮削一部作品；已经完成的媒体也可重新选择。</p></div>
          <div class="work-list" role="group" aria-label="选择作品">
            <button v-for="group in groups" :key="group.key" class="work-card" :class="{ active: currentGroup?.key === group.key }" :aria-pressed="currentGroup?.key === group.key" @click="switchGroup(group.key)">
              <span class="work-art" :class="group.type"><Film v-if="group.type === 'movie'" :size="23" :stroke-width="1.6" /><Layers3 v-else :size="23" :stroke-width="1.6" /></span>
              <span class="work-info"><span class="work-type">{{ group.type === 'tv' ? 'SERIES' : 'FILM' }} <span>· TMDB {{ group.id }}</span></span><strong :title="group.title">{{ group.title }}</strong><small>{{ group.items.length }} 个视频版本 · {{ group.ready }} 个可刮削</small></span>
              <span class="work-arrow"><ArrowRight :size="17" /></span>
            </button>
          </div>
        </div>
        <div class="scrape-workspace pane">
          <div class="workspace-topline"><span class="eyebrow">02 / SELECT MEDIA</span><span class="workspace-id">{{ currentGroup?.type === 'tv' ? '剧集' : '电影' }} · TMDB {{ currentGroup?.id }}</span></div>
          <div class="workspace-intro"><div><h2>选择刮削范围</h2><p>同集的不同视频版本各自生成同名产物；共享的剧集和季资料只生成一次。</p></div><button class="button small" :disabled="busy || !availableCount" @click="selectItems(current)">选择全部可用</button></div>
          <div class="state-strip" aria-label="待办状态"><span><i class="state-dot pending" />{{ counts.pending ?? 0 }} 待办</span><span><i class="state-dot failed" />{{ counts.failed ?? 0 }} 失败</span><span><i class="state-dot completed" />{{ counts.completed ?? 0 }} 完成</span><span><i class="state-dot unavailable" />{{ counts.unavailable ?? 0 }} 不可用</span></div>
          <div class="season-list">
            <section v-for="season in seasons" :key="season.number" class="season-section">
              <div v-if="currentGroup?.type === 'tv'" class="season-heading"><span class="season-number">S{{ String(season.number).padStart(2, '0') }}</span><div><h3>第 {{ season.number }} 季</h3><p>{{ season.episodes.length }} 集 · {{ season.items.length }} 个视频版本</p></div><button class="text-button" :disabled="busy || !season.items.some(item => item.state !== 'unavailable')" @click="selectItems(season.items)">选择本季 <ArrowRight :size="14" /></button></div>
              <div v-for="episode in season.episodes" :key="episode.key" class="episode-block">
                <div v-if="currentGroup?.type === 'tv'" class="episode-heading"><span class="episode-index">{{ episodeLabel(episode.items[0]?.episodes ?? []) }}</span><span class="episode-versions">{{ episode.items.length }} 个版本</span><button class="episode-select" :disabled="busy || !episode.items.some(item => item.state !== 'unavailable')" @click="selectItems(episode.items)">选择本集</button></div>
                <label v-for="item in episode.items" :key="item.path" class="video-row" :class="{ selected: selected.includes(item.path), unavailable: item.state === 'unavailable' }">
                  <input type="checkbox" :checked="selected.includes(item.path)" :disabled="item.state === 'unavailable' || busy" @change="toggle(item)" />
                  <span class="video-icon"><Film :size="17" :stroke-width="1.7" /></span>
                  <span class="video-info"><strong :title="item.path">{{ fileName(item.path) }}</strong><small :title="item.path"><FolderOpen :size="12" /> {{ parentName(item.path) }}</small></span>
                  <span class="status-pill" :class="item.state">{{ stateNames[item.state] }}</span>
                </label>
              </div>
            </section>
          </div>
          <div class="selection-footer"><span><strong>{{ selectedCount }}</strong> / {{ availableCount }} 个版本已选择</span><button class="button primary" :disabled="!selectedCount || busy" @click="prepare">预览刮削 <ArrowRight :size="15" /></button></div>
        </div>
        <section v-if="preview" class="scrape-preview pane" aria-label="刮削预览">
          <div class="preview-heading"><div><span class="eyebrow">03 / REVIEW OUTPUT</span><h2>{{ preview.title }}</h2><p>{{ preview.targets.length }} 个视频版本将生成 {{ preview.artifacts.length }} 项文件</p></div><span class="space-badge">预计占用 <strong>{{ bytes(preview.required) }}</strong></span></div>
          <div v-if="preview.issues.length" class="preview-issues"><CircleAlert :size="17" /><div><strong>预览存在冲突</strong><p v-for="issue in preview.issues" :key="issue">{{ issue }}</p></div></div>
          <div class="artifact-sections"><div v-for="group in artifactGroups" :key="group.key" class="artifact-section"><div class="artifact-section-heading"><span class="artifact-icon"><component :is="group.icon" :size="18" /></span><div><strong>{{ group.title }}</strong><small>{{ group.description }}</small></div><span class="artifact-total">{{ group.files.length }}</span></div><div class="artifact-list"><div v-for="artifact in group.files" :key="artifact" class="artifact-row" :title="artifact"><span class="artifact-mark"><Check :size="12" /></span><span><strong>{{ fileName(artifact) }}</strong><small>{{ parentName(artifact) }}</small></span></div></div></div></div>
          <div class="preview-footer"><div class="overwrite-note"><RefreshCw :size="16" /><span>同名 NFO 与图片会无备份覆盖。撤销仅能移除本次仍拥有的文件，<strong>无法恢复旧内容</strong>。</span></div><button class="button primary" :disabled="busy || !!preview.issues.length || !preview.artifacts.length" @click="execute">确认刮削 <ArrowRight :size="15" /></button></div>
        </section>
      </template>
      <p v-if="error" class="notice error">{{ error }}</p>
    </QueryState>
  </section>
</template>
<style scoped>
.scrape-page { --accent:#6553bd; --accent-soft:#f2effa; --ink:#292637; --hairline:#e8e5ef; display:grid;gap:17px;min-width:0 }
.eyebrow { color:var(--accent);font-size:10px;font-weight:750;letter-spacing:.16em }
.scrape-empty { min-height:320px;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;gap:13px;padding:34px }
.scrape-empty .empty-art { width:72px;height:72px;border-radius:19px;display:grid;place-items:center;color:var(--accent);background:radial-gradient(circle at 25% 20%,white,#e9e3f8);box-shadow:0 14px 30px #6454a018 }
.scrape-empty h2 { font-size:20px;color:var(--ink) }
.scrape-empty p { max-width:380px;color:var(--muted);font-size:12px }
.scrape-overview { padding:20px;border:1px solid var(--hairline);border-radius:12px;background:linear-gradient(135deg,#f0ecfa 0%,#f8f6fc 48%,#fff 100%);box-shadow:var(--shadow) }
.overview-heading h2 { margin:5px 0 2px;color:var(--ink);font-size:19px;letter-spacing:-.03em }
.overview-heading p { color:var(--muted);font-size:12px }
.work-list { display:grid;grid-template-columns:repeat(auto-fit,minmax(230px,1fr));gap:10px;margin-top:18px }
.work-card { display:flex;align-items:center;gap:13px;min-width:0;min-height:88px;padding:12px;text-align:left;border:1px solid #e5e0ef;border-radius:10px;background:#fff;box-shadow:0 2px 8px #34285108 }
.work-card:hover { border-color:#b9abdf;background:#fff }
.work-card.active { border-color:#9280d0;box-shadow:0 0 0 2px #e5dff7,0 6px 16px #49378214 }
.work-art { flex:none;width:50px;height:58px;display:grid;place-items:center;border-radius:7px;color:#fff;background:linear-gradient(145deg,#6c5ab8,#a497d8);box-shadow:inset 0 1px #ffffff60 }
.work-art.movie { background:linear-gradient(145deg,#3d6874,#78a5a8) }
.work-info { display:grid;gap:3px;min-width:0 }
.work-type { color:var(--accent);font-size:9px;font-weight:750;letter-spacing:.12em }
.work-type span { color:#9b96a5;letter-spacing:0;font-weight:500 }
.work-info strong { overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:14px;color:var(--ink) }
.work-info small { color:var(--muted);font-size:10px }
.work-arrow { margin-left:auto;color:#a9a1b8 }
.work-card.active .work-arrow { color:var(--accent) }
.scrape-workspace { padding:0;overflow:hidden }
.workspace-topline { display:flex;align-items:center;justify-content:space-between;gap:12px;padding:15px 20px 0 }
.workspace-id { color:#9993a5;font-size:11px }
.workspace-intro { display:flex;align-items:center;justify-content:space-between;gap:16px;padding:7px 20px 17px }
.workspace-intro h2 { color:var(--ink);font-size:20px;letter-spacing:-.03em }
.workspace-intro p { margin-top:4px;color:var(--muted);font-size:11px }
.state-strip { display:flex;gap:19px;flex-wrap:wrap;padding:11px 20px;border-top:1px solid var(--hairline);border-bottom:1px solid var(--hairline);background:#faf9fc;color:#666273;font-size:11px }
.state-strip span { display:inline-flex;align-items:center;gap:6px }
.state-dot { width:6px;height:6px;border-radius:50%;background:#9991b7 }
.state-dot.failed { background:#c2626b }.state-dot.completed { background:#4c9b7a }.state-dot.unavailable { background:#c7a86e }
.season-list { padding:4px 20px 0 }
.season-section { padding:12px 0 17px;border-bottom:1px solid var(--hairline) }
.season-section:last-child { border-bottom:0 }
.season-heading { display:flex;align-items:center;gap:11px;margin:0 0 12px }
.season-number { width:42px;height:42px;flex:none;display:grid;place-items:center;border-radius:9px;background:var(--accent-soft);color:var(--accent);font-size:12px;font-weight:750;letter-spacing:.04em }
.season-heading h3 { font-size:14px;font-weight:700;color:var(--ink) }
.season-heading p { margin-top:1px;font-size:10px;color:var(--muted) }
.season-heading .text-button { display:inline-flex;align-items:center;gap:5px;margin-left:auto;color:var(--accent);font-size:11px;font-weight:700 }
.episode-block { overflow:hidden;margin:8px 0;border:1px solid var(--hairline);border-radius:9px;background:#fff }
.episode-heading { display:flex;align-items:center;gap:9px;padding:9px 13px;border-bottom:1px solid #efedf3;background:#fcfbfd }
.episode-index { color:#474052;font-size:11px;font-weight:750;letter-spacing:.04em }
.episode-versions { padding-left:9px;border-left:1px solid #e5e0ea;color:#a19baa;font-size:10px }
.episode-select { margin-left:auto;padding:3px 6px;border:0;border-radius:5px;background:transparent;color:var(--accent);font-size:10px;font-weight:700 }
.episode-select:hover:not(:disabled) { background:var(--accent-soft) }
.video-row { display:flex;align-items:center;gap:10px;min-width:0;padding:11px 13px;border-top:1px solid #f2f0f5;cursor:pointer }
.video-row:first-child { border-top:0 }
.video-row:hover:not(.unavailable) { background:#faf9fd }
.video-row.selected { background:#f8f5ff }
.video-row.unavailable { opacity:.62;cursor:not-allowed }
.video-row input { flex:none;accent-color:var(--accent);margin:0 }
.video-icon { display:grid;place-items:center;flex:none;width:30px;height:30px;border-radius:7px;background:#f1eff6;color:#8e84ac }
.video-info { display:grid;gap:2px;min-width:0;flex:1 }
.video-info strong { overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--ink);font-size:12px }
.video-info small { display:flex;align-items:center;gap:4px;margin:0;color:#9b96a4;font-size:10px }
.status-pill { flex:none;padding:5px 7px;border-radius:5px;background:#f3f0fa;color:#7663b0;font-size:10px;font-weight:700 }
.status-pill.failed { background:#fdf0f0;color:#bc5865 }.status-pill.completed { background:#edf7f2;color:#328064 }.status-pill.unavailable { background:#f7f3ec;color:#9a7c48 }
.selection-footer { display:flex;align-items:center;justify-content:space-between;gap:12px;padding:15px 20px;border-top:1px solid var(--hairline);background:#fcfbfe;color:var(--muted);font-size:11px }
.selection-footer strong { color:var(--accent);font-size:16px }
.scrape-preview { padding:0;overflow:hidden }
.preview-heading { display:flex;justify-content:space-between;align-items:center;gap:14px;padding:20px;background:linear-gradient(110deg,#f5f1fc,#fff 80%) }
.preview-heading h2 { margin:5px 0 2px;font-size:20px;color:var(--ink) }
.preview-heading p { font-size:11px;color:var(--muted) }
.space-badge { display:grid;gap:2px;flex:none;padding:8px 12px;border:1px solid #e6e0f3;border-radius:8px;background:#fff;color:var(--muted);font-size:10px }
.space-badge strong { color:var(--ink);font-size:14px }
.preview-issues { display:flex;gap:9px;margin:14px 20px;padding:13px;color:var(--red);border-radius:8px;background:#fff2f2;font-size:11px }
.preview-issues p { margin-top:3px }
.artifact-sections { display:grid;gap:13px;padding:17px 20px }
.artifact-section { overflow:hidden;border:1px solid var(--hairline);border-radius:9px }
.artifact-section-heading { display:flex;align-items:center;gap:10px;padding:11px 12px;background:#faf9fc;border-bottom:1px solid var(--hairline) }
.artifact-icon { display:grid;place-items:center;width:33px;height:33px;border-radius:7px;background:var(--accent-soft);color:var(--accent) }
.artifact-section-heading > div { display:grid;gap:1px;min-width:0 }
.artifact-section-heading strong { font-size:12px;color:var(--ink) }
.artifact-section-heading small { color:var(--muted);font-size:10px }
.artifact-total { margin-left:auto;color:var(--accent);font-size:13px;font-weight:750 }
.artifact-list { display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));max-height:250px;overflow-y:auto }
.artifact-row { display:flex;align-items:center;gap:8px;min-width:0;padding:9px 12px;border-top:1px solid #f2f0f5 }
.artifact-mark { display:grid;place-items:center;flex:none;width:19px;height:19px;border-radius:50%;background:#edf7f2;color:#3f9672 }
.artifact-row > span:last-child { display:grid;gap:1px;min-width:0 }
.artifact-row strong { overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#393645;font-size:11px }
.artifact-row small { color:#a39eaa;font-size:10px }
.preview-footer { display:flex;align-items:center;justify-content:space-between;gap:15px;padding:15px 20px;border-top:1px solid var(--hairline) }
.overwrite-note { display:flex;align-items:flex-start;gap:8px;max-width:460px;color:#9a7450;font-size:10px;line-height:1.5 }
.overwrite-note svg { flex:none }.overwrite-note strong { color:#80563b }
@media (max-width:600px) {
  .scrape-overview { padding:16px }
  .work-list { grid-template-columns:1fr }
  .workspace-intro { align-items:flex-start;flex-direction:column }
  .state-strip { gap:8px 13px }
  .season-list { padding:3px 12px 0 }
  .video-row { gap:8px;padding:10px 9px }
  .video-icon { display:none }
  .status-pill { max-width:72px;white-space:nowrap }
  .selection-footer,.preview-footer { align-items:stretch;flex-direction:column }
  .selection-footer .button,.preview-footer .button { width:100% }
  .preview-heading { align-items:flex-start;flex-direction:column }
  .artifact-list { grid-template-columns:1fr }
}
</style>
