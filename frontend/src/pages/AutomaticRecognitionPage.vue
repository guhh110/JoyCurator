<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowLeftRight,
  ArrowRight,
  Check,
  CircleAlert,
  Film,
  FolderOpen,
  Layers3,
  LoaderCircle,
  Pencil,
  Plus,
  RotateCcw,
  ScrollText,
  Search,
  Trash2,
  Tv,
  X,
} from '@lucide/vue'
import { RouterLink } from 'vue-router'
import { bytes, episodeCode, name } from '../api'
import { recognitionStates, sourceFolder } from '../recognition'
import FileBrowser from '../components/FileBrowser.vue'
import Modal from '../components/Modal.vue'
import QueryState from '../components/ui/QueryState.vue'
import PageHeader from '../components/ui/PageHeader.vue'
import Select from '../components/ui/Select.vue'
import Tooltip from '../components/ui/Tooltip.vue'
import { useAutomaticRecognition } from '../features/automatic/useAutomaticRecognition'
const {
  runtime,
  busy,
  error,
  notice,
  task,
  preview,
  browser,
  filter,
  query,
  focus,
  mode,
  groupPage,
  filePage,
  searchGroup,
  searchQuery,
  searchYear,
  searchKind,
  searchResults,
  searchBusy,
  searchError,
  editing,
  editSeason,
  editEpisodes,
  seasons,
  editError,
  transfer,
  transferGroup,
  groupSeason,
  savingSeasonGroup,
  groupSeasons,
  groupSeasonsLoading,
  groupSeasonsError,
  freshImport,
  recognizing,
  locked,
  groups,
  filtered,
  visibleGroups,
  current,
  visibleFiles,
  selected,
  pendingCount,
  readyCount,
  modes,
  previewPage,
  previewRows,
  taskQuery,
  loading,
  act,
  save,
  openBrowser,
  start,
  poll,
  retry,
  cancelRecognition,
  selectFile,
  selectReady,
  selectGroup,
  removeFile,
  openSearch,
  search,
  pick,
  openEdit,
  saveEdit,
  setSeason,
  moveGroup,
  prepare,
  execute,
  chooseGroup,
  openTransfer,
  closeSearch,
  closeEdit,
  changeFilter,
  source,
  sources,
  chooseSource,
  removeSource,
  stateCounts,
} = useAutomaticRecognition()
const fileStates = ['ready', 'review', 'unmatched', 'failed', 'pending']
const stateFilters = ['all', 'review', 'unmatched', 'failed', 'pending', 'ready']
type Group = (typeof groups.value)[number]
const poster = (path: string) => `https://image.tmdb.org/t/p/w154${path}`
const workYear = (group: Group) =>
  group.entry?.year || group.files[0]?.metadata.year || '年份未知'
const workType = (group: Group) =>
  group.entry?.type === 'movie'
    ? '电影'
    : group.entry?.type === 'tv'
      ? '剧集'
      : '类型待确认'
// The same issue is often reported once per file; show it once with a count.
const previewIssues = computed(() => {
  const counts = new Map<string, number>()
  for (const issue of preview.value?.issues || [])
    if (!conflictPath(issue)) counts.set(issue, (counts.get(issue) || 0) + 1)
  return [...counts]
})
function conflictPath(issue: string) {
  return /^(?:目标已存在|目标重复|作品之间目标重复)：(.+)$/.exec(issue)?.[1] || ''
}
function conflictTargets(issues: string[]) {
  return [...new Set(issues.map(conflictPath).filter(Boolean))]
}
const folderOf = (path: string) =>
  sourceFolder(path, task.value?.paths || []) || '直接添加的文件'
</script>

