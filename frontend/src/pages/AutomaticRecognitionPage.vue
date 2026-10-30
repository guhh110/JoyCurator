<script setup lang="ts">
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
  Trash2,
} from '@lucide/vue'
import { RouterLink } from 'vue-router'
import { bytes, episodeCode, name } from '../api'
import { recognitionStates } from '../recognition'
import FileBrowser from '../components/FileBrowser.vue'
import Modal from '../components/Modal.vue'
import QueryState from '../components/ui/QueryState.vue'
import PageHeader from '../components/ui/PageHeader.vue'
import { useAutomaticRecognition } from '../features/automatic/useAutomaticRecognition'
const {
  runtime,
  busy,
  error,
  notice,
  batch,
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
  runStates,
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
  cancelBatch,
  chooseGroup,
  openTransfer,
  closeSearch,
  closeEdit,
  changeFilter,
  source,
  sources,
  chooseSource,
  removeSource,
} = useAutomaticRecognition()
</script>

<template>
  <section class="auto-workspace" aria-label="自动识别">
    <PageHeader
      class="workflow-heading"
      title="自动识别"
      eyebrow="DISCOVER EVERY STORY"
      description="电影与剧集一起添加，按作品检查，再统一整理。"
      ><template #actions>
        <div class="auto-heading-actions">
          <RouterLink class="text-button" :to="{ name: 'recognition-rules' }"
            >识别规则</RouterLink
          >
          <RouterLink class="text-button" :to="{ name: 'extraction-history' }"
            >执行历史</RouterLink
          >
          <RouterLink class="text-button" :to="{ name: 'recognition-debug' }"
            >识别调试</RouterLink
          >
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
      </template></PageHeader
    >
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
          <section class="pane auto-sources" aria-label="添加的来源">
            <header class="pane-head">
              <h2>来源</h2>
              <span class="count">{{ sources.length }}</span>
            </header>
            <div class="auto-source-list">
              <button
                class="auto-source"
                :class="{ active: !source }"
                :aria-pressed="!source"
                @click="chooseSource('')"
              >
                <strong>全部来源</strong
                ><span class="muted tiny"
                  >{{ readyCount + pendingCount }} 个文件 ·
                  {{ groups.length }} 部作品</span
                >
              </button>
              <div
                v-for="item in sources"
                :key="item.path"
                class="auto-source-item"
              ><button
                class="auto-source"
                :class="{ active: source === item.path }"
                :aria-pressed="source === item.path"
                :title="item.path"
                @click="chooseSource(item.path)"
              >
                <span class="auto-source-name"
                  ><FolderOpen :size="15" /><strong>{{
                    name(item.path) || item.path
                  }}</strong></span
                ><span class="auto-source-path muted tiny">{{
                  item.path
                }}</span
                ><span class="muted tiny"
                  >{{ item.files }} 个文件 · {{ item.works }} 部作品</span
                ><span
                  v-if="item.files"
                  class="auto-source-bar"
                  aria-hidden="true"
                  ><i
                    v-for="state in [
                      'ready',
                      'review',
                      'unmatched',
                      'failed',
                      'pending',
                    ]"
                    v-show="item.states[state]"
                    :key="state"
                    :class="state"
                    :style="{ flexGrow: item.states[state] }"
                /></span
                ><span v-if="item.files" class="auto-source-states tiny"
                  ><template
                    v-for="state in [
                      'ready',
                      'review',
                      'unmatched',
                      'failed',
                      'pending',
                    ]"
                    :key="state"
                    ><span v-if="item.states[state]" :class="state"
                      >{{ recognitionStates[state] }}
                      {{ item.states[state] }}</span
                    ></template
                  ></span
                ><span v-else class="muted tiny">{{
                  recognizing ? '扫描中…' : '没有找到媒体文件'
                }}</span>
              </button><button
                class="auto-source-remove"
                :disabled="locked"
                :aria-label="`移除来源 ${item.path}`"
                title="移除这个来源"
                @click="removeSource(item.path)"
              >
                <Trash2 :size="14" />
              </button></div>
            </div>
            <button
              class="auto-source-add"
              :disabled="locked || loading"
              @click="openBrowser()"
            >
              <Plus :size="15" />添加文件或目录
            </button>
          </section>
          <section class="pane auto-groups" aria-label="作品分组">
            <header class="pane-head">
              <h2>{{ source ? '本来源的作品' : '作品分组' }}</h2>
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
                      group.files[0]?.metadata.year ||
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
                      <RouterLink
                        v-if="file.traceId"
                        class="text-button"
                        :to="{
                          name: 'extraction-history',
                          params: { traceId: file.traceId },
                        }"
                        >查看识别过程</RouterLink
                      ><span v-else class="tiny muted">无规则执行轨迹</span>
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
            @click="runtime.inspect(run)"
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
