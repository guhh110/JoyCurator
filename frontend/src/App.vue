<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  ArrowDownToLine,
  ArrowRight,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleCheck,
  Clock3,
  Copy,
  FileCheck2,
  Film,
  Folder,
  FolderOpen,
  History,
  Layers3,
  Link2,
  LoaderCircle,
  LogOut,
  Pencil,
  Plus,
  Search,
  Settings2,
  ShieldCheck,
  Trash2,
  Undo2,
  X,
} from '@lucide/vue'
import { api, bytes, episodeCode, name, parseEpisodes } from './api'
import type {
  Entry,
  Episode,
  MediaFile,
  Preview,
  Run,
  Season,
  Settings,
} from './api'
import Modal from './components/Modal.vue'
import FileBrowser from './components/FileBrowser.vue'

const view = ref('work'),
  sessionReady = ref(false),
  initialized = ref(true),
  username = ref(''),
  loginName = ref('admin'),
  password = ref('')
const busy = ref(false),
  message = ref(''),
  success = ref('')
const files = ref<MediaFile[]>([]),
  entry = ref<Entry | null>(null),
  season = ref(1),
  seasons = ref<Season[]>([]),
  episodes = ref<Episode[]>([]),
  loadingEpisodes = ref(false)
const filter = ref(''),
  filePage = ref(1),
  panel = ref('files'),
  mode = ref('copy'),
  browser = ref(false),
  directoryTarget = ref<'movieRoot' | 'tvRoot' | null>(null)
const searchOpen = ref(false),
  query = ref(''),
  kind = ref<'tv' | 'movie'>('tv'),
  year = ref(''),
  searching = ref(false),
  searched = ref(false),
  results = ref<Entry[]>([]),
  searchError = ref('')
const editing = ref<MediaFile | null>(null),
  editSeason = ref(1),
  editEpisodes = ref(''),
  editError = ref('')
const preview = ref<Preview | null>(null),
  previewFilter = ref(''),
  previewPage = ref(1),
  omitted = ref(new Set<string>())
const settings = ref<Settings | null>(null),
  token = ref(''),
  clearToken = ref(false),
  settingsSection = ref('folders')
const records = ref<Run[]>([]),
  recordPage = ref(1),
  recordTotal = ref(0),
  detail = ref<Run | null>(null),
  undoing = ref<Run | null>(null),
  active = ref<Run | null>(null)
let episodeRequest = 0,
  searchRequest = 0,
  timer: ReturnType<typeof setInterval> | undefined,
  polling = false
const modeNames: Record<string, string> = {
  copy: '复制',
  move: '移动',
  hardlink: '硬链接',
}
const kindNames: Record<string, string> = {
  video: '视频',
  subtitle: '字幕',
  nfo: 'NFO',
  image: '图片',
}
const states: Record<string, string> = {
  running: '进行中',
  completed: '已完成',
  failed: '部分失败',
  cancelled: '已取消',
  interrupted: '已中断',
  undone: '已撤销',
  pending: '等待中',
  published: '已写入',
  staged: '待发布',
  preparing: '处理中',
  undoing: '撤销中',
}
const filtered = computed(() =>
  files.value.filter((file) =>
    file.path.toLowerCase().includes(filter.value.toLowerCase()),
  ),
)
const visibleFiles = computed(() =>
  filtered.value.slice((filePage.value - 1) * 40, filePage.value * 40),
)
const filePages = computed(() =>
  Math.max(1, Math.ceil(filtered.value.length / 40)),
)
const running = computed(() => active.value?.state === 'running')
const mapping = (file: MediaFile) => ({
  ...file,
  season: file.season < 0 ? season.value : file.season,
})
const ready = computed(() =>
  !entry.value
    ? []
    : entry.value.type === 'movie'
      ? files.value
      : files.value.filter(
          (file) =>
            mapping(file).season === season.value &&
            file.episodes.length > 0 &&
            file.episodes.every((n) =>
              episodes.value.some((ep) => ep.number === n),
            ),
        ),
)
const unmatched = computed(() => files.value.length - ready.value.length)
const matchedCount = computed(
  () => episodes.value.filter((ep) => assigned(ep.number).length).length,
)
const previewRows = computed(() =>
  (preview.value?.rows ?? []).filter((row) =>
    `${row.source} ${row.target}`
      .toLowerCase()
      .includes(previewFilter.value.toLowerCase()),
  ),
)
const shownPreview = computed(() =>
  previewRows.value.slice((previewPage.value - 1) * 30, previewPage.value * 30),
)
const step = computed(() => (!files.value.length ? 1 : !entry.value ? 2 : 3))

watch(filter, () => (filePage.value = 1))
watch(filePages, (value) => (filePage.value = Math.min(filePage.value, value)))
watch(previewFilter, () => (previewPage.value = 1))
watch(entry, (value) => {
  if (username.value)
    try {
      localStorage.setItem('joy-rewrite-entry', JSON.stringify(value))
    } catch {}
})
watch(
  files,
  (value) => {
    if (username.value)
      try {
        localStorage.setItem('joy-rewrite-files', JSON.stringify(value))
      } catch {
        /* Work remains in memory if storage is full. */
      }
  },
  { deep: true },
)
watch(view, (value) => {
  message.value = ''
  if (value === 'records') void loadRecords()
  if (value === 'settings') void act(loadSettings)
})
watch([query, kind, year], () => {
  searchRequest++
  searched.value = false
  results.value = []
  searching.value = false
})