<template>
  <section
    class="auto-workspace"
    :class="{ 'has-task': task }"
    aria-label="自动识别"
  >
    <PageHeader
      class="workflow-heading"
      title="自动识别"
      eyebrow="DISCOVER EVERY STORY"
      description="电影与剧集一起添加，按作品检查，再统一整理。"
    />
    <QueryState
      v-if="taskQuery.error.value || runtime.batchQuery.error.value"
      :error="taskQuery.error.value || runtime.batchQuery.error.value"
      @retry="poll"
    />
    <p v-if="error" class="notice error" role="alert">
      {{ error
      }}<button class="text-button" :disabled="busy" @click="act(poll)">
        重新加载
      </button>
    </p>
    <p v-if="notice" class="notice good" role="status">{{ notice }}</p>
    <div v-if="loading && !task" class="pane empty auto-initial-loading">
      <LoaderCircle class="spin" />读取识别工作区…
    </div>
    <template v-if="task || !loading">
      <div v-if="recognizing" class="auto-progress" role="status">
        <LoaderCircle class="spin purple" :size="18" />
        <div>
          <strong
            >正在识别 {{ task?.completed }}/{{
              task?.total || '扫描中'
            }}</strong
          ><span class="muted"
            >已解析的结果会自动出现，可以切换页面，稍后继续查看。</span
          >
          <i
            class="auto-progress-bar"
            :style="{
              '--done': task?.total
                ? `${(task.completed / task.total) * 100}%`
                : '0%',
            }"
          />
        </div>
        <button
          class="button small"
          :disabled="busy"
          @click="cancelRecognition"
        >
          取消识别
        </button>
      </div>
      <div
        v-if="
          task &&
          !recognizing &&
          (task.error ||
            task.state !== 'completed' ||
            task.items.some((file) => file.state === 'failed'))
        "
        class="notice error auto-retry"
      >
        <span>{{ task.error || '部分文件没有完成识别。' }}</span
        ><button class="text-button" :disabled="locked" @click="retry">
          <RotateCcw :size="13" />重试失败或未完成项
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
        <div class="auto-toolbar">
          <div class="search-input">
            <Search :size="15" /><input
              v-model="query"
              aria-label="筛选作品或文件"
              placeholder="筛选作品或文件"
              @input="changeFilter"
            />
          </div>
          <div class="auto-filters" role="group" aria-label="识别状态">
            <button
              v-for="state in stateFilters"
              v-show="state === 'all' || stateCounts[state]"
              :key="state"
              class="auto-filter"
              :class="[state, { active: filter === state }]"
              :aria-pressed="filter === state"
              @click="
                filter = state;
                changeFilter()
              "
            >
              <i v-if="state !== 'all'" />{{
                state === 'all' ? '全部' : recognitionStates[state]
              }}<span>{{ stateCounts[state] || 0 }}</span>
            </button>
          </div>
        </div>
        <div class="auto-grid">
          <section class="pane auto-sources" aria-label="添加的来源">
            <header class="pane-head">
              <h2>来源</h2>
              <button
                class="text-button"
                title="丢弃当前工作区，重新选择文件或目录"
                :disabled="locked || loading"
                @click="openBrowser(true)"
              >
                新建识别
              </button>
            </header>
            <div class="auto-source-list">
              <button
                class="auto-source"
                :class="{ active: !source }"
                :aria-pressed="!source"
                @click="chooseSource('')"
              >
                <span class="auto-source-name"
                  ><Layers3 :size="15" /><strong>全部来源</strong
                  ><em>{{ readyCount + pendingCount }}</em></span
                ><span class="muted tiny"
                  >{{ groups.length }} 部作品 · {{ sources.length }} 个来源</span
                >
              </button>
              <div
                v-for="item in sources"
                :key="item.path"
                class="auto-source-item"
              >
                <button
                  class="auto-source"
                  :class="{ active: source === item.path }"
                  :aria-pressed="source === item.path"
                  :title="item.path"
                  @click="chooseSource(item.path)"
                >
                  <span class="auto-source-name"
                    ><FolderOpen :size="15" /><strong>{{
                      name(item.path) || item.path
                    }}</strong
                    ><em>{{ item.files }}</em></span
                  ><span
                    v-if="item.files"
                    class="auto-source-bar"
                    aria-hidden="true"
                    ><i
                      v-for="state in fileStates"
                      v-show="item.states[state]"
                      :key="state"
                      :class="state"
                      :style="{ flexGrow: item.states[state] }"
                  /></span>
                  <span v-if="item.files" class="auto-source-states tiny"
                    ><template v-for="state in fileStates" :key="state"
                      ><span v-if="item.states[state]" :class="state"
                        >{{ recognitionStates[state] }}
                        {{ item.states[state] }}</span
                      ></template
                    ></span
                  ><span v-else class="muted tiny">{{
                    recognizing ? '扫描中…' : '没有找到媒体文件'
                  }}</span>
                </button>
                <button
                  class="auto-source-remove"
                  :disabled="locked"
                  :aria-label="`移除来源 ${item.path}`"
                  title="移除这个来源"
                  @click="removeSource(item.path)"
                >
                  <Trash2 :size="14" />
                </button>
              </div>
            </div>
            <button
              class="auto-source-add"
              aria-label="添加文件或目录"
              title="添加文件或目录"
              :disabled="locked || loading"
              @click="openBrowser()"
            >
              <Plus :size="15" /><span>添加文件或目录</span>
            </button>
          </section>
          <section class="pane auto-groups" aria-label="作品分组">
            <header class="pane-head">
              <h2>
                {{ source ? name(source) : '作品' }}
                <span class="count">{{ filtered.length }}</span>
              </h2>
            </header>
            <div class="auto-group-list">
              <div
                v-for="group in visibleGroups"
                :key="group.id"
                class="auto-group"
                :class="{ active: current?.id === group.id }"
              >
                <button
                  class="auto-group-main"
                  :aria-current="current?.id === group.id"
                  @click="chooseGroup(group.id)"
                >
                  <span class="auto-poster">
                    <img
                      v-if="group.entry?.posterPath"
                      :src="poster(group.entry.posterPath)"
                      alt=""
                      loading="lazy"
                    /><component
                      :is="group.entry?.type === 'tv' ? Tv : Film"
                      v-else
                      :size="16"
                    />
                  </span>
                  <span class="auto-group-text">
                    <strong>{{ group.title }}</strong>
                    <span class="muted"
                      >{{ workYear(group) }} · {{ workType(group) }} ·
                      {{ group.files.length }} 个文件</span
                    >
                    <span class="auto-state" :class="group.state"
                      ><i />{{ recognitionStates[group.state] }}</span
                    >
                  </span>
                </button>
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
              <header class="auto-work">
                <span class="auto-poster large">
                  <img
                    v-if="current.entry?.posterPath"
                    :src="poster(current.entry.posterPath)"
                    alt=""
                  /><component
                    :is="current.entry?.type === 'tv' ? Tv : Film"
                    v-else
                    :size="22"
                  />
                </span>
                <div class="auto-work-text">
                  <span class="auto-state" :class="current.state"
                    ><i />{{ recognitionStates[current.state] }}</span
                  >
                  <h2>{{ current.title }}</h2>
                  <p class="muted">
                    <template v-if="current.entry"
                      >{{
                        current.entry.originalTitle &&
                        current.entry.originalTitle !== current.entry.title
                          ? `${current.entry.originalTitle} · `
                          : ''
                      }}{{ workYear(current) }} · {{ workType(current) }} ·
                      TMDB {{ current.entry.tmdbId }}</template
                    ><template v-else
                      >尚未确定作品，选择候选或手动搜索</template
                    >
                  </p>
                  <p v-if="groupSeasonsError" class="danger tiny" role="alert">{{ groupSeasonsError }}</p>
                </div>
                <div class="auto-work-actions">
                  <div v-if="current.entry?.type === 'tv'" class="auto-season-controls">
                    <span v-if="savingSeasonGroup === current.id" class="muted tiny" role="status"><LoaderCircle class="spin" :size="13" />校验季集…</span>
                    <Select
                      :model-value="groupSeason === -1 ? undefined : String(groupSeason)"
                      label="选择作品季度"
                      :placeholder="groupSeasonsLoading ? '正在读取季度…' : '暂无可选季度'"
                      :options="groupSeasons.map(season => ({ value: String(season.number), label: `${season.name} · ${season.count} 集` }))"
                      :disabled="locked || groupSeasonsLoading || !groupSeasons.length"
                      @update:model-value="(value) => { groupSeason = Number(value); setSeason() }"
                    />
                  </div>
                <button
                  class="button small"
                  :class="{ primary: !current.entry }"
                  :disabled="locked"
                  @click="openSearch"
                >
                  <Search :size="14" />{{
                    current.entry ? '修改作品' : '选择作品'
                  }}
                </button>
                </div>
              </header>
              <div class="auto-files">
                <article
                  v-for="file in visibleFiles"
                  :key="file.path"
                  class="auto-file"
                  :class="{ off: file.state !== 'ready' }"
                >
                  <input
                    type="checkbox"
                    :aria-label="`选择文件 ${name(file.path)}`"
                    :checked="file.selected"
                    :disabled="locked || file.state !== 'ready'"
                    @change="selectFile(file)"
                  />
                  <div class="auto-file-main">
                    <div class="auto-file-title">
                      <span
                        v-if="file.entry?.type === 'tv'"
                        class="auto-episode"
                        :class="{ unknown: file.season < 0 }"
                        >{{
                          file.season < 0
                            ? '季号未知'
                            : episodeCode(file.season, file.episodes)
                        }}</span
                      ><strong :title="file.path">{{ name(file.path) }}</strong>
                    </div>
                    <p class="auto-file-meta tiny">
                      <span class="auto-state" :class="file.state"
                        ><i />{{ recognitionStates[file.state] }}</span
                      ><span class="auto-path" :title="file.path">{{
                        folderOf(file.path)
                      }}</span
                      ><span>{{ bytes(file.size) }}</span>
                    </p>
                    <p class="auto-reason tiny">
                      {{ file.reason || '等待识别' }}
                    </p>
                  </div>
                  <div class="auto-file-actions">
                    <RouterLink
                      v-if="file.traceId"
                      class="icon-button"
                      :to="{
                        name: 'extraction-history',
                        params: { traceId: file.traceId },
                      }"
                      aria-label="查看识别过程"
                      title="查看识别过程"
                      ><ScrollText :size="15"
                    /></RouterLink>
                    <button
                      v-if="file.entry?.type === 'tv'"
                      class="icon-button"
                      :disabled="locked"
                      aria-label="修正季集"
                      title="修正季集"
                      @click="openEdit(file)"
                    >
                      <Pencil :size="15" /></button
                    ><button
                      class="icon-button"
                      :disabled="locked"
                      aria-label="调整分组"
                      title="调整分组"
                      @click="openTransfer(file)"
                    >
                      <ArrowLeftRight :size="15" /></button
                    ><button
                      class="icon-button danger-hover"
                      :disabled="locked"
                      aria-label="移出工作区"
                      title="移出工作区"
                      @click="removeFile(file)"
                    >
                      <X :size="15" />
                    </button>
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
        <footer class="auto-footer">
          <div class="auto-footer-summary">
            <strong
              >已选 <b>{{ selected.length }}</b> / {{ readyCount }} 个可整理文件</strong
            >
            <p class="muted tiny">
              <template v-if="pendingCount"
                >{{ pendingCount }} 个文件待处理，可以稍后再整理 · </template
              >每部作品分别记录
            </p>
          </div>
          <div class="auto-footer-select">
            <button
              class="text-button"
              :disabled="locked"
              @click="selectReady(true)"
            >
              全选可整理</button
            ><button
              class="text-button"
              :disabled="locked || !selected.length"
              @click="selectReady(false)"
            >
              清空
            </button>
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
              v-if="busy && !savingSeasonGroup"
              class="spin"
              :size="16"
            />统一预览<ArrowRight :size="16" />
          </button>
        </footer>
      </template>
    </template>
  </section>
  <FileBrowser
    v-if="browser"
    confirm-label="开始识别"
    @close="browser = false"
    @select="start"
  />
  <Modal v-if="searchGroup" title="选择对应作品" @close="closeSearch">
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
        <div class="result-poster">
          <img
            v-if="entry.posterPath"
            :src="`https://image.tmdb.org/t/p/w185${entry.posterPath}`"
            alt=""
          /><component :is="entry.type === 'tv' ? Tv : Film" v-else :size="22" />
        </div>
        <div>
          <h3>
            {{ entry.title }}<span>{{ entry.year || '年份未知' }}</span>
          </h3>
          <p class="tiny muted">
            {{
              entry.originalTitle && entry.originalTitle !== entry.title
                ? `${entry.originalTitle} · `
                : ''
            }}{{ entry.type === 'tv' ? '剧集' : '电影' }} · TMDB
            {{ entry.tmdbId }}
          </p>
          <p class="result-overview">{{ entry.overview || '暂无简介' }}</p>
        </div>
        <Check :size="17" />
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
  <Modal v-if="editing" title="修正季集" @close="closeEdit"
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
    <div v-if="previewIssues.length" class="notice error">
      <p v-for="[issue, count] in previewIssues" :key="issue">
        {{ issue }}<b v-if="count > 1" class="auto-issue-count"> × {{ count }}</b>
      </p>
      <p>关闭预览，调整选中文件或分组后重新预览。</p>
    </div>
    <div class="auto-preview-rows">
      <article v-for="row in previewRows" :key="row.source">
        <strong>{{ name(row.source) }}</strong>
        <p class="muted tiny auto-path">{{ row.source }}</p>
        <p class="auto-path auto-preview-target" :class="{ 'auto-row-issue': conflictTargets([...row.issues, ...preview.issues]).includes(row.target) }">
          <span>→ {{ row.target }}</span>
          <Tooltip v-if="conflictTargets([...row.issues, ...preview.issues]).includes(row.target)" content="已存在">
            <CircleAlert :size="16" tabindex="0" role="img" aria-label="已存在" />
          </Tooltip>
        </p>
        <p v-for="target in conflictTargets(row.issues).filter(target => target !== row.target)" :key="target" class="auto-path auto-preview-target auto-row-issue">
          <span>→ {{ target }}</span>
          <Tooltip content="已存在">
            <CircleAlert :size="16" tabindex="0" role="img" aria-label="已存在" />
          </Tooltip>
        </p>
        <p v-if="row.artifacts.length" class="muted tiny">
          {{ row.artifacts.join(' · ') }}
        </p>
        <p
          v-for="issue in new Set(row.issues.filter(issue => !conflictPath(issue)))"
          :key="issue"
          class="auto-row-issue tiny"
        >
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
