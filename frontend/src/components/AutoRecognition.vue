<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  ArrowRight,
  Check,
  CircleCheck,
  Film,
  FolderOpen,
  Layers3,
  LoaderCircle,
  Pencil,
  Plus,
  Search,
} from '@lucide/vue'
import { api, bytes, episodeCode, name, parseEpisodes } from '../api'
import type {
  Batch,
  BatchPreview,
  Entry,
  Recognition,
  RecognitionItem,
  Run,
  Season,
} from '../api'
import {
  recognitionEdits,
  recognitionGroups,
  recognitionStates,
} from '../recognition'
import FileBrowser from './FileBrowser.vue'
import Modal from './Modal.vue'

const props = defineProps<{ executionBusy?: boolean }>()
const emit = defineEmits<{
  debug: []
  rules: []
  history: []
  trace: [id: string]
  batch: [value: Batch | null]
  inspect: [run: Run]
}>()
const task = ref<Recognition | null>(null),
  batch = ref<Batch | null>(null),
  preview = ref<BatchPreview | null>(null)
const busy = ref(false),
  loading = ref(true),
  error = ref(''),
  notice = ref(''),
  browser = ref(false)
const filter = ref('all'),
  query = ref(''),
  focus = ref(''),
  mode = ref('copy'),
  groupPage = ref(1),
  filePage = ref(1)
const searchGroup = ref(''),
  searchQuery = ref(''),
  searchYear = ref(''),
  searchKind = ref<'movie' | 'tv'>('movie'),
  searchResults = ref<Entry[]>([]),
  searchBusy = ref(false),
  searchError = ref('')
const editing = ref<RecognitionItem | null>(null),
  editSeason = ref(-1),
  editEpisodes = ref(''),
  seasons = ref<Season[]>([]),
  editError = ref('')
const transfer = ref<RecognitionItem | null>(null),
  transferGroup = ref('new')
const groupSeason = ref<number>(1),
  freshImport = ref(false)
let timer: ReturnType<typeof setInterval> | undefined,
  alive = true,
  polling = false,
  searchRequest = 0,
  editRequest = 0,
  lastBatchSync = 0
const recognizing = computed(() => task.value?.state === 'running')
const locked = computed(
  () =>
    busy.value ||
    recognizing.value ||
    !!batch.value?.active ||
    props.executionBusy,
)
const groups = computed(() => recognitionGroups(task.value?.items || []))
const filtered = computed(() =>
  groups.value.filter(
    (group) =>
      (filter.value === 'all' ||
        group.files.some((file) => file.state === filter.value)) &&
      `${group.title} ${group.files.map((file) => file.path).join(' ')}`
        .toLowerCase()
        .includes(query.value.toLowerCase()),
  ),
)
const visibleGroups = computed(() =>
  filtered.value.slice((groupPage.value - 1) * 30, groupPage.value * 30),
)
const current = computed(
  () =>
    groups.value.find((group) => group.id === focus.value) || filtered.value[0],
)
const visibleFiles = computed(
  () =>
    current.value?.files.slice(
      (filePage.value - 1) * 40,
      filePage.value * 40,
    ) || [],
)
const selected = computed(
  () =>
    task.value?.items.filter(
      (file) => file.selected && !file.removed && file.state === 'ready',
    ) || [],
)
const pendingCount = computed(
  () =>
    task.value?.items.filter((file) => !file.removed && file.state !== 'ready')
      .length || 0,
)
const readyCount = computed(
  () =>
    task.value?.items.filter((file) => !file.removed && file.state === 'ready')
      .length || 0,
)
const runStates: Record<string, string> = {
  queued: '等待中',
  running: '进行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
  interrupted: '已中断',
  undone: '已撤销',
}
const modes: Record<string, string> = {
  copy: '复制',
  move: '移动',
  hardlink: '硬链接',
}
const previewPage = ref(1)
const previewRows = computed(
  () =>
    preview.value?.rows.slice(
      (previewPage.value - 1) * 30,
      previewPage.value * 30,
    ) || [],
)