async function act<T>(fn: () => Promise<T>): Promise<T | undefined> {
  if (busy.value) return
  busy.value = true
  message.value = ''
  success.value = ''
  try {
    return await fn()
  } catch (e) {
    message.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
async function session() {
  try {
    const value = await api<{ initialized: boolean; username: string }>(
      '/session',
    )
    initialized.value = value.initialized
    username.value = value.username
    if (value.username) {
      restore()
      await loadSettings()
    }
  } catch (e) {
    message.value = (e as Error).message
  } finally {
    sessionReady.value = true
  }
}
function restore() {
  try {
    const value = JSON.parse(localStorage.getItem('joy-rewrite-files') || '[]')
    if (Array.isArray(value))
      files.value = value
        .filter(
          (file) =>
            typeof file.path === 'string' && Array.isArray(file.episodes),
        )
        .slice(0, 2000)
    const saved = JSON.parse(
      localStorage.getItem('joy-rewrite-entry') || 'null',
    )
    if (
      saved &&
      (saved.type === 'tv' || saved.type === 'movie') &&
      Number.isInteger(saved.tmdbId)
    )
      void pick(saved)
  } catch {}
}
async function login() {
  await act(async () => {
    const value = await api<{ username: string }>(
      initialized.value ? '/login' : '/setup',
      { username: loginName.value, password: password.value },
    )
    username.value = value.username
    initialized.value = true
    password.value = ''
    restore()
    await loadSettings()
  })
}
function expire() {
  username.value = ''
  files.value = []
  entry.value = null
  preview.value = null
  detail.value = null
  searchOpen.value = false
  browser.value = false
  active.value = null
  settings.value = null
  episodeRequest++
  searchRequest++
  localStorage.removeItem('joy-rewrite-files')
  localStorage.removeItem('joy-rewrite-entry')
  localStorage.removeItem('joy-rewrite-active')
}
async function logout() {
  await act(async () => {
    await api('/logout', {})
    expire()
  })
}
async function loadSettings() {
  settings.value = await api<Settings>('/settings')
}
async function add(paths: string[]) {
  browser.value = false
  await act(async () => {
    const added = await api<MediaFile[]>('/scan', { paths })
    const merged = new Map(files.value.map((file) => [file.path, file]))
    for (const file of added) merged.set(file.path, file)
    if (merged.size > 2000)
      throw new Error('工作区最多 2000 个文件，请分批整理')
    files.value = [...merged.values()].sort((a, b) =>
      a.path.localeCompare(b.path, 'zh-CN', { numeric: true }),
    )
    success.value = `已添加 ${added.length} 个视频`
    omitted.value.clear()
  })
}
function remove(file: MediaFile) {
  files.value = files.value.filter((item) => item.path !== file.path)
  omitted.value.clear()
}
function assigned(n: number) {
  return files.value.filter(
    (file) =>
      mapping(file).season === season.value && file.episodes.includes(n),
  )
}
async function search() {
  const request = ++searchRequest
  searching.value = true
  searched.value = true
  searchError.value = ''
  results.value = []
  try {
    const value = await api<Entry[]>(
      `/search?type=${kind.value}&q=${encodeURIComponent(query.value.trim())}&year=${encodeURIComponent(year.value)}`,
    )
    if (request === searchRequest) results.value = value
  } catch (e) {
    if (request === searchRequest) searchError.value = (e as Error).message
  } finally {
    if (request === searchRequest) searching.value = false
  }
}
async function pick(value: Entry) {
  const request = ++episodeRequest
  entry.value = value
  searchOpen.value = false
  episodes.value = []
  seasons.value = []
  omitted.value.clear()
  loadingEpisodes.value = false
  if (value.type === 'movie') return
  loadingEpisodes.value = true
  try {
    const list = await api<Season[]>(`/seasons?id=${value.tmdbId}`)
    if (request !== episodeRequest) return
    seasons.value = list
    const known = files.value.find(
      (file) => file.season >= 0 && list.some((s) => s.number === file.season),
    )
    season.value =
      known?.season ??
      list.find((s) => s.number > 0 && s.count > 0)?.number ??
      list[0]?.number ??
      1
    await changeSeason(season.value)
  } catch (e) {
    message.value = (e as Error).message
  } finally {
    if (request === episodeRequest) loadingEpisodes.value = false
  }
}
async function changeSeason(n: number) {
  season.value = n
  episodes.value = []
  omitted.value.clear()
  const request = ++episodeRequest
  loadingEpisodes.value = true
  message.value = ''
  try {
    const value = await api<Episode[]>(
      `/episodes?id=${entry.value?.tmdbId}&season=${n}`,
    )
    if (request === episodeRequest) episodes.value = value
  } catch (e) {
    if (request === episodeRequest) message.value = (e as Error).message
  } finally {
    if (request === episodeRequest) loadingEpisodes.value = false
  }
}
function edit(file: MediaFile) {
  editing.value = file
  editSeason.value = mapping(file).season
  editEpisodes.value = file.episodes.join(',')
  editError.value = ''
}
function saveEdit() {
  try {
    if (
      !Number.isInteger(editSeason.value) ||
      editSeason.value < 0 ||
      editSeason.value > 999
    )
      throw new Error('季号为 0–999，0 表示特别篇')
    const values = parseEpisodes(editEpisodes.value)
    if (editing.value)
      files.value = files.value.map((file) =>
        file.path === editing.value!.path
          ? { ...file, season: editSeason.value, episodes: values }
          : file,
      )
    editing.value = null
    omitted.value.clear()
  } catch (e) {
    editError.value = (e as Error).message
  }
}
async function prepare() {
  await act(async () => {
    const chosen = ready.value.filter((file) => !omitted.value.has(file.path))
    if (!chosen.length) throw new Error('没有可整理的文件')
    preview.value = await api<Preview>('/preview', {
      type: entry.value?.type,
      tmdbId: entry.value?.tmdbId,
      mode: mode.value,
      files: chosen.map(mapping),
    })
    previewPage.value = 1
    previewFilter.value = ''
  })
}
async function skip(path: string) {
  omitted.value = new Set([...omitted.value, path])
  await prepare()
}
async function execute() {
  if (!preview.value) return
  await act(async () => {
    active.value = await api<Run>('/execute', { id: preview.value!.id })
    localStorage.setItem('joy-rewrite-active', active.value.id)
    preview.value = null
    success.value = '正在整理，进度会自动更新'
  })
}
async function loadRecords() {
  try {
    const result = await api<{ items: Run[]; total: number }>(
      `/runs?page=${recordPage.value}`,
    )
    records.value = result.items
    recordTotal.value = result.total
  } catch (e) {
    message.value = (e as Error).message
  }
}
async function inspect(run: Run) {
  await act(async () => (detail.value = await api<Run>(`/runs/${run.id}`)))
}
async function recordAction(run: Run, action: string) {
  undoing.value = null
  await act(async () => {
    if (action === 'cancel') {
      await api(`/runs/${run.id}/cancel`, {})
      return
    }
    active.value = await api<Run>(`/runs/${run.id}/${action}`, {})
    localStorage.setItem('joy-rewrite-active', active.value.id)
    detail.value = null
    await loadRecords()
  })
}
async function poll() {
  if (!username.value || polling) return
  const id =
    active.value?.state === 'running'
      ? active.value.id
      : localStorage.getItem('joy-rewrite-active')
  if (!id) return
  polling = true
  try {
    const value = await api<Run>(`/runs/${id}`)
    active.value = value
    if (value.state !== 'running') {
      localStorage.removeItem('joy-rewrite-active')
      if (!value.undo) {
        const done = new Set(
          value.ops
            ?.filter((op) => op.kind === 'video' && op.status === 'completed')
            .map((op) => op.source),
        )
        files.value = files.value.filter((file) => !done.has(file.path))
      }
      if (view.value === 'records') await loadRecords()
    }
  } catch (e) {
    message.value = (e as Error).message
    localStorage.removeItem('joy-rewrite-active')
  } finally {
    polling = false
  }
}
async function saveSettings() {
  if (!settings.value) return
  await act(async () => {
    const { hasToken: _, roots: __, ...value } = settings.value!
    value.rules.categories = value.rules.categories.map((category) => ({
      ...category,
      yearMin: Number(category.yearMin) || 0,
      yearMax: Number(category.yearMax) || 0,
    }))
    await api(
      '/settings',
      {
        ...value,
        ...(clearToken.value
          ? { tmdbToken: '' }
          : token.value.trim()
            ? { tmdbToken: token.value.trim() }
            : {}),
      },
      'PUT',
    )
    token.value = ''
    clearToken.value = false
    await loadSettings()
    success.value = '设置已保存'
  })
}
function chooseDirectory(paths: string[]) {
  if (settings.value && directoryTarget.value)
    settings.value[directoryTarget.value] = paths[0] || ''
  directoryTarget.value = null
}
function newPreview() {
  omitted.value.clear()
  void prepare()
}
function changeRecordPage(delta: number) {
  recordPage.value += delta
  void loadRecords()
}
function parts(event: Event) {
  return (event.target as HTMLInputElement).value
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
}
function date(value: string) {
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
onMounted(() => {
  void session()
  timer = setInterval(poll, 1200)
  window.addEventListener('session-expired', expire)
})
onUnmounted(() => {
  clearInterval(timer)
  window.removeEventListener('session-expired', expire)
  episodeRequest++
  searchRequest++
})
</script>

<template>
  <div v-if="!sessionReady" class="boot">
    <LoaderCircle class="spin" />正在打开工作台
  </div>
  <div v-else-if="!username" class="login-screen">
    <div class="login-art">
      <div class="brand light">
        <div class="brand-symbol"><Layers3 :size="25" /></div>
        JoyCurator<span class="edition">轻量版</span>
      </div>
      <div class="login-copy">
        <span class="eyebrow">A PLACE FOR EVERY STORY</span>
        <h1>好故事，<br />也值得好好收藏。</h1>
        <p>让散落的媒体文件，<br />成为井然有序的私人片库。</p>
        <div class="art-shelves">
          <div><Film /><span>电影</span><i>FILMS</i></div>
          <div><Layers3 /><span>剧集</span><i>SERIES</i></div>
          <div><Folder /><span>你的收藏</span><i>COLLECTION</i></div>
        </div>
      </div>
      <span class="art-caption">本地文件 · 手动配对 · 先预览再整理</span>
    </div>
    <main class="login-form">
      <div class="mobile-brand brand"><Layers3 />JoyCurator</div>
      <span class="eyebrow">WELCOME TO YOUR COLLECTION</span>
      <h2>{{ initialized ? '欢迎回来' : '创建你的工作台' }}</h2>
      <p class="muted">
        {{
          initialized
            ? '登录，继续整理你的收藏。'
            : '设置一个管理员账号，开始整理本地媒体。'
        }}
      </p>
      <form @submit.prevent="login">
        <label
          >用户名<input
            v-model="loginName"
            autocomplete="username"
            required
            maxlength="64" /></label
        ><label
          >密码<input
            v-model="password"
            type="password"
            :autocomplete="initialized ? 'current-password' : 'new-password'"
            :minlength="initialized ? 1 : 12"
            maxlength="72"
            required
            :placeholder="initialized ? '输入密码' : '至少 12 个字符'"
        /></label>
        <p v-if="message" role="alert" class="notice error">{{ message }}</p>
        <button class="button primary full" :disabled="busy">
          <LoaderCircle v-if="busy" class="spin" :size="17" />{{
            initialized ? '进入工作台' : '创建并进入'
          }}<ArrowRight :size="17" />
        </button>
      </form>
      <p class="login-note">
        <ShieldCheck :size="15" />文件与凭据保存在你的设备上
      </p>
    </main>
  </div>
  <div v-else class="app-shell">
    <header class="topbar">
      <a class="brand" href="#" @click.prevent="view = 'work'"
        ><div class="brand-symbol"><Layers3 :size="23" /></div>
        JoyCurator<span class="edition">轻量版</span></a
      >
      <nav aria-label="主导航">
        <button :class="{ active: view === 'work' }" @click="view = 'work'">
          <FolderOpen :size="17" />整理工作台</button
        ><button
          :class="{ active: view === 'records' }"
          @click="view = 'records'"
        >
          <History :size="17" />整理记录</button
        ><button
          :class="{ active: view === 'settings' }"
          @click="view = 'settings'"
        >
          <Settings2 :size="17" />设置
        </button>
      </nav>
      <div class="account">
        <span class="status-dot" /><span>{{ username }}</span
        ><button class="icon-button" aria-label="退出登录" @click="logout">
          <LogOut :size="16" />
        </button>
      </div>
    </header>
    <main :class="['main', { workspace: view === 'work' }]">
      <div v-if="message" class="notice error global-notice" role="alert">
        <span>{{ message }}</span
        ><button
          class="icon-button"
          aria-label="关闭错误提示"
          @click="message = ''"
        >
          <X :size="16" />
        </button>
      </div>
      <div v-if="success" class="notice good global-notice" role="status">
        <span>{{ success }}</span
        ><button
          class="icon-button"
          aria-label="关闭提示"
          @click="success = ''"
        >
          <X :size="16" />
        </button>
      </div>
      <template v-if="view === 'work'">
        <div class="workspace-heading">
          <div>
            <span class="eyebrow">YOUR MEDIA, BEAUTIFULLY ORDERED</span>
            <h1>整理工作台<span class="heading-dot">.</span></h1>
          </div>
          <div class="steps" aria-label="整理步骤">
            <span :class="{ current: step === 1, done: step > 1 }"
              ><i>{{ step > 1 ? '✓' : '1' }}</i
              >添加文件</span
            ><b /><span :class="{ current: step === 2, done: step > 2 }"
              ><i>{{ step > 2 ? '✓' : '2' }}</i
              >选择作品</span
            ><b /><span :class="{ current: step === 3 }"><i>3</i>预览整理</span>
          </div>
        </div>
        <div class="mobile-switch">
          <button
            :class="{ active: panel === 'files' }"
            @click="panel = 'files'"
          >
            源文件 <span>{{ files.length }}</span></button
          ><button
            :class="{ active: panel === 'episodes' }"
            @click="panel = 'episodes'"
          >
            {{ entry?.type === 'movie' ? '电影信息' : '作品与剧集' }}
          </button>
        </div>
        <div class="work-grid">
          <section
            class="pane files-pane"
            :class="{ 'mobile-hidden': panel !== 'files' }"
            aria-label="源文件列表"
          >
            <header class="pane-head">
              <div class="pane-label">
                <FolderOpen :size="17" />
                <h2>源文件</h2>
                <span class="count">{{ files.length }}</span>
              </div>
              <button
                class="button small"
                :disabled="busy || running"
                @click="browser = true"
              >
                <Plus :size="15" />添加文件
              </button>
            </header>
            <div v-if="files.length" class="file-toolbar">
              <div class="search-input">
                <Search :size="15" /><input
                  v-model="filter"
                  placeholder="筛选文件名或路径"
                  aria-label="筛选源文件"
                />
              </div>
              <span class="muted tiny">{{ filtered.length }} 个结果</span>
            </div>
            <div class="pane-scroll file-list">
              <div v-if="!files.length" class="empty files-empty">
                <div class="empty-illustration">
                  <div />
                  <div />
                  <span
                    ><FolderOpen :size="35" stroke-width="1.4" /><i
                      ><Plus :size="15" /></i
                  ></span>
                </div>
                <h3>从一份收藏开始</h3>
                <p>
                  添加一个视频，或整个文件夹。<br />原文件会在你确认执行前保持原样。
                </p>
                <button
                  class="button primary"
                  :disabled="busy"
                  @click="browser = true"
                >
                  <Plus :size="16" />添加文件或文件夹</button
                ><small>支持 MKV、MP4、AVI 等常见视频格式</small>
              </div>
              <div v-else-if="!filtered.length" class="empty">
                <Search :size="28" />
                <h3>没有找到文件</h3>
                <button class="text-button" @click="filter = ''">
                  清空筛选
                </button>
              </div>
              <article
                v-for="(file, index) in visibleFiles"
                :key="file.path"
                class="file-row"
              >
                <span class="file-index">{{
                  String((filePage - 1) * 40 + index + 1).padStart(2, '0')
                }}</span>
                <div class="file-content">
                  <p class="file-name" :title="file.path" tabindex="0">
                    {{ name(file.path) }}
                  </p>
                  <div class="file-meta">
                    <span
                      v-if="entry?.type === 'tv' && file.episodes.length"
                      class="episode-chip"
                      :class="{ quiet: mapping(file).season !== season }"
                      >{{
                        episodeCode(mapping(file).season, file.episodes)
                      }}</span
                    ><span v-else-if="entry?.type === 'tv'" class="needs-edit"
                      >待填写集数</span
                    ><span>{{ bytes(file.size) }}</span
                    ><span v-if="file.probe.resolution">{{
                      file.probe.resolution
                    }}</span
                    ><span
                      v-if="file.probe.warning"
                      :title="file.probe.warning"
                      class="muted"
                      >未探测</span
                    >
                  </div>
                </div>
                <div class="row-actions">
                  <button
                    v-if="entry?.type === 'tv'"
                    class="icon-button"
                    :disabled="busy || running"
                    :aria-label="`编辑季集 ${name(file.path)}`"
                    title="修正季集"
                    @click="edit(file)"
                  >
                    <Pencil :size="15" /></button
                  ><button
                    class="icon-button danger-hover"
                    :disabled="busy || running"
                    :aria-label="`移除 ${name(file.path)}`"
                    title="从工作区移除"
                    @click="remove(file)"
                  >
                    <X :size="16" />
                  </button>
                </div>
              </article>
            </div>
            <footer v-if="files.length" class="pane-foot">
              <span
                >{{ files.length }} 个视频 ·
                {{ bytes(files.reduce((n, file) => n + file.size, 0)) }}</span
              >
              <div v-if="filePages > 1" class="pagination">
                <button
                  class="icon-button"
                  aria-label="上一页文件"
                  :disabled="filePage === 1"
                  @click="filePage--"
                >
                  <ChevronLeft :size="16" /></button
                >{{ filePage }} / {{ filePages
                }}<button
                  class="icon-button"
                  aria-label="下一页文件"
                  :disabled="filePage === filePages"
                  @click="filePage++"
                >
                  <ChevronRight :size="16" />
                </button>
              </div>
            </footer>
          </section>
          <section
            class="pane match-pane"
            :class="{ 'mobile-hidden': panel !== 'episodes' }"
            aria-label="作品与剧集对照"
          >
            <header class="pane-head">
              <div class="pane-label">
                <div v-if="entry?.posterPath" class="poster header-poster">
                  <img
                    :src="`https://image.tmdb.org/t/p/w185${entry.posterPath}`"
                    alt="作品海报"
                  />
                </div>
                <Film v-else :size="17" />
                <h2 :title="entry?.title">{{ entry?.title || '配对作品' }}</h2>
                <span class="mini-label">{{
                  entry
                    ? `${entry.year || ''} · ${entry.type === 'tv' ? '剧集' : '电影'}`
                    : 'TMDB'
                }}</span>
              </div>
              <button
                v-if="entry"
                class="text-button"
                :disabled="busy || running"
                @click="searchOpen = true"
              >
                更换作品<ChevronRight :size="14" />
              </button>
            </header>
            <template v-if="entry">
              <div v-if="entry.type === 'tv'" class="season-bar">
                <label class="season-select"
                  ><Layers3 :size="15" /><select
                    :value="season"
                    aria-label="选择季"
                    :disabled="loadingEpisodes || running"
                    @change="
                      changeSeason(
                        Number(($event.target as HTMLSelectElement).value),
                      )
                    "
                  >
                    <option
                      v-for="s in seasons"
                      :key="s.number"
                      :value="s.number"
                    >
                      {{ s.number === 0 ? '特别篇' : `第 ${s.number} 季` }} ·
                      {{ s.count }} 集
                    </option></select
                  ><ChevronDown :size="13" /></label
                ><span class="tiny muted"
                  >已配对 <b class="purple">{{ matchedCount }}</b> /
                  {{ episodes.length }} 集</span
                >
              </div>
              <div class="pane-scroll episode-list">
                <div v-if="loadingEpisodes" class="empty">
                  <LoaderCircle class="spin" />正在获取剧集…
                </div>
                <template v-else-if="entry.type === 'tv'"
                  ><article
                    v-for="ep in episodes"
                    :key="ep.number"
                    class="episode-row"
                    :class="{ matched: assigned(ep.number).length }"
                  >
                    <span class="episode-number">{{
                      String(ep.number).padStart(2, '0')
                    }}</span>
                    <div>
                      <h4>{{ ep.name || `第 ${ep.number} 集` }}</h4>
                      <p
                        v-if="assigned(ep.number).length"
                        :title="
                          assigned(ep.number)
                            .map((file) => file.path)
                            .join('\n')
                        "
                      >
                        <Link2 :size="11" />{{
                          assigned(ep.number)
                            .map((file) => name(file.path))
                            .join(' · ')
                        }}
                      </p>
                      <p v-else>
                        {{ ep.airDate || '暂无播出日期' }}<span>·</span>等待文件
                      </p>
                    </div>
                    <CircleCheck
                      v-if="assigned(ep.number).length"
                      :size="17"
                      class="match-check"
                      aria-label="已匹配"
                    /><span v-else class="unmatched-dot" role="img" aria-label="未匹配" />
                  </article>
                  <div v-if="!episodes.length" class="empty">
                    <p>当前季暂无集列表</p>
                    <button class="button small" @click="changeSeason(season)">
                      重新获取
                    </button>
                  </div></template
                >
                <div v-else class="movie-details">
                  <span class="eyebrow">ABOUT THE FILM</span>
                  <p>{{ entry.overview || '这部影片暂无简介。' }}</p>
                  <div class="tip">
                    <Copy :size="17" /><span
                      >每个视频作为独立版本整理。<br />多个版本会保留，冲突在预览中处理。</span
                    >
                  </div>
                </div>
              </div>
              <footer class="pane-foot">
                <span class="legend-dot" /><span>{{
                  entry.type === 'tv'
                    ? '集号来自文件名，可手动修正'
                    : '电影无需填写季集'
                }}</span>
              </footer>
            </template>
            <div v-else class="empty match-empty">
              <div class="match-art">
                <Film :size="34" stroke-width="1.3" /><span
                  ><Search :size="17"
                /></span>
              </div>
              <h3>给文件找到它的故事</h3>
              <p>
                搜索 TMDB，选择正确的电影或剧集。<br />我们负责把文件名与季集对上。
              </p>
              <button class="button" @click="searchOpen = true">
                <Search :size="16" />搜索作品
              </button>
              <div class="empty-note">
                <ShieldCheck :size="13" />由你确认，不自动猜测作品
              </div>
            </div>
          </section>
        </div>
        <footer class="organize-bar">
          <div class="organize-summary">
            <div class="ready-icon"><FileCheck2 :size="20" /></div>
            <div>
              <strong>{{
                ready.length
                  ? `${ready.length} 个文件已就绪`
                  : '准备好你的下一份收藏'
              }}</strong>
              <p>
                {{
                  ready.length
                    ? unmatched
                      ? `${unmatched} 个文件不在当前配对范围内`
                      : '先预览完整路径，再确认执行'
                    : !files.length
                      ? '添加文件，开启整理'
                      : !entry
                        ? '选择作品后继续'
                        : '修正季集，或切换到文件对应的季'
                }}
              </p>
            </div>
          </div>
          <div class="organize-actions">
            <select
              v-model="mode"
              aria-label="整理方式"
              :disabled="busy || running"
            >
              <option value="copy">复制文件</option>
              <option value="move">移动文件</option>
              <option value="hardlink">创建硬链接</option></select
            ><button
              class="button primary"
              :disabled="busy || running || loadingEpisodes || !ready.length"
              @click="newPreview"
            >
              <LoaderCircle v-if="busy" class="spin" :size="16" /><span>{{
                busy ? '正在生成…' : '预览整理'
              }}</span
              ><ArrowRight :size="16" />
            </button>
          </div>
        </footer>
      </template>
      <template v-else-if="view === 'records'"
        ><div class="page-heading">
          <div>
            <span class="eyebrow">COLLECTION JOURNAL</span>
            <h1>每一次整理，都有迹可循。</h1>
            <p class="muted">查看结果、恢复中断，或把文件放回原处。</p>
          </div>
          <button class="button small" @click="loadRecords">刷新记录</button>
        </div>
        <div v-if="!records.length" class="pane empty record-empty">
          <History :size="38" stroke-width="1.2" />
          <h3>还没有整理记录</h3>
          <p>完成第一批整理后，结果会保存在这里。</p>
          <button class="button" @click="view = 'work'">
            前往工作台<ArrowRight :size="15" />
          </button>
        </div>
        <div v-else class="records-list">
          <article v-for="run in records" :key="run.id" class="record-card">
            <span class="record-symbol"
              ><Undo2 v-if="run.state === 'undone'" :size="21" /><Folder
                v-else
                :size="21" /></span
            ><button class="record-main" @click="inspect(run)">
              <h3>{{ run.title }}</h3>
              <p>
                {{ date(run.created) }}<span>·</span>{{ modeNames[run.mode]
                }}<span>·</span>{{ run.completed }}/{{ run.total }} 项
              </p></button
            ><span class="state-pill" :class="run.state">{{
              run.undo && run.state === 'running' ? '撤销中' : states[run.state]
            }}</span
            ><button
              class="icon-button"
              aria-label="查看整理详情"
              @click="inspect(run)"
            >
              <ChevronRight :size="18" />
            </button>
          </article>
        </div>
        <div v-if="recordTotal > 20" class="records-pagination">
          <button
            class="button small"
            :disabled="recordPage === 1"
            @click="changeRecordPage(-1)"
          >
            上一页</button
          >{{ recordPage }} / {{ Math.ceil(recordTotal / 20)
          }}<button
            class="button small"
            :disabled="recordPage * 20 >= recordTotal"
            @click="changeRecordPage(1)"
          >
            下一页
          </button>
        </div></template
      >
      <template v-else-if="settings"
        ><div class="page-heading">
          <div>
            <span class="eyebrow">MAKE IT YOURS</span>
            <h1>按照你的习惯整理。</h1>
            <p class="muted">一次配置，之后专注于你的收藏。</p>
          </div>
          <button
            class="button primary"
            :disabled="busy || running"
            @click="saveSettings"
          >
            <Check :size="16" />保存设置
          </button>
        </div>
        <div class="settings-layout">
          <nav class="settings-nav" aria-label="设置分类">
            <button
              :class="{ active: settingsSection === 'folders' }"
              @click="settingsSection = 'folders'"
            >
              <Folder :size="17" />媒体目录</button
            ><button
              :class="{ active: settingsSection === 'tmdb' }"
              @click="settingsSection = 'tmdb'"
            >
              <Film :size="17" />TMDB 连接</button
            ><button
              :class="{ active: settingsSection === 'rules' }"
              @click="settingsSection = 'rules'"
            >
              <Layers3 :size="17" />命名与分类</button
            ><button
              :class="{ active: settingsSection === 'output' }"
              @click="settingsSection = 'output'"
            >
              <ArrowDownToLine :size="17" />刮削产物
            </button>
          </nav>
          <section class="pane settings-content">
            <template v-if="settingsSection === 'folders'"
              ><h2>让每份收藏各归其位</h2>
              <p class="muted">
                目录是 Go 服务所在设备上的路径。目标根目录需预先存在。
              </p>
              <label
                >电影目标目录
                <div class="input-action">
                  <input v-model="settings.movieRoot" /><button
                    class="button small"
                    @click="directoryTarget = 'movieRoot'"
                  >
                    <FolderOpen :size="16" />选择
                  </button>
                </div></label
              ><label
                >剧集目标目录
                <div class="input-action">
                  <input v-model="settings.tvRoot" /><button
                    class="button small"
                    @click="directoryTarget = 'tvRoot'"
                  >
                    <FolderOpen :size="16" />选择
                  </button>
                </div></label
              >
              <div class="tip">
                <ShieldCheck :size="18" />
                <div>
                  已授权媒体根目录<code
                    v-for="root in settings.roots"
                    :key="root"
                    >{{ root }}</code
                  ><small>可通过 JOYCURATOR_MEDIA_ROOTS 添加授权目录。</small>
                </div>
              </div></template
            >
            <template v-else-if="settingsSection === 'tmdb'"
              ><h2>连接你的电影资料库</h2>
              <p class="muted">
                使用 TMDB 的 API Read Access Token，凭据只保存在后端。
              </p>
              <label
                >Read Access Token<span
                  v-if="settings.hasToken"
                  class="inline-good"
                  >已保存</span
                ><input
                  v-model="token"
                  type="password"
                  autocomplete="off"
                  :placeholder="
                    settings.hasToken ? '留空保留已保存凭据' : '粘贴 TMDB Token'
                  " /></label
              ><label v-if="settings.hasToken" class="checkbox-label"
                ><input v-model="clearToken" type="checkbox" />清除已保存的
                Token</label
              ><label
                >代理地址 <span class="muted">可选</span
                ><input
                  v-model="settings.proxyURL"
                  placeholder="http://127.0.0.1:7890" /></label
              ><label
                >元数据语言<select v-model="settings.language">
                  <option value="zh-CN">简体中文</option>
                  <option value="zh-TW">繁體中文</option>
                  <option value="en-US">English</option>
                  <option value="ja-JP">日本語</option>
                </select></label
              ></template
            >
            <template v-else-if="settingsSection === 'rules'"
              ><h2>熟悉的名字，整齐的目录</h2>
              <p class="muted">
                分类按顺序取首条命中的规则。模板缺少字段会在预览中提示。
              </p>
              <label
                >电影目录模板<input
                  v-model="settings.rules.movieFolder" /></label
              ><label
                >电影文件名<input v-model="settings.rules.movieFile" /></label
              ><label
                >剧集目录模板<input v-model="settings.rules.tvFolder" /></label
              ><label
                >剧集文件名<input v-model="settings.rules.tvFile"
              /></label>
              <p class="template-fields">
                可用字段：<code>{title}</code><code>{year}</code
                ><code>{category}</code><code>{tmdb_id}</code
                ><code>{season:02}</code><code>{episode_range}</code
                ><code>{resolution}</code><code>{codec}</code
                ><code>{original_name}</code><code>{ext}</code>
              </p>
              <div class="section-heading">
                <h3>分类规则</h3>
                <button
                  class="button small"
                  @click="
                    settings.rules.categories.push({
                      name: '新分类',
                      type: '',
                      countries: [],
                      genres: [],
                    })
                  "
                >
                  <Plus :size="14" />添加规则
                </button>
              </div>
              <details
                v-for="(category, index) in settings.rules.categories"
                :key="index"
                class="category-rule"
              >
                <summary>
                  <span class="rule-index">{{
                    String(index + 1).padStart(2, '0')
                  }}</span
                  >{{ category.name || '未命名分类' }}<ChevronDown :size="15" />
                </summary>
                <div class="category-fields">
                  <label>目录名称<input v-model="category.name" /></label
                  ><label
                    >媒体类型<select v-model="category.type">
                      <option value="">全部</option>
                      <option value="movie">电影</option>
                      <option value="tv">剧集</option>
                    </select></label
                  ><label
                    >国家/地区代码<input
                      :value="category.countries.join(',')"
                      placeholder="CN,HK,TW"
                      @change="category.countries = parts($event)" /></label
                  ><label
                    >类型标签<input
                      :value="category.genres.join(',')"
                      placeholder="动画,Animation"
                      @change="category.genres = parts($event)" /></label
                  ><label
                    >最早年份<input
                      v-model.number="category.yearMin"
                      type="number"
                      placeholder="0 为不限" /></label
                  ><label
                    >最晚年份<input
                      v-model.number="category.yearMax"
                      type="number"
                      placeholder="0 为不限"
                  /></label>
                  <div class="category-actions">
                    <button
                      class="button small"
                      :disabled="index === 0"
                      @click="
                        [
                          settings.rules.categories[index - 1],
                          settings.rules.categories[index],
                        ] = [
                          settings.rules.categories[index]!,
                          settings.rules.categories[index - 1]!,
                        ]
                      "
                    >
                      上移</button
                    ><button
                      class="button small danger"
                      @click="settings.rules.categories.splice(index, 1)"
                    >
                      <Trash2 :size="13" />移除
                    </button>
                  </div>
                </div>
              </details></template
            >
            <template v-else
              ><h2>补全收藏的每一个细节</h2>
              <p class="muted">
                所有关联产物都会列入预览。已有 NFO、海报和背景图会保留。
              </p>
              <label class="output-choice"
                ><span
                  ><strong>生成 NFO 元数据</strong
                  ><small
                    >影片、剧集及单集的信息文件，供媒体库读取。</small
                  ></span
                ><input
                  v-model="settings.nfo"
                  type="checkbox"
                  role="switch" /></label
              ><label class="output-choice"
                ><span
                  ><strong>下载海报与背景图</strong
                  ><small>从 TMDB 下载 poster.jpg、fanart.jpg。</small></span
                ><input v-model="settings.images" type="checkbox" role="switch"
              /></label>
              <div class="tip">
                <Copy :size="18" />
                <p>
                  同名字幕自动关联，无需额外开关。<br />支持 SRT、ASS、SSA、VTT
                  及成对的 IDX/SUB。
                </p>
              </div></template
            >
          </section>
        </div>
        <div class="mobile-save">
          <button
            class="button primary full"
            :disabled="busy || running"
            @click="saveSettings"
          >
            保存设置<Check :size="16" />
          </button></div
      ></template>
    </main>
    <div v-if="active" class="task-toast" :class="{ finished: !running }">
      <LoaderCircle v-if="running" class="spin purple" :size="19" /><CircleCheck
        v-else-if="active.state === 'completed' || active.state === 'undone'"
        :size="19"
        class="green"
      /><Clock3 v-else :size="19" />
      <div>
        <strong
          >{{ active.undo ? '撤销' : '整理' }}{{ states[active.state] }}</strong
        ><span
          >{{ active.title }} · {{ active.completed }}/{{
            active.total
          }}
          项</span
        >
      </div>
      <button
        v-if="running"
        class="text-button"
        @click="recordAction(active, 'cancel')"
      >
        取消</button
      ><button v-else class="text-button" @click="inspect(active)">查看</button
      ><button
        v-if="!running"
        class="icon-button"
        aria-label="关闭任务提示"
        @click="active = null"
      >
        <X :size="15" />
      </button>
    </div>
    <nav class="bottom-nav" aria-label="手机主导航">
      <button :class="{ active: view === 'work' }" @click="view = 'work'">
        <FolderOpen :size="20" />工作台</button
      ><button
        :class="{ active: view === 'records' }"
        @click="view = 'records'"
      >
        <History :size="20" />记录</button
      ><button
        :class="{ active: view === 'settings' }"
        @click="view = 'settings'"
      >
        <Settings2 :size="20" />设置
      </button>
    </nav>
  </div>

  <FileBrowser v-if="browser" @close="browser = false" @select="add" />
  <FileBrowser
    v-if="directoryTarget"
    directory
    @close="directoryTarget = null"
    @select="chooseDirectory"
  />
  <Modal v-if="searchOpen" title="找到要整理的作品" @close="searchOpen = false"
    ><p class="muted">选择正确的年份与类型，确认这是你的那部收藏。</p>
    <form class="tmdb-search" @submit.prevent="search">
      <select v-model="kind" aria-label="作品类型">
        <option value="tv">剧集</option>
        <option value="movie">电影</option>
      </select>
      <div class="search-input">
        <Search :size="16" /><input
          v-model="query"
          aria-label="搜索片名"
          placeholder="搜索电影或剧集名称"
          autofocus
          required
        />
      </div>
      <input
        v-model="year"
        aria-label="年份"
        placeholder="年份"
        inputmode="numeric"
        maxlength="4"
      /><button class="button primary" :disabled="searching || !query.trim()">
        <LoaderCircle v-if="searching" class="spin" :size="16" /><Search
          v-else
          :size="16"
        /><span>搜索</span>
      </button>
    </form>
    <p v-if="searchError" class="notice error" role="alert">
      {{ searchError }}
    </p>
    <div class="search-results">
      <button
        v-for="result in results"
        :key="`${result.type}-${result.tmdbId}`"
        class="search-result"
        @click="pick(result)"
      >
        <div class="result-poster">
          <img
            v-if="result.posterPath"
            :src="`https://image.tmdb.org/t/p/w185${result.posterPath}`"
            alt=""
          /><Film v-else :size="22" />
        </div>
        <div>
          <h3>
            {{ result.title }}<span>{{ result.year }}</span>
          </h3>
          <p class="tiny muted">
            {{ result.type === 'tv' ? '剧集' : '电影' }} · TMDB
            {{ result.tmdbId }}
          </p>
          <p class="result-overview">{{ result.overview || '暂无简介' }}</p>
        </div>
        <ChevronRight :size="18" />
      </button>
      <div v-if="!results.length && !searching" class="empty small-empty">
        <Search :size="28" />
        <p>
          {{
            searched
              ? '没有找到结果，试试别的名称或年份。'
              : '好故事，从一个名字开始。'
          }}
        </p>
      </div>
    </div></Modal
  >
  <Modal v-if="editing" title="修正季集" @close="editing = null"
    ><p class="editing-name" :title="editing.path">{{ name(editing.path) }}</p>
    <div class="edit-fields">
      <label
        >季号<input
          v-model.number="editSeason"
          type="number"
          min="0"
          max="999"
        /><small>0 表示特别篇</small></label
      ><label
        >集数<input
          v-model="editEpisodes"
          placeholder="例如 1、1-3 或 1,3,5"
        /><small>支持合并集，不修改原文件名</small></label
      >
    </div>
    <p v-if="editError" role="alert" class="notice error">{{ editError }}</p>
    <template #footer
      ><span class="muted tiny">映射只用于本次整理</span
      ><button class="button primary" @click="saveEdit">
        <Check :size="16" />保存修正
      </button></template
    ></Modal
  >
  <Modal
    v-if="preview"
    title="确认这次整理"
    wide
    :locked="busy"
    @close="preview = null"
    ><div class="preview-summary">
      <span class="state-pill">{{ modeNames[preview.mode] }}</span
      ><strong>{{ preview.title }}</strong
      ><span>{{ preview.rows.length }} 个视频</span
      ><span>预计空间 {{ bytes(preview.required) }}</span>
    </div>
    <p v-if="preview.mode === 'move'" class="notice warning">
      移动会在校验成功后删除源文件，请确认目标目录。
    </p>
    <div v-if="preview.issues.length" class="notice error">
      <strong>还有 {{ preview.issues.length }} 项问题</strong>
      <p v-for="issue in preview.issues.slice(0, 5)" :key="issue">
        {{ issue }}
      </p>
    </div>
    <p v-if="message" class="notice error" role="alert">{{ message }}</p>
    <div class="search-input preview-search">
      <Search :size="15" /><input
        v-model="previewFilter"
        placeholder="筛选源文件或目标路径"
        aria-label="筛选预览"
      />
    </div>
    <div class="preview-table">
      <div class="preview-table-head">
        <span>源文件 → 目标路径</span><span>关联产物</span><span>状态</span>
      </div>
      <article
        v-for="row in shownPreview"
        :key="row.source"
        class="preview-row"
      >
        <div class="preview-paths">
          <strong :title="row.source">{{ name(row.source) }}</strong
          ><span
            ><ArrowRight :size="13" /><code>{{
              row.target || '无法生成路径'
            }}</code></span
          >
          <details>
            <summary>完整源路径</summary>
            <code>{{ row.source }}</code>
          </details>
        </div>
        <div class="artifact-list">
          <span v-for="artifact in row.artifacts" :key="artifact">{{
            artifact
          }}</span
          ><span v-if="!row.artifacts.length" class="muted">仅视频</span>
        </div>
        <div>
          <span v-if="!row.issues.length" class="ready-label"
            ><Check :size="13" />就绪</span
          ><button
            v-else
            class="text-button danger"
            :disabled="busy"
            @click="skip(row.source)"
          >
            跳过此文件
          </button>
        </div>
      </article>
    </div>
    <div v-if="previewRows.length > 30" class="records-pagination">
      <button
        class="button small"
        :disabled="previewPage === 1"
        @click="previewPage--"
      >
        上一页</button
      >{{ previewPage }} / {{ Math.ceil(previewRows.length / 30)
      }}<button
        class="button small"
        :disabled="previewPage * 30 >= previewRows.length"
        @click="previewPage++"
      >
        下一页
      </button>
    </div>
    <template #footer
      ><span class="tiny muted"><ShieldCheck :size="14" />不覆盖已有文件</span>
      <div class="footer-buttons">
        <button class="button" :disabled="busy" @click="preview = null">
          返回调整</button
        ><button
          class="button primary"
          :disabled="busy || preview.issues.length > 0 || !preview.rows.length"
          @click="execute"
        >
          确认并执行<ArrowRight :size="16" />
        </button></div></template
  ></Modal>
  <Modal v-if="detail" :title="detail.title" wide @close="detail = null"
    ><div class="preview-summary">
      <span class="state-pill" :class="detail.state">{{
        states[detail.state]
      }}</span
      ><span>{{ modeNames[detail.mode] }}</span
      ><span>{{ date(detail.created) }}</span
      ><span>{{ detail.completed }}/{{ detail.total }} 项</span>
    </div>
    <p v-if="detail.error" class="notice error preserve-lines">
      {{ detail.error }}
    </p>
    <div class="operation-list">
      <article v-for="(op, index) in detail.ops" :key="index">
        <span class="operation-kind">{{ kindNames[op.kind] }}</span>
        <div>
          <code>{{ op.target }}</code>
          <p v-if="op.error" class="danger">{{ op.error }}</p>
        </div>
        <span
          class="tiny"
          :class="op.status === 'completed' ? 'green' : 'muted'"
          >{{ states[op.status] || op.status }}</span
        >
      </article>
    </div>
    <template #footer
      ><span class="muted tiny">文件变化时会停止操作</span>
      <div class="footer-buttons">
        <button
          v-if="
            ['failed', 'interrupted', 'cancelled'].includes(detail.state) &&
            !detail.undo
          "
          class="button"
          :disabled="busy || running"
          @click="recordAction(detail, 'retry')"
        >
          重试未完成项</button
        ><button
          v-if="detail.state !== 'undone' && detail.state !== 'running'"
          class="button"
          :disabled="busy || running"
          @click="undoing = detail"
        >
          <Undo2 :size="15" />{{ detail.undo ? '继续撤销' : '撤销本次整理' }}
        </button>
      </div></template
    ></Modal
  >
  <Modal
    v-if="undoing"
    title="撤销这次整理？"
    :locked="busy"
    @close="undoing = null"
    ><p>将撤销「{{ undoing.title }}」的文件操作。</p>
    <p class="muted">
      {{
        undoing.mode === 'move'
          ? '视频和字幕将恢复到原路径。原路径被占用时停止对应操作。'
          : '将移除本次创建的目标文件，来源文件保持原样。'
      }}目标内容被修改时会停止，不会删除被替换的文件。
    </p>
    <template #footer
      ><button class="button" @click="undoing = null">保留整理结果</button
      ><button
        class="button primary"
        :disabled="busy"
        @click="recordAction(undoing!, 'undo')"
      >
        确认撤销
      </button></template
    ></Modal
  >
</template>
