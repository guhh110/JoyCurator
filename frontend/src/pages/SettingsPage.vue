<script setup lang="ts">
import { VueDraggable } from 'vue-draggable-plus'
import {
  ArrowDownToLine,
  Check,
  ChevronDown,
  Copy,
  Film,
  Folder,
  FolderOpen,
  GripVertical,
  Layers3,
  Plus,
  ShieldCheck,
  Trash2,
} from '@lucide/vue'
import FileBrowser from '../components/FileBrowser.vue'
import QueryState from '../components/ui/QueryState.vue'
import PageHeader from '../components/ui/PageHeader.vue'
import Select from '../components/ui/Select.vue'
import EliText from '../components/ui/EliText.vue'
import { useSettingsForm } from '../features/settings/useSettingsForm'
const {
  settings,
  token,
  clearToken,
  directoryTarget,
  settingsSection,
  running,
  busy,
  categoryDragging,
  categoryKey,
  moveCategory,
  finishCategorySort,
  loadSettings,
  saveSettings,
  chooseDirectory,
  parts,
  settingsQuery,
} = useSettingsForm()
</script>
<template>
  <div>
    <QueryState
      :loading="settingsQuery.isPending.value"
      :error="settingsQuery.error.value"
      @retry="settingsQuery.refetch()"
      ><template v-if="settings">
        <PageHeader
          title="偏好设置"
          eyebrow="MAKE IT YOURS"
          description="配置目录、TMDB、命名与分类。"
          ><template #actions>
            <button
              class="button primary"
              :disabled="busy || running"
              @click="saveSettings"
            >
              <Check :size="16" />保存设置
            </button>
          </template></PageHeader
        >
        <div
          class="settings-layout"
          :class="{ 'rules-layout': settingsSection === 'rules' }"
        >
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
            >
          </nav>
          <section
            class="pane settings-content"
            :class="{ 'rules-content': settingsSection === 'rules' }"
          >
            <template v-if="settingsSection === 'folders'"
              ><h2>让每份收藏各归其位</h2>
              <p class="muted">
                源目录用于添加文件，目标目录保存整理结果。电影、剧集等子目录由分类规则生成。
              </p>
              <label
                >源目录
                <div class="input-action">
                  <input v-model="settings.sourceRoot" /><button
                    class="button small"
                    @click="directoryTarget = 'sourceRoot'"
                  >
                    <FolderOpen :size="16" />选择
                  </button>
                </div></label
              ><label
                >目标目录
                <div class="input-action">
                  <input v-model="settings.targetRoot" /><button
                    class="button small"
                    @click="directoryTarget = 'targetRoot'"
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
                    ><EliText :text="root" ellipsis="start" /></code
                  ><small
                    >通过 JOYCURATOR_SOURCE_DIR 和 JOYCURATOR_TARGET_DIR
                    配置授权范围。</small
                  >
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
                >元数据语言<Select v-model="settings.language" label="元数据语言" :options="[{ value: 'zh-CN', label: '简体中文' }, { value: 'zh-TW', label: '繁體中文' }, { value: 'en-US', label: 'English' }, { value: 'ja-JP', label: '日本語' }]" /></label
              ></template
            >
            <template v-else-if="settingsSection === 'rules'"
              ><h2>熟悉的名字，整齐的目录</h2>
              <p class="muted">
                分类按顺序取首条命中的规则。模板缺少字段会在预览中提示。
              </p>
              <div class="rules-columns">
                <section class="naming-panel" aria-labelledby="naming-title">
                  <h3 id="naming-title">命名模板</h3>
                  <label
                    >电影目录模板<input
                      v-model="settings.rules.movieFolder" /></label
                  ><label
                    >电影文件名<input
                      v-model="settings.rules.movieFile" /></label
                  ><label
                    >剧集目录模板<input
                      v-model="settings.rules.tvFolder" /></label
                  ><label
                    >剧集文件名<input v-model="settings.rules.tvFile"
                  /></label>
                  <p class="template-fields">
                    可用字段：<code>{title}</code><code>{year}</code
                    ><code>{category}</code><code>{tmdb_id}</code
                    ><code>{season:02}</code><code>{episode_range}</code
                    ><code>{resolution}</code><code>{codec}</code
                    ><code>{original_stem}</code><code>{ext}</code>
                  </p>
                  <p class="rules-help">
                    目录模板相对于目标目录，<code>{category}</code>
                    由右侧分类规则生成。清晰度和编码仅从源文件名标签提取，不检测实际媒体流；自定义模板引用缺失字段时预览会提示。
                  </p>
                </section>
                <section
                  class="category-panel"
                  aria-labelledby="categories-title"
                >
                  <div class="section-heading">
                    <h3 id="categories-title">
                      分类规则
                      <span class="rule-count">{{
                        settings.rules.categories.length
                      }}</span>
                    </h3>
                    <button
                      class="button small"
                      :disabled="busy || running || categoryDragging"
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
                  <p class="rules-help" id="category-order-help">
                    从上到下匹配，首个命中生效。子目录支持
                    <code>电影/华语电影</code>、<code>电视剧/国产剧</code>。
                  </p>
                  <VueDraggable
                    v-model="settings.rules.categories"
                    class="category-list"
                    handle=".rule-handle"
                    draggable=".category-rule"
                    :animation="160"
                    :disabled="busy || running"
                    :delay="120"
                    :delay-on-touch-only="true"
                    :touch-start-threshold="5"
                    :scroll-sensitivity="48"
                    :scroll-speed="12"
                    ghost-class="rule-ghost"
                    chosen-class="rule-chosen"
                    @start="categoryDragging = true"
                    @end="finishCategorySort"
                  >
                    <details
                      v-for="(category, index) in settings.rules.categories"
                      :key="categoryKey(category)"
                      class="category-rule"
                    >
                      <summary>
                        <button
                          type="button"
                          class="rule-handle"
                          :aria-label="`拖动排序 ${category.name || '未命名分类'}，第 ${index + 1} 条`"
                          aria-describedby="category-order-help"
                          title="拖动排序，或按 ↑ ↓ 调整"
                          :disabled="busy || running"
                          @click.stop.prevent
                          @keydown.up.prevent="moveCategory(index, -1, $event)"
                          @keydown.down.prevent="moveCategory(index, 1, $event)"
                        >
                          <GripVertical :size="16" />
                        </button>
                        <span class="rule-index">{{
                          String(index + 1).padStart(2, '0')
                        }}</span
                        ><EliText class="rule-name" :text="category.name || '未命名分类'" ellipsis="start" />
                        <span class="rule-kind">{{
                          category.type === 'movie'
                            ? '电影'
                            : category.type === 'tv'
                              ? '电视剧'
                              : '全部'
                        }}</span>
                        <ChevronDown :size="15" />
                      </summary>
                      <div class="category-fields">
                        <label
                          >分类子目录<input
                            v-model="category.name"
                            placeholder="电影/华语电影" /></label
                        ><label
                          >媒体类型<Select :model-value="category.type || ''" label="媒体类型" :options="[{ value: '', label: '全部' }, { value: 'movie', label: '电影' }, { value: 'tv', label: '剧集' }]" @update:model-value="value => category.type = value" /></label
                        ><label
                          >国家/地区代码<input
                            :value="category.countries.join(',')"
                            placeholder="CN,HK,TW"
                            @change="
                              category.countries = parts($event)
                            " /></label
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
                            :disabled="
                              index === 0 || busy || running || categoryDragging
                            "
                            @click="moveCategory(index, -1, $event)"
                          >
                            上移
                          </button>
                          <button
                            class="button small"
                            :disabled="
                              index === settings.rules.categories.length - 1 ||
                              busy ||
                              running ||
                              categoryDragging
                            "
                            @click="moveCategory(index, 1, $event)"
                          >
                            下移</button
                          ><button
                            class="button small danger"
                            :disabled="busy || running || categoryDragging"
                            @click="settings.rules.categories.splice(index, 1)"
                          >
                            <Trash2 :size="13" />移除
                          </button>
                        </div>
                      </div>
                    </details>
                  </VueDraggable>
                  <p class="rule-order-status">
                    拖动左侧手柄调整优先级，保存设置后生效。
                  </p>
                </section>
              </div></template
            >
            <template v-else><p>整理只处理视频及同名字幕。刮削在独立工作区进行。</p></template>
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
          </button></div></template
    ></QueryState>
  </div>
  <FileBrowser
    v-if="directoryTarget"
    directory
    @close="directoryTarget = null"
    @select="chooseDirectory"
  />
</template>
