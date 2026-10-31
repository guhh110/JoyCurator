<script setup lang="ts">
import { computed, nextTick, ref, watch, onBeforeUnmount } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ArrowLeft, ArrowRight, Check, ChevronDown, CircleAlert, Film, FolderOpen, Image as ImageIcon, Layers3, FileText, RefreshCw, Search } from '@lucide/vue'
import { api, bytes, type MediaMapping, type Run, type ScrapePreview, type ScrapeWork, type ScrapeWorkPage } from '../api'
import { useRuntime } from '../app/runtime'
import { notify } from '../app/toast'
import EliText from '../components/ui/EliText.vue'
import PageHeader from '../components/ui/PageHeader.vue'
import QueryState from '../components/ui/QueryState.vue'
import { addSelection, artifactSummary, hasMultipleVersions, visibleEpisodes, type ScrapeStateFilter } from '../features/scrape/selection'

const runtime = useRuntime()
const client = useQueryClient()
const search = ref('')
const query = ref('')
const works = ref<ScrapeWork[]>([])
const cursor = ref('')
const loadingMore = ref(false)
const active = ref<ScrapeWork | null>(null)
const selected = ref<string[]>([])
const expandedSeason = ref<number | null>(null)
const expandedEpisode = ref('')
const stateFilter = ref<ScrapeStateFilter>('all')
const fileSearch = ref('')
const preview = ref<ScrapePreview | null>(null)
const previewElement = ref<HTMLElement | null>(null)
const artifactSearch = ref('')
const showArtifacts = ref(false)
const busy = ref(false)
const worksQuery = useQuery({
  queryKey: computed(() => ['scrape', 'works', query.value]),
  queryFn: ({ signal }) => api<ScrapeWorkPage>(`/scrape/works?q=${encodeURIComponent(query.value)}`, undefined, 'GET', { signal }),
  refetchInterval: 15000,
})
watch(query, () => { works.value = []; cursor.value = '' })
watch(() => worksQuery.data.value, page => {
  if (!page) return
  if (!works.value.length || !cursor.value) {
    works.value = page.items
    cursor.value = page.nextCursor
    return
  }
  const refreshed = new Map(page.items.map(item => [`${item.type}:${item.tmdbId}`, item]))
  works.value = works.value.map(item => refreshed.get(`${item.type}:${item.tmdbId}`) ?? item)
}, { immediate: true })
let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, value => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { query.value = value.trim() }, 250)
})
onBeforeUnmount(() => clearTimeout(searchTimer))
const detail = useQuery({
  queryKey: computed(() => ['scrape', 'work', active.value?.type, active.value?.tmdbId]),
  queryFn: ({ signal }) => api<MediaMapping[]>(`/scrape/works/${active.value!.type}/${active.value!.tmdbId}`, undefined, 'GET', { signal }),
  enabled: computed(() => !!active.value),
  refetchInterval: 10000,
})
const current = computed(() => active.value && detail.data.value?.every(item => item.type === active.value?.type && item.tmdbId === active.value?.tmdbId) ? detail.data.value : [])
watch(() => detail.data.value, items => {
  if (active.value && items) {
    const available = selected.value.filter(path => items.some(item => item.path === path && item.state !== 'unavailable'))
    if (available.length !== selected.value.length) {
      selected.value = available
      preview.value = null
    }
  }
})
const counts = computed(() => current.value.reduce((result, item) => {
  result[item.state] = (result[item.state] ?? 0) + 1
  return result
}, {} as Partial<Record<MediaMapping['state'], number>>))
const seasons = computed(() => visibleEpisodes(current.value, stateFilter.value, fileSearch.value))
const activeSeasonItems = (number: number) => current.value.filter(item => item.season === number)
const activeEpisodeItems = (season: number, key: string) => current.value.filter(item => item.season === season && item.episodes.join(',') === key)
const hasMultipleVersionsInWork = (season: number, key: string) => hasMultipleVersions(current.value, season, key)
const bulkReady = (items: MediaMapping[], completed: boolean) => items.filter(item => completed ? item.state === 'completed' : item.state === 'pending' || item.state === 'failed').length
const artifactCounts = computed(() => preview.value ? artifactSummary(preview.value) : null)
const filteredArtifacts = computed(() => (preview.value?.artifacts ?? []).filter(path => path.toLocaleLowerCase().includes(artifactSearch.value.trim().toLocaleLowerCase())))
const stateNames: Record<MediaMapping['state'], string> = { pending: '待刮削', completed: '已完成', failed: '失败', unavailable: '视频不可用' }
const tabs: { key: ScrapeStateFilter; label: string }[] = [
  { key: 'all', label: '全部' }, { key: 'pending', label: '待办' }, { key: 'failed', label: '失败' },
  { key: 'completed', label: '已完成' }, { key: 'unavailable', label: '不可用' },
]
function fileName(path: string) { return path.split(/[\\/]/).pop() ?? path }
function parentName(path: string) { return path.split(/[\\/]/).slice(-2, -1)[0] ?? '' }
function workTitle(work: ScrapeWork) {
  const parts = work.representative.split(/[\\/]/)
  return parts[parts.length - (work.type === 'tv' ? 3 : 2)] || `TMDB ${work.tmdbId}`
}
function episodeLabel(episodes: number[]) { return episodes.length ? episodes.map(number => `E${String(number).padStart(2, '0')}`).join(' + ') : '影片' }
function openWork(work: ScrapeWork) {
  if (active.value?.type === work.type && active.value.tmdbId === work.tmdbId) return
  active.value = work
  selected.value = []
  preview.value = null
  expandedSeason.value = null
  expandedEpisode.value = ''
  fileSearch.value = ''
  stateFilter.value = 'all'
}
function closeWork() { active.value = null; selected.value = []; preview.value = null }
async function loadMore() {
  if (!cursor.value || loadingMore.value || worksQuery.isFetching.value) return
  const requestQuery = query.value
  const requestCursor = cursor.value
  loadingMore.value = true
  try {
    const page = await api<ScrapeWorkPage>(`/scrape/works?q=${encodeURIComponent(requestQuery)}&cursor=${encodeURIComponent(requestCursor)}`)
    if (requestQuery !== query.value || requestCursor !== cursor.value) return
    works.value = [...works.value, ...page.items.filter(item => !works.value.some(old => old.type === item.type && old.tmdbId === item.tmdbId))]
    cursor.value = page.nextCursor
  } catch (e) { notify((e as Error).message, 'error') }
  finally { loadingMore.value = false }
}
function bulk(items: MediaMapping[], completed = false) {
  const next = addSelection(selected.value, items, completed)
  if (!next) { notify('单次最多选择 1000 个视频，请按季或单集拆分刮削。', 'error'); return }
  selected.value = next; preview.value = null
}
function toggle(item: MediaMapping) {
  if (item.state === 'unavailable' || (item.state === 'completed' && stateFilter.value !== 'completed') || busy.value) return
  if (selected.value.includes(item.path)) selected.value = selected.value.filter(path => path !== item.path)
  else if (selected.value.length >= 1000) { notify('单次最多选择 1000 个视频，请按季拆分刮削。', 'error'); return }
  else selected.value = [...selected.value, item.path]
  preview.value = null
}
async function prepare() {
  if (!selected.value.length || busy.value) return
  const work = `${active.value?.type}:${active.value?.tmdbId}`
  const paths = [...selected.value]
  busy.value = true
  try {
    const result = await api<ScrapePreview>('/scrape/preview', { paths })
    if (`${active.value?.type}:${active.value?.tmdbId}` !== work || paths.join('\0') !== selected.value.join('\0')) return
    preview.value = result
    showArtifacts.value = false
    artifactSearch.value = ''
    await nextTick()
    previewElement.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
  catch (e) { notify((e as Error).message, 'error') }
  finally { busy.value = false }
}
async function execute() {
  if (!preview.value || preview.value.issues.length || busy.value) return
  busy.value = true
  try {
    const run = await api<Run>('/scrape/execute', { id: preview.value.id })
    runtime.startRun(run)
    selected.value = []; preview.value = null
    await Promise.all([client.invalidateQueries({ queryKey: ['scrape', 'works'] }), client.invalidateQueries({ queryKey: ['scrape', 'work'] })])
  } catch (e) { notify((e as Error).message, 'error') }
  finally { busy.value = false }
}
</script>
<template>
  <section class="scrape-page" aria-label="媒体刮削">
    <PageHeader title="媒体刮削" eyebrow="LIBRARY / SCRAPE" description="按作品和季度挑选需要补齐的媒体资料。" />
    <div class="scrape-shell" :class="{ 'has-detail': active }">
      <aside class="catalog pane" aria-label="作品列表">
        <div class="catalog-heading"><div><span class="eyebrow">01 / LIBRARY</span><h2>片库</h2></div><span class="catalog-total">{{ works.length }} 部已加载</span></div>
        <label class="search-field"><Search :size="16" /><input v-model="search" type="search" placeholder="搜索作品、文件名或 TMDB ID" aria-label="搜索作品或文件" /></label>
        <QueryState :loading="worksQuery.isPending.value || worksQuery.isFetching.value && !worksQuery.data.value" :error="worksQuery.error.value" @retry="worksQuery.refetch()">
          <div v-if="!works.length" class="catalog-empty"><Layers3 :size="28" /><p>{{ query ? '没有匹配的作品，试试其他关键词。' : '暂无已整理的视频。' }}</p><RouterLink v-if="!query" class="button small" to="/recognition">前往整理工作区</RouterLink></div>
          <div v-else class="work-list">
            <button v-for="work in works" :key="`${work.type}:${work.tmdbId}`" class="work-row" :class="{ active: active?.type === work.type && active?.tmdbId === work.tmdbId }" @click="openWork(work)">
              <span class="work-glyph" :class="work.type"><Film v-if="work.type === 'movie'" :size="18" /><Layers3 v-else :size="18" /></span>
              <span class="work-info"><strong :title="work.representative">{{ workTitle(work) }}</strong><small>{{ work.type === 'tv' ? '剧集' : '电影' }} · TMDB {{ work.tmdbId }} · {{ work.total }} 个版本</small><span class="work-progress"><i :style="{ width: `${work.total ? work.completed / work.total * 100 : 0}%` }" /></span></span>
              <span v-if="work.pending + work.failed" class="work-need">{{ work.pending + work.failed }}</span>
              <ArrowRight v-else :size="14" class="work-chevron" />
            </button>
            <button v-if="cursor" class="load-more" :disabled="loadingMore" @click="loadMore">{{ loadingMore ? '正在加载…' : '加载更多作品' }} <ChevronDown :size="14" /></button>
          </div>
        </QueryState>
      </aside>
      <div class="detail-area">
        <div v-if="!active" class="detail-empty pane"><div class="empty-mark"><Layers3 :size="34" /></div><span class="eyebrow">02 / WORKSPACE</span><h2>选一部作品，开始补齐资料</h2><p>无需翻页。进入作品后按季度展开，只处理想要的剧集与版本。</p></div>
        <template v-else>
          <div v-if="!preview" class="work-detail pane">
            <div class="detail-header"><button class="back-button" @click="closeWork"><ArrowLeft :size="16" />返回作品</button><div class="detail-title"><span class="eyebrow">{{ active.type === 'tv' ? 'SERIES' : 'FILM' }} · TMDB {{ active.tmdbId }}</span><h2 :title="active.representative">{{ workTitle(active) }}</h2></div></div>
            <QueryState :loading="detail.isPending.value" :error="detail.error.value" @retry="detail.refetch()">
              <div class="detail-toolbar"><div class="status-tabs" aria-label="按状态筛选"><button v-for="tab in tabs" :key="tab.key" :class="{ active: stateFilter === tab.key }" @click="stateFilter = tab.key">{{ tab.label }} <small>{{ tab.key === 'all' ? current.length : (counts[tab.key] ?? 0) }}</small></button></div><label class="search-field file-search"><Search :size="15" /><input v-model="fileSearch" type="search" placeholder="查找季集或文件名" aria-label="查找季集或文件名" /></label></div>
              <div class="bulk-bar"><span>当前作品 · {{ current.length }} 个版本<span v-if="counts.unavailable"> · {{ counts.unavailable }} 个不可用</span></span><div><button class="button small" :disabled="busy || !bulkReady(current, false)" @click="bulk(current)">选整部待办/失败 ({{ bulkReady(current, false) }})</button><button v-if="stateFilter === 'completed'" class="button small" :disabled="busy || !bulkReady(current, true)" @click="bulk(current, true)">重刮整部已完成 ({{ bulkReady(current, true) }})</button><button class="text-button" :disabled="!selected.length" @click="selected = []; preview = null">清空选择</button></div></div>
              <div v-if="!seasons.length" class="no-matches">没有符合筛选条件的文件。</div>
              <div v-if="active.type === 'movie'" class="movie-versions"><label v-for="item in current.filter(item => stateFilter === 'all' || item.state === stateFilter).filter(item => !fileSearch || item.path.toLocaleLowerCase().includes(fileSearch.toLocaleLowerCase()))" :key="item.path" class="video-row" :class="{ selected: selected.includes(item.path), unavailable: item.state === 'unavailable' }"><input type="checkbox" :checked="selected.includes(item.path)" :disabled="busy || item.state === 'unavailable' || (item.state === 'completed' && stateFilter !== 'completed')" @change="toggle(item)" /><span class="video-info"><strong :title="item.path">{{ fileName(item.path) }}</strong><small><FolderOpen :size="12" /> {{ parentName(item.path) }}</small></span><span class="status-pill" :class="item.state">{{ stateNames[item.state] }}</span></label></div>
              <div v-else class="season-list">
                <section v-for="season in seasons" :key="season.number" class="season-section">
                  <div class="season-head">
                    <button class="season-expand" :aria-expanded="expandedSeason === season.number" @click="expandedSeason = expandedSeason === season.number ? null : season.number; expandedEpisode = ''">
                      <span class="season-index">S{{ String(season.number).padStart(2, '0') }}</span>
                      <span><strong>{{ season.number === 0 ? '特别篇' : `第 ${season.number} 季` }}</strong><small>{{ season.episodes.length }} 组单集 · {{ season.episodes.reduce((sum, episode) => sum + episode.items.length, 0) }} 个版本</small></span>
                      <ChevronDown :size="18" class="expand-arrow" />
                    </button>
                    <div class="season-actions">
                      <button class="text-button" :disabled="busy || !bulkReady(activeSeasonItems(season.number), false)" @click="bulk(activeSeasonItems(season.number))">选本季 ({{ bulkReady(activeSeasonItems(season.number), false) }})</button>
                      <button v-if="stateFilter === 'completed'" class="text-button" :disabled="busy || !bulkReady(activeSeasonItems(season.number), true)" @click="bulk(activeSeasonItems(season.number), true)">重刮本季</button>
                    </div>
                  </div>
                  <div v-if="expandedSeason === season.number" class="episode-list">
                    <div v-for="episode in season.episodes" :key="episode.key" class="episode-row">
                      <label v-if="!hasMultipleVersionsInWork(season.number, episode.key)" class="video-row single-episode" :class="{ selected: selected.includes(episode.items[0]!.path), unavailable: episode.items[0]!.state === 'unavailable' }">
                        <input type="checkbox" :checked="selected.includes(episode.items[0]!.path)" :disabled="busy || episode.items[0]!.state === 'unavailable' || (episode.items[0]!.state === 'completed' && stateFilter !== 'completed')" @change="toggle(episode.items[0]!)" />
                        <span class="episode-number">{{ episodeLabel(episode.items[0]!.episodes) }}</span>
                        <span class="video-info"><strong :title="episode.items[0]!.path">{{ fileName(episode.items[0]!.path) }}</strong></span>
                        <span class="status-pill" :class="episode.items[0]!.state">{{ stateNames[episode.items[0]!.state] }}</span>
                      </label>
                      <template v-else>
                        <div class="episode-summary">
                          <button class="episode-expand" :aria-expanded="expandedEpisode === `${season.number}:${episode.key}`" @click="expandedEpisode = expandedEpisode === `${season.number}:${episode.key}` ? '' : `${season.number}:${episode.key}`">
                            <span class="episode-number">{{ episodeLabel(episode.items[0]?.episodes ?? []) }}</span><span>{{ episode.items.length }} 个版本</span><ChevronDown :size="15" />
                          </button>
                          <button class="text-button" :disabled="busy || !bulkReady(activeEpisodeItems(season.number, episode.key), false)" @click="bulk(activeEpisodeItems(season.number, episode.key))">选本集</button>
                          <button v-if="stateFilter === 'completed'" class="text-button" :disabled="busy || !bulkReady(activeEpisodeItems(season.number, episode.key), true)" @click="bulk(activeEpisodeItems(season.number, episode.key), true)">重刮本集</button>
                        </div>
                        <div v-if="expandedEpisode === `${season.number}:${episode.key}`" class="version-list">
                          <label v-for="item in episode.items" :key="item.path" class="video-row" :class="{ selected: selected.includes(item.path), unavailable: item.state === 'unavailable' }">
                            <input type="checkbox" :checked="selected.includes(item.path)" :disabled="busy || item.state === 'unavailable' || (item.state === 'completed' && stateFilter !== 'completed')" @change="toggle(item)" />
                            <span class="video-info"><strong :title="item.path">{{ fileName(item.path) }}</strong></span>
                            <span class="status-pill" :class="item.state">{{ stateNames[item.state] }}</span>
                          </label>
                        </div>
                      </template>
                    </div>
                  </div>
                </section>
              </div>
              <div class="selection-footer"><span><strong>{{ selected.length }}</strong> 个版本已选 · 筛选不影响整部/整季选择</span><button class="button primary" :disabled="!selected.length || busy" @click="prepare">预览刮削 <ArrowRight :size="15" /></button></div>
            </QueryState>
          </div>
          <section v-else ref="previewElement" class="scrape-preview pane" aria-label="刮削预览">
            <div class="preview-head">
              <button class="preview-back" :disabled="busy" @click="preview = null"><ArrowLeft :size="15" />返回选择</button>
              <span class="eyebrow">03 / REVIEW</span>
              <h2>{{ preview.title }}</h2>
              <p>{{ preview.targets.length }} 个视频版本 · {{ preview.artifacts.length }} 项文件 · 预计 {{ bytes(preview.required) }}</p>
            </div>
            <div class="preview-body">
              <div class="summary-grid"><div><strong>{{ artifactCounts?.videoNfo }}</strong><span>版本 NFO</span></div><div><strong>{{ artifactCounts?.sharedNfo }}</strong><span>共享 NFO</span></div><div><strong>{{ artifactCounts?.videoImages }}</strong><span>单集封面</span></div><div><strong>{{ artifactCounts?.sharedImages }}</strong><span>作品/季度图片</span></div></div>
              <div v-if="preview.issues.length" class="preview-issues"><CircleAlert :size="17" /><div><strong>预览存在冲突</strong><p v-for="issue in preview.issues" :key="issue">{{ issue }}</p></div></div>
              <details class="artifact-details" :open="showArtifacts" @toggle="showArtifacts = ($event.target as HTMLDetailsElement).open">
                <summary>查看完整产物路径 <span>{{ preview.artifacts.length }}</span><ChevronDown :size="16" /></summary>
                <div class="artifact-inner"><label class="search-field"><Search :size="14" /><input v-model="artifactSearch" aria-label="筛选产物路径" placeholder="筛选文件路径" /></label><p class="tiny muted">显示 {{ filteredArtifacts.length }} / {{ preview.artifacts.length }} 项</p><div class="artifact-list"><div v-for="artifact in filteredArtifacts" :key="artifact" class="artifact-row"><component :is="artifact.toLowerCase().endsWith('.nfo') ? FileText : ImageIcon" :size="15" /><EliText :text="artifact" ellipsis="start" /><Check :size="13" /></div></div></div>
              </details>
            </div>
            <div class="preview-footer"><div class="overwrite-note"><RefreshCw :size="16" /><span>同名 NFO 与图片会无备份覆盖。撤销只能移除本次仍拥有的文件，<strong>无法恢复旧内容</strong>。</span></div><button class="button primary" :disabled="busy || !!preview.issues.length || !preview.artifacts.length" @click="execute">确认刮削 <ArrowRight :size="15" /></button></div>
          </section>
        </template>
      </div>
    </div>
  </section>
</template>
<style scoped>
.scrape-page { --accent:#247b72; --accent-soft:#e9f5f1; --ink:#1d3035; --hairline:#dce9e5; display:grid;gap:16px;min-width:0 }
.eyebrow { color:var(--accent);font-size:10px;font-weight:750;letter-spacing:.15em }
.scrape-shell { display:grid;grid-template-columns:minmax(250px,320px) minmax(0,1fr);gap:16px;align-items:start;min-height:500px }
.scrape-page .pane { border-color:var(--hairline) }
.scrape-page .button.primary { background:var(--accent);border-color:var(--accent) }
.scrape-page .button.primary:hover:not(:disabled) { background:#17665f;border-color:#17665f }
.scrape-page .text-button:hover { color:#17665f }
.catalog { overflow:hidden;padding:0;background:#f9fbfa }
.catalog-heading { display:flex;align-items:center;justify-content:space-between;padding:11px 14px 7px }
.catalog-heading h2 { margin-top:1px;font-size:18px;line-height:1.15;letter-spacing:-.04em;color:var(--ink) }
.catalog-total { font-size:10px;color:var(--muted) }
.search-field { display:flex;align-items:center;gap:8px;min-width:0;height:36px;border:1px solid var(--hairline);border-radius:8px;background:#fff;color:#67918c;padding:0 10px }
.search-field:focus-within { border-color:var(--accent);box-shadow:0 0 0 3px #247b7215 }
.scrape-page .search-field > input { height:30px;min-height:0;margin:0;padding:0;border:0;outline:0;box-shadow:none;background:transparent;min-width:0;width:100%;font-size:12px;color:var(--ink) }
.catalog > .search-field { margin:0 12px 8px }
.catalog-empty { display:grid;justify-items:center;gap:12px;padding:56px 18px;color:var(--muted);text-align:center;font-size:12px }
.work-list { max-height:min(70vh,780px);overflow:auto;padding:0 8px 10px }
.work-row { width:100%;display:flex;align-items:center;gap:10px;min-width:0;text-align:left;border:0;border-bottom:1px solid #e8efed;padding:11px 9px;background:transparent }
.work-row:hover,.work-row.active { background:#eaf5f1;border-radius:7px }
.work-glyph { flex:none;display:grid;place-items:center;width:36px;height:42px;border-radius:7px;background:#dcefe9;color:var(--accent) }
.work-glyph.movie { background:#e9e9df;color:#777641 }
.work-info { flex:1;min-width:0;display:grid;gap:4px }
.work-info strong { overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--ink);font-size:12px }
.work-info small { color:#758b89;font-size:10px }
.work-progress { width:100%;height:3px;border-radius:3px;background:#d9e7e2;overflow:hidden }
.work-progress i { display:block;height:100%;background:#3aa891 }
.work-need { flex:none;min-width:23px;padding:4px;border-radius:5px;background:#fff0d9;color:#98631f;text-align:center;font-size:10px;font-weight:750 }
.work-chevron { color:#8da5a0 }
.load-more { display:flex;align-items:center;justify-content:center;gap:7px;width:100%;border:0;background:transparent;padding:13px;color:var(--accent);font-size:11px;font-weight:700 }
.detail-area { min-width:0 }
.detail-empty { min-height:470px;display:grid;place-content:center;justify-items:center;gap:12px;text-align:center;background:radial-gradient(circle at 70% 20%,#e7f4ef,#fff 60%) }
.empty-mark { display:grid;place-items:center;width:76px;height:76px;border-radius:18px;background:#dff0e9;color:var(--accent) }
.detail-empty h2 { font-size:21px;color:var(--ink) }.detail-empty p { max-width:330px;color:var(--muted);font-size:12px }
.work-detail,.scrape-preview { padding:0;overflow:hidden }
.detail-header { display:grid;gap:8px;padding:18px 20px;border-bottom:1px solid var(--hairline);background:linear-gradient(110deg,#ecf6f1,#fff 75%) }
.back-button { display:none;align-items:center;gap:5px;justify-self:start;color:var(--accent);border:0;background:transparent;font-size:11px }
.detail-title h2 { font-size:21px;letter-spacing:-.035em;color:var(--ink);margin:4px 0 }
.detail-toolbar { display:flex;align-items:center;justify-content:space-between;gap:10px;padding:6px 14px;border-bottom:1px solid var(--hairline);flex-wrap:wrap }
.status-tabs { display:flex;align-items:center;gap:2px;flex-wrap:wrap }
.status-tabs button { min-height:32px;padding:5px 7px;border:0;border-radius:6px;background:transparent;color:#617571;font-size:11px }
.status-tabs button.active { background:var(--accent);color:#fff }
.status-tabs small { margin-left:3px;opacity:.7 }
.file-search { flex:1;max-width:250px }
.bulk-bar { display:flex;align-items:center;justify-content:space-between;gap:12px;padding:12px 18px;background:#f7faf8;border-bottom:1px solid var(--hairline);font-size:11px;color:#5f7370;flex-wrap:wrap }
.bulk-bar > div { display:flex;gap:8px;align-items:center;flex-wrap:wrap }
.bulk-bar .text-button,.season-actions .text-button,.episode-summary .text-button { color:var(--accent);font-size:11px;font-weight:700 }
.no-matches { padding:34px;text-align:center;color:var(--muted);font-size:12px }
.season-list { padding:12px 18px }
.season-section { margin-bottom:8px;border:1px solid var(--hairline);border-radius:9px;overflow:hidden;background:#fff }
.season-head { display:flex;align-items:center;gap:8px;background:#fcfdfc;padding:6px 12px }
.season-expand { flex:1;min-width:0;display:flex;align-items:center;gap:12px;text-align:left;border:0;background:transparent;padding:7px 0 }
.season-index { display:grid;place-items:center;flex:none;width:42px;height:42px;border-radius:7px;background:var(--accent-soft);color:var(--accent);font-size:12px;font-weight:750 }
.season-expand > span:nth-child(2) { display:grid;gap:2px;min-width:0 }
.season-expand strong { color:var(--ink);font-size:13px }.season-expand small { color:var(--muted);font-size:10px }
.expand-arrow { margin-left:auto;color:#90a6a0 }.season-expand[aria-expanded=true] .expand-arrow { transform:rotate(180deg) }
.season-actions { display:flex;gap:10px;flex-wrap:wrap }
.episode-list { border-top:1px solid var(--hairline);padding:5px 12px 12px }
.episode-row { display:block;min-height:0;padding:0;border-bottom:1px solid #e9f0ed }.episode-row:last-child { border:0 }
.episode-summary { display:flex;align-items:center;gap:8px;min-height:43px }
.episode-expand { flex:1;display:flex;align-items:center;gap:9px;text-align:left;border:0;background:transparent;color:#758b86;font-size:11px }
.episode-number { min-width:58px;width:auto;height:auto;background:transparent;color:#304842;font:inherit;font-weight:750 }
.episode-expand svg { margin-left:auto }.episode-expand[aria-expanded=true] svg { transform:rotate(180deg) }
.version-list { border-top:1px dashed var(--hairline);background:#f9fcfa }
.movie-versions { padding:10px 18px }
.video-row { display:flex;align-items:center;gap:11px;min-width:0;padding:10px 12px;border-bottom:1px solid #e9f0ed;cursor:pointer }
.single-episode { width:100%;min-height:48px;border-bottom:0 }
.single-episode .episode-number { flex:none;min-width:48px }
.video-row.selected { background:#e9f5f1 }.video-row.unavailable { opacity:.55;cursor:not-allowed }
.video-row input { flex:none;accent-color:var(--accent) }
.video-info { display:grid;gap:3px;flex:1;min-width:0 }
.video-info strong { overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--ink);font-size:11px }
.video-info small { display:flex;align-items:center;gap:4px;color:var(--muted);font-size:10px }
.status-pill { flex:none;padding:5px 7px;border-radius:5px;background:#f3efe1;color:#996c2e;font-size:10px;font-weight:700 }
.status-pill.failed { background:#ffeded;color:#bd6260 }.status-pill.completed { background:#e5f4ed;color:#3a846e }.status-pill.unavailable { background:#ececea;color:#868987 }
.selection-footer { display:flex;align-items:center;justify-content:space-between;gap:12px;padding:14px 18px;border-top:1px solid var(--hairline);color:var(--muted);font-size:11px;background:#fcfdfc }
.selection-footer strong { color:var(--accent);font-size:17px }
.scrape-preview { display:flex;flex-direction:column;max-height:calc(100vh - 130px);min-height:440px }
.preview-head { display:grid;gap:4px;padding:14px 20px;background:linear-gradient(100deg,#e9f5ef,#fff 75%) }
.preview-back { display:flex;align-items:center;gap:5px;justify-self:start;border:0;background:transparent;padding:0 0 8px;color:var(--accent);font-size:11px;font-weight:700 }
.preview-head h2 { color:var(--ink);font-size:18px }.preview-head p { color:var(--muted);font-size:11px }
.preview-body { min-height:0;overflow-y:auto;flex:1 }
.summary-grid { display:grid;grid-template-columns:repeat(4,1fr);gap:8px;padding:14px 18px }
.summary-grid div { display:grid;gap:3px;background:#f3f9f5;border:1px solid var(--hairline);border-radius:7px;padding:10px }
.summary-grid strong { color:var(--accent);font-size:18px }.summary-grid span { color:var(--muted);font-size:10px }
.preview-issues { display:flex;gap:8px;padding:12px 18px;color:var(--red);font-size:11px }
.artifact-details { margin:0 18px 16px;border:1px solid var(--hairline);border-radius:7px }
.artifact-details summary { display:flex;align-items:center;gap:8px;cursor:pointer;padding:12px;font-size:11px;font-weight:700;color:var(--ink);list-style:none }
.artifact-details summary span { margin-left:auto;color:var(--accent) }.artifact-details summary svg { color:var(--accent) }
.artifact-inner { padding:0 12px 12px }.artifact-inner .search-field { margin-bottom:8px }
.artifact-list { max-height:250px;overflow:auto }.artifact-row { display:flex;gap:8px;align-items:center;min-width:0;border-top:1px solid #edf1ee;padding:8px 2px;color:#60867b;font-size:11px }
.artifact-row .eli-text { flex:1;color:var(--ink) }
.preview-footer { display:flex;align-items:center;justify-content:space-between;gap:14px;padding:15px 18px;border-top:1px solid var(--hairline);background:#fff }
.overwrite-note { display:flex;gap:8px;max-width:430px;color:#947553;font-size:10px }.overwrite-note svg { flex:none }.overwrite-note strong { color:#78542e }
@media(max-width:900px) { .scrape-shell { grid-template-columns:minmax(210px,260px) minmax(0,1fr) } .season-head { flex-wrap:wrap } .season-actions { padding-left:54px } }
@media(max-width:700px) {
  .scrape-shell,.scrape-shell.has-detail { display:block }
  .scrape-shell.has-detail .catalog { display:none }
  .scrape-shell:not(.has-detail) .detail-area { display:none }
  .back-button { display:flex }
  .work-list { max-height:none }
  .scrape-preview { min-height:0;max-height:calc(100dvh - 170px) }
  .detail-toolbar,.selection-footer,.preview-footer { align-items:stretch;flex-direction:column }
  .status-tabs { flex-wrap:nowrap;overflow-x:auto;max-width:100%;padding-bottom:3px }
  .status-tabs button { flex:none }
  .file-search { max-width:none }
  .summary-grid { grid-template-columns:repeat(2,1fr) }
  .season-head { display:block }
  .season-actions { padding:0 0 8px 54px }
  .episode-summary { flex-wrap:wrap;gap:5px 10px;padding:9px 0 }
  .episode-expand { flex-basis:100%;min-height:28px }
  .episode-expand svg { margin-left:0 }
  .episode-summary .text-button { margin-left:58px }
  .episode-summary .text-button + .text-button { margin-left:0 }
  .video-row { padding:10px 2px }
  .preview-footer .button,.selection-footer .button { width:100% }
}
</style>