async function act(fn: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await fn()
  } catch (e) {
    if (alive) error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
async function save() {
  if (!task.value) return
  preview.value = null
  const original = task.value
  try {
    const value = await api<Recognition>(
      `/recognitions/${original.id}`,
      { revision: original.revision, items: recognitionEdits(original.items) },
      'PUT',
    )
    if (alive) task.value = value
  } catch (e) {
    // Reload the authoritative snapshot after a rejected edit (including edits
    // made from another tab), so an optimistic checkbox cannot look saved.
    const value = await api<Recognition>(`/recognitions/${original.id}`).catch(
      () => null,
    )
    if (alive && value) task.value = value
    throw e
  }
}
function openBrowser(fresh = false) {
  freshImport.value = fresh
  browser.value = true
}
async function start(paths: string[]) {
  browser.value = false
  await act(async () => {
    const value = await api<Recognition>('/recognitions', {
      paths,
      id: freshImport.value ? '' : task.value?.id || '',
    })
    if (!alive) return
    task.value = value
    preview.value = null
    focus.value = ''
    filePage.value = 1
    groupPage.value = 1
  })
}
async function load() {
  try {
    const [value, latestBatch] = await Promise.all([
      api<Recognition | null>('/recognitions'),
      api<Batch | null>('/batches'),
    ])
    if (!alive) return
    task.value = value
    batch.value = latestBatch
    emit('batch', latestBatch)
  } catch (e) {
    if (alive) error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}
async function poll() {
  if (polling || busy.value || !alive) return
  polling = true
  try {
    if (recognizing.value && task.value) {
      const id = task.value.id
      const value = await api<Recognition>(`/recognitions/${id}`)
      if (
        alive &&
        !busy.value &&
        task.value?.id === id &&
        value.revision >= task.value.revision
      )
        task.value = value
    }
    if (batch.value?.active || Date.now() - lastBatchSync > 5000) {
      const wasActive = batch.value?.active
      const value = await api<Batch | null>('/batches')
      lastBatchSync = Date.now()
      if (alive && !busy.value) {
        batch.value = value
        emit('batch', value)
      }
      if ((wasActive || value?.active) && task.value) {
        const id = task.value.id
        const updated = await api<Recognition>(`/recognitions/${id}`)
        if (
          alive &&
          !busy.value &&
          task.value?.id === id &&
          updated.revision >= task.value.revision
        )
          task.value = updated
      }
    }
  } catch (e) {
    if (alive) error.value = (e as Error).message
  } finally {
    polling = false
  }
}
async function retry() {
  await act(async () => {
    if (task.value)
      task.value = await api<Recognition>(
        `/recognitions/${task.value.id}/retry`,
        {},
      )
    preview.value = null
  })
}
async function cancelRecognition() {
  await act(async () => {
    if (task.value) await api(`/recognitions/${task.value.id}/cancel`, {})
    await poll()
  })
}
async function selectFile(file: RecognitionItem) {
  await act(async () => {
    file.selected = !file.selected
    await save()
  })
}
async function selectReady(value: boolean) {
  await act(async () => {
    for (const group of filtered.value)
      for (const file of group.files)
        file.selected = value && file.state === 'ready'
    await save()
  })
}
async function selectGroup(id: string) {
  await act(async () => {
    const group = groups.value.find((g) => g.id === id)
    if (!group) return
    const ready = group.files.filter((file) => file.state === 'ready')
    const value = !ready.every((file) => file.selected)
    for (const file of ready) file.selected = value
    await save()
  })
}
async function removeFile(file: RecognitionItem) {
  await act(async () => {
    file.removed = true
    file.selected = false
    await save()
    filePage.value = 1
  })
}
function openSearch() {
  if (!current.value) return
  searchGroup.value = current.value.id
  searchQuery.value =
    current.value.entry?.title || current.value.files[0]!.parsed.title
  searchYear.value = String(current.value.files[0]!.parsed.year || '')
  searchKind.value =
    current.value.entry?.type ||
    (current.value.files[0]!.parsed.type === 'tv' ? 'tv' : 'movie')
  const found = new Map<string, Entry>()
  for (const file of current.value.files)
    for (const candidate of file.candidates)
      found.set(`${candidate.type}:${candidate.tmdbId}`, candidate)
  searchResults.value = [...found.values()]
  searchError.value = ''
  searchRequest++
}
async function search() {
  const request = ++searchRequest
  searchBusy.value = true
  searchError.value = ''
  try {
    const value = await api<Entry[]>(
      `/search?type=${searchKind.value}&q=${encodeURIComponent(searchQuery.value.trim())}&year=${encodeURIComponent(searchYear.value)}`,
    )
    if (alive && request === searchRequest) searchResults.value = value
  } catch (e) {
    if (alive && request === searchRequest)
      searchError.value = (e as Error).message
  } finally {
    if (request === searchRequest) searchBusy.value = false
  }
}
async function pick(entry: Entry) {
  await act(async () => {
    for (const file of task.value?.items || [])
      if (file.group === searchGroup.value && !file.removed) {
        file.entry = entry
        file.group = `tmdb:${entry.type}:${entry.tmdbId}`
        file.selected = true
      }
    const id = `tmdb:${entry.type}:${entry.tmdbId}`
    await save()
    focus.value = id
    filePage.value = 1
    searchGroup.value = ''
  })
}
async function openEdit(file: RecognitionItem) {
  editing.value = file
  editSeason.value = file.season
  editEpisodes.value = file.episodes.join(',')
  editError.value = ''
  seasons.value = []
  const request = ++editRequest
  if (file.entry?.type === 'tv')
    try {
      const value = await api<Season[]>(`/seasons?id=${file.entry.tmdbId}`)
      if (alive && request === editRequest) seasons.value = value
    } catch (e) {
      if (alive && request === editRequest)
        editError.value = (e as Error).message
    }
}
async function saveEdit() {
  try {
    if (
      !Number.isInteger(editSeason.value) ||
      editSeason.value < 0 ||
      editSeason.value > 999
    )
      throw new Error('请选择 0–999 的季号，0 为特别篇')
    const eps = parseEpisodes(editEpisodes.value)
    await act(async () => {
      if (!editing.value) return
      editing.value.season = editSeason.value
      editing.value.episodes = eps
      editing.value.selected = true
      await save()
      editing.value = null
    })
  } catch (e) {
    editError.value = (e as Error).message
  }
}
async function setSeason() {
  await act(async () => {
    if (
      !Number.isInteger(groupSeason.value) ||
      groupSeason.value < 0 ||
      groupSeason.value > 999
    )
      throw new Error('季号为 0–999')
    for (const file of current.value?.files || []) {
      file.season = groupSeason.value
      file.selected = true
    }
    await save()
  })
}
async function moveGroup() {
  await act(async () => {
    if (!transfer.value) return
    const target = groups.value.find(
      (group) => group.id === transferGroup.value,
    )
    transfer.value.group =
      target?.id ||
      `custom:${Date.now()}:${Math.random().toString(16).slice(2)}`
    transfer.value.entry = target?.entry || null
    transfer.value.selected = !!target?.entry
    const id = transfer.value.group
    await save()
    transfer.value = null
    focus.value = id
    filePage.value = 1
  })
}
async function prepare() {
  await act(async () => {
    if (!task.value) return
    preview.value = await api<BatchPreview>('/batches/preview', {
      recognitionId: task.value.id,
      revision: task.value.revision,
      mode: mode.value,
    })
    previewPage.value = 1
  })
}
async function execute() {
  await act(async () => {
    if (!preview.value) return
    batch.value = await api<Batch>('/batches/execute', { id: preview.value.id })
    emit('batch', batch.value)
    preview.value = null
    notice.value = '已加入整理队列，每部作品分别记录，可独立重试或撤销。'
  })
}
async function cancelBatch() {
  await act(async () => {
    if (batch.value) await api(`/batches/${batch.value.id}/cancel`, {})
  })
}
function chooseGroup(id: string) {
  focus.value = id
  filePage.value = 1
}
function openTransfer(file: RecognitionItem) {
  transfer.value = file
  transferGroup.value = 'new'
}
function closeSearch() {
  searchGroup.value = ''
  searchRequest++
  searchBusy.value = false
}
function closeEdit() {
  editing.value = null
  editRequest++
}
function changeFilter() {
  focus.value = ''
  groupPage.value = 1
  filePage.value = 1
}
onMounted(() => {
  void load()
  timer = setInterval(poll, 1200)
})
onUnmounted(() => {
  alive = false
  clearInterval(timer)
  searchRequest++
  editRequest++
})
</script>

<template>
  <section class="auto-workspace" aria-label="自动识别">
    <div class="workspace-heading">
      <div>
        <span class="eyebrow">DISCOVER EVERY STORY</span>
        <h1>自动识别<span class="heading-dot">.</span></h1>
        <p class="muted">电影与剧集一起添加，按作品检查，再统一整理。</p>
      </div>
      <div class="auto-heading-actions">
        <button class="text-button" @click="emit('rules')">识别规则</button>
        <button class="text-button" @click="emit('history')">执行历史</button>
        <button class="text-button" @click="emit('debug')">识别调试</button>
        <button
          v-if="task"
          class="text-button"
          :disabled="locked || loading"
          @click="openBrowser(true)"
        >
          新建识别</button
        ><button
          class="button primary"
          :disabled="locked || loading"
          @click="openBrowser()"
        >
          <Plus :size="17" />添加文件或目录
        </button>
      </div>
    </div>
    <p v-if="error" class="notice error" role="alert">
      {{ error
      }}<button class="text-button" :disabled="busy" @click="act(load)">
        重新加载
      </button>
    </p>
    <p v-if="notice" class="notice good" role="status">{{ notice }}</p>
    <div v-if="loading" class="pane empty">
      <LoaderCircle class="spin" />读取识别工作区…
    </div>
    <template v-else>
      <div v-if="recognizing" class="auto-progress pane" role="status">
        <LoaderCircle class="spin purple" :size="22" />
        <div>
          <strong
            >正在识别 {{ task?.completed }}/{{
              task?.total || '扫描中'
            }}</strong
          >
          <p class="muted">
            已解析的结果会自动出现，可以切换页面，稍后继续查看。
          </p>
        </div>
        <button
          class="button small"
          :disabled="busy"
          @click="cancelRecognition"
        >
          取消识别
        </button>
      </div>
      <p v-if="task?.error" class="notice error">{{ task.error }}</p>
      <div
        v-if="
          task &&
          !recognizing &&
          (task.state !== 'completed' ||
            task.items.some((file) => file.state === 'failed'))
        "
        class="auto-retry"
      >
        <button class="button small" :disabled="locked" @click="retry">
          重试失败或未完成项
        </button>
      </div>
      <div v-if="!task" class="pane empty auto-empty">
        <Layers3 :size="42" class="purple" />
        <h2>让每部作品找到自己的位置</h2>
        <p class="muted">
          添加混合目录，自动识别片名、年份和季集。匹配不确定时，由你选择。
        </p>
        <button
          class="button primary"
          :disabled="locked"
          @click="openBrowser()"
        >
          <FolderOpen :size="17" />选择本地文件
        </button>
      </div>
      <template v-else>
        <div class="auto-stats">
          <span
            ><strong>{{ groups.length }}</strong> 部作品</span
          ><span
            ><strong>{{ readyCount }}</strong> 个可整理文件</span
          ><span
            ><strong>{{ pendingCount }}</strong> 个待处理文件</span
          ><span
            ><strong>{{ selected.length }}</strong> 个已选</span
          >
        </div>
        <div class="auto-toolbar">
          <div class="search-input">
            <Search :size="16" /><input
              v-model="query"
              aria-label="筛选作品或文件"
              placeholder="筛选作品或文件"
              @input="changeFilter"
            />
          </div>
          <select v-model="filter" aria-label="识别状态" @change="changeFilter">
            <option value="all">全部状态</option>
            <option
              v-for="state in [
                'ready',
                'review',
                'unmatched',
                'failed',
                'pending',
              ]"
              :key="state"
              :value="state"
            >
              {{ recognitionStates[state] }}
            </option>
          </select>
          <button
            class="text-button"
            :disabled="locked"
            @click="selectReady(true)"
          >
            选中可整理项</button
          ><button
            class="text-button"
            :disabled="locked"
            @click="selectReady(false)"
          >
            取消选择
          </button>
        </div>
        <div class="auto-grid">
          <section class="pane auto-groups" aria-label="作品分组">
            <header class="pane-head">
              <h2>作品分组</h2>
              <span class="count">{{ filtered.length }}</span>
            </header>
            <div class="auto-group-list">
              <div
                v-for="group in visibleGroups"
                :key="group.id"
                class="auto-group"
                :class="{ active: current?.id === group.id }"
              >
                <input
                  type="checkbox"
                  :aria-label="`选择 ${group.title}`"
                  :checked="group.files.some((file) => file.selected)"
                  :disabled="
                    locked ||
                    !group.files.some((file) => file.state === 'ready')
                  "
                  @change="selectGroup(group.id)"
                />
                <button class="auto-group-main" @click="chooseGroup(group.id)">
                  <strong>{{ group.title }}</strong
                  ><span class="muted"
                    >{{
                      group.entry?.year ||
                      group.files[0]?.parsed.year ||
                      '年份未知'
                    }}
                    ·
                    {{
                      group.entry?.type === 'movie'
                        ? '电影'
                        : group.entry?.type === 'tv'
                          ? '剧集'
                          : '类型待确认'
                    }}
                    · {{ group.files.length }} 个文件</span
                  ><span class="state-pill" :class="group.state">{{
                    recognitionStates[group.state]
                  }}</span>
                </button>
              </div>
              <p v-if="!filtered.length" class="empty small-empty">
                没有符合条件的作品。
              </p>
            </div>
            <div v-if="filtered.length > 30" class="auto-pagination">
              <button
                class="text-button"
                :disabled="groupPage === 1"
                @click="groupPage--"
              >
                上一页</button
              ><span>{{ groupPage }}/{{ Math.ceil(filtered.length / 30) }}</span
              ><button
                class="text-button"
                :disabled="groupPage * 30 >= filtered.length"
                @click="groupPage++"
              >
                下一页
              </button>
            </div>
          </section>
          <section class="pane auto-detail" aria-label="分组文件">
            <template v-if="current">
              <header class="pane-head">
                <div>
                  <h2>{{ current.title }}</h2>
                  <p class="muted tiny">
                    {{
                      current.entry
                        ? `TMDB ${current.entry.tmdbId}`
                        : '选择候选作品，或手动搜索'
                    }}
                  </p>
                </div>
                <button
                  class="button small"
                  :disabled="locked"
                  @click="openSearch"
                >
                  <Search :size="15" />{{
                    current.entry ? '修改作品' : '选择作品'
                  }}
                </button>
              </header>
              <div v-if="current.entry?.type === 'tv'" class="auto-season">
                <label
                  >整组季号
                  <input
                    v-model.number="groupSeason"
                    type="number"
                    min="0"
                    max="999"
                    aria-label="整组季号"
                    :disabled="locked" /></label
                ><button
                  class="button small"
                  :disabled="locked"
                  @click="setSeason"
                >
                  应用到本组</button
                ><span class="muted tiny">0 为特别篇，集数保持原值</span>
              </div>
              <div class="auto-files">
                <article
                  v-for="file in visibleFiles"
                  :key="file.path"
                  class="auto-file"
                >
                  <input
                    type="checkbox"
                    :aria-label="`选择文件 ${name(file.path)}`"
                    :checked="file.selected"
                    :disabled="locked || file.state !== 'ready'"
                    @change="selectFile(file)"
                  />
                  <div class="auto-file-main">
                    <strong :title="file.path">{{ name(file.path) }}</strong>
                    <p class="muted tiny auto-path" :title="file.path">
                      {{ file.path }}
                    </p>
                    <div class="auto-file-meta">
                      <span class="state-pill" :class="file.state">{{
                        recognitionStates[file.state]
                      }}</span
                      ><span v-if="file.entry?.type === 'tv'" class="tiny">{{
                        file.season < 0
                          ? '季号未知'
                          : episodeCode(file.season, file.episodes)
                      }}</span
                      ><span class="muted tiny">{{ bytes(file.size) }}</span>
                    </div>
                    <p class="tiny muted">{{ file.reason || '等待识别' }}</p>
                    <div class="auto-file-actions">
                      <button v-if="file.traceId" class="text-button" @click="emit('trace', file.traceId)">查看识别过程</button><span v-else class="tiny muted">无规则执行轨迹</span>
                      <button
                        v-if="file.entry?.type === 'tv'"
                        class="text-button"
                        :disabled="locked"
                        @click="openEdit(file)"
                      >
                        <Pencil :size="13" />修正季集</button
                      ><button
                        class="text-button"
                        :disabled="locked"
                        @click="openTransfer(file)"
                      >
                        调整分组</button
                      ><button
                        class="text-button"
                        :disabled="locked"
                        @click="removeFile(file)"
                      >
                        移出
                      </button>
                    </div>
                  </div>
                </article>
              </div>
              <div v-if="current.files.length > 40" class="auto-pagination">
                <button
                  class="text-button"
                  :disabled="filePage === 1"
                  @click="filePage--"
                >
                  上一页</button
                ><span
                  >{{ filePage }}/{{
                    Math.ceil(current.files.length / 40)
                  }}</span
                ><button
                  class="text-button"
                  :disabled="filePage * 40 >= current.files.length"
                  @click="filePage++"
                >
                  下一页
                </button>
              </div>
            </template>
            <div v-else class="empty">
              <Film :size="30" />
              <p>选择作品，检查文件对应关系。</p>
            </div>
          </section>
        </div>
        <div class="auto-footer pane">
          <div>
            <strong>已选 {{ selected.length }} 个文件</strong>
            <p class="muted tiny">
              待确认的文件可以稍后处理；每部作品分别记录。
            </p>
          </div>
          <select v-model="mode" aria-label="自动整理方式" :disabled="locked">
            <option v-for="(label, value) in modes" :key="value" :value="value">
              {{ label }}
            </option></select
          ><button
            class="button primary"
            :disabled="locked || !selected.length || selected.length > 1000"
            @click="prepare"
          >
            <LoaderCircle
              v-if="busy"
              class="spin"
              :size="16"
            />统一预览<ArrowRight :size="16" />
          </button>
        </div>
      </template>
      <section v-if="batch" class="pane auto-batch" aria-label="自动整理队列">
        <header class="pane-head">
          <div>
            <h2>{{ batch.active ? '正在按作品整理' : '最近整理批次' }}</h2>
            <p class="muted tiny">
              {{
                batch.runs.filter((run) => run.state === 'completed').length
              }}/{{ batch.runs.length }} 部作品已完成
            </p>
          </div>
          <button
            v-if="batch.active"
            class="button small"
            :disabled="busy"
            @click="cancelBatch"
          >
            取消整批
          </button>
        </header>
        <div class="auto-batch-runs">
          <button
            v-for="run in batch.runs"
            :key="run.id"
            class="auto-batch-run"
            @click="emit('inspect', run)"
          >
            <LoaderCircle
              v-if="run.state === 'running'"
              class="spin purple"
              :size="16"
            /><CircleCheck
              v-else-if="run.state === 'completed'"
              class="green"
              :size="16"
            /><Film v-else :size="16" /><strong>{{ run.title }}</strong
            ><span class="state-pill" :class="run.state">{{
              runStates[run.state]
            }}</span
            ><span class="tiny muted">{{ run.completed }}/{{ run.total }}</span>
          </button>
        </div>
      </section>
    </template>
  </section>
  <FileBrowser v-if="browser" confirm-label="开始识别" @close="browser = false" @select="start" />
  <Modal
    v-if="searchGroup"
    title="选择对应作品"
    @close="closeSearch"
  >
    <p class="muted">选择后应用到当前组的所有文件，同一作品会合并显示。</p>
    <form class="tmdb-search" @submit.prevent="search">
      <select v-model="searchKind" aria-label="搜索类型">
        <option value="movie">电影</option>
        <option value="tv">剧集</option></select
      ><input
        v-model="searchQuery"
        placeholder="片名"
        aria-label="搜索片名"
        required
      /><input
        v-model="searchYear"
        placeholder="年份（可选）"
        aria-label="搜索年份"
        type="number"
      /><button
        class="button primary"
        :disabled="searchBusy || !searchQuery.trim()"
      >
        <Search :size="16" />搜索
      </button>
    </form>
    <p v-if="searchError" class="notice error">{{ searchError }}</p>
    <div class="search-results">
      <button
        v-for="entry in searchResults"
        :key="`${entry.type}:${entry.tmdbId}`"
        class="search-result"
        :disabled="busy || searchBusy"
        @click="pick(entry)"
      >
        <img
          v-if="entry.posterPath"
          :src="`https://image.tmdb.org/t/p/w185${entry.posterPath}`"
          alt=""
        />
        <div v-else class="result-poster"><Film :size="24" /></div>
        <span
          ><strong>{{ entry.title }}</strong
          ><small
            >{{ entry.originalTitle }} · {{ entry.year || '年份未知' }} ·
            {{ entry.type === 'tv' ? '剧集' : '电影' }} · TMDB
            {{ entry.tmdbId }}</small
          >
          <p>{{ entry.overview }}</p></span
        ><Check :size="17" />
      </button>
      <p v-if="!searchResults.length" class="empty small-empty">
        {{
          searchBusy
            ? '正在搜索…'
            : '没有候选作品，请调整片名、年份或类型后搜索。'
        }}
      </p>
    </div>
  </Modal>
  <Modal
    v-if="editing"
    title="修正季集"
    @close="closeEdit"
    ><p class="muted">{{ name(editing.path) }}</p>
    <label
      >季号<input
        v-model.number="editSeason"
        type="number"
        min="0"
        max="999"
        list="auto-season-list"
      /><datalist id="auto-season-list">
        <option
          v-for="season in seasons"
          :key="season.number"
          :value="season.number"
        >
          {{ season.name }}
        </option>
      </datalist></label
    ><label
      >集数<input v-model="editEpisodes" placeholder="例如 1,3-5"
    /></label>
    <p v-if="editError" class="notice error">{{ editError }}</p>
    <template #footer
      ><span class="muted">0 为特别篇</span
      ><button class="button primary" :disabled="busy" @click="saveEdit">
        保存并校验
      </button></template
    ></Modal
  >
  <Modal v-if="transfer" title="调整文件分组" @close="transfer = null"
    ><p class="muted">{{ name(transfer.path) }}</p>
    <label
      >移至分组<select v-model="transferGroup">
        <option value="new">新建待确认分组</option>
        <option
          v-for="group in groups.filter(
            (group) => group.id !== transfer?.group,
          )"
          :key="group.id"
          :value="group.id"
        >
          {{ group.title }}
        </option>
      </select></label
    >
    <p class="muted">
      新分组需要重新选择作品；移入已有分组时会校验作品和季集。
    </p>
    <template #footer
      ><span /><button
        class="button primary"
        :disabled="busy"
        @click="moveGroup"
      >
        保存分组
      </button></template
    ></Modal
  >
  <Modal v-if="preview" title="确认多作品整理预览" wide @close="preview = null"
    ><div class="auto-preview-summary">
      <strong
        >{{ preview.groups.length }} 部作品 ·
        {{ preview.rows.length }} 个视频</strong
      ><span
        >{{ modes[preview.mode] }} · 需要 {{ bytes(preview.required) }}</span
      >
    </div>
    <p class="muted">每部作品单独记录并依次执行，可独立重试或撤销。</p>
    <div v-if="preview.issues.length" class="notice error">
      <p v-for="issue in preview.issues" :key="issue">{{ issue }}</p>
      <p>关闭预览，调整选中文件或分组后重新预览。</p>
    </div>
    <div class="auto-preview-rows">
      <article v-for="row in previewRows" :key="row.source">
        <strong>{{ name(row.source) }}</strong>
        <p class="muted tiny auto-path">{{ row.source }}</p>
        <p class="auto-path">→ {{ row.target }}</p>
        <p v-if="row.artifacts.length" class="muted tiny">
          {{ row.artifacts.join(' · ') }}
        </p>
        <p v-for="issue in row.issues" :key="issue" class="red tiny">
          {{ issue }}
        </p>
      </article>
    </div>
    <div v-if="preview.rows.length > 30" class="auto-pagination">
      <button
        class="text-button"
        :disabled="previewPage === 1"
        @click="previewPage--"
      >
        上一页</button
      ><span>{{ previewPage }}/{{ Math.ceil(preview.rows.length / 30) }}</span
      ><button
        class="text-button"
        :disabled="previewPage * 30 >= preview.rows.length"
        @click="previewPage++"
      >
        下一页
      </button>
    </div>
    <template #footer
      ><span class="muted"
        >确认后依次执行 {{ preview.groups.length }} 部作品</span
      ><button
        class="button primary"
        :disabled="busy || !!preview.issues.length"
        @click="execute"
      >
        确认整理
      </button></template
    ></Modal
  >
</template>
