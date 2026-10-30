<script setup lang="ts">
import {
  ArrowRight,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleCheck,
  Copy,
  FileCheck2,
  Film,
  FolderOpen,
  Layers3,
  Link2,
  LoaderCircle,
  Pencil,
  Plus,
  Search,
  ShieldCheck,
  X,
} from '@lucide/vue'
import { bytes, episodeCode, name } from '../api'
import Modal from '../components/Modal.vue'
import FileBrowser from '../components/FileBrowser.vue'
import Notices from '../components/ui/Notices.vue'
import PageHeader from '../components/ui/PageHeader.vue'
import { modeNames } from '../features/records/presentation'
import { useWorkbench } from '../features/workbench/useWorkbench'
const {
  files,
  entry,
  busy,
  message,
  success,
  running,
  settings,
  season,
  seasons,
  episodes,
  loadingEpisodes,
  filter,
  filePage,
  panel,
  mode,
  browser,
  searchOpen,
  query,
  kind,
  year,
  searching,
  searched,
  results,
  searchError,
  editing,
  editSeason,
  editEpisodes,
  editError,
  preview,
  previewFilter,
  previewPage,
  omitted,
  filtered,
  visibleFiles,
  filePages,
  mapping,
  ready,
  unmatched,
  matchedCount,
  previewRows,
  shownPreview,
  step,
  add,
  remove,
  assigned,
  search,
  pick,
  changeSeason,
  edit,
  saveEdit,
  prepare,
  skip,
  execute,
  newPreview,
} = useWorkbench()
</script>
<template>
  <div class="workbench-page">
    <Notices :error="message" :success="success" />

    <PageHeader
      class="workflow-heading"
      title="整理工作台"
      eyebrow="YOUR MEDIA, BEAUTIFULLY ORDERED"
      ><template #actions>
        <div class="steps" aria-label="整理步骤">
          <span :class="{ current: step === 1, done: step > 1 }"
            ><i>{{ step > 1 ? '✓' : '1' }}</i
            >添加文件</span
          ><b /><span :class="{ current: step === 2, done: step > 2 }"
            ><i>{{ step > 2 ? '✓' : '2' }}</i
            >选择作品</span
          ><b /><span :class="{ current: step === 3 }"><i>3</i>预览整理</span>
        </div>
      </template></PageHeader
    >
    <div class="mobile-switch">
      <button :class="{ active: panel === 'files' }" @click="panel = 'files'">
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
            <button class="text-button" @click="filter = ''">清空筛选</button>
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
                  >{{ episodeCode(mapping(file).season, file.episodes) }}</span
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
                <option v-for="s in seasons" :key="s.number" :value="s.number">
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
                /><span
                  v-else
                  class="unmatched-dot"
                  role="img"
                  aria-label="未匹配"
                />
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
  </div>
  <FileBrowser v-if="browser" @close="browser = false" @select="add" />
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
</template>
<style scoped>
.workbench-page {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
  height: 100%;
}
</style>
