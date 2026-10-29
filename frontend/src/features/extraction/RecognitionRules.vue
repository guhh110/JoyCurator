<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import { keys } from '../../app/query'
import { useRuleLibrary } from './queries'
import { api } from '../../api'
import type { Rule } from './model'
import { targets, sources, moveRule, emptyScript } from './model'
import RuleEditor from './RuleEditor.vue'
import ExtractionResult from './ExtractionResult.vue'
import { useExtraction } from './useExtraction'
defineEmits<{ back: [] }>()
const libraryQuery = useRuleLibrary(),
  client = useQueryClient()
const library = libraryQuery.data
const rules = ref<Rule[]>([]),
  selected = ref(-1),
  error = ref(''),
  notice = ref('')
const dragIndex = ref(-1)
const editor = ref<HTMLDivElement>()
async function selectRule(index: number) {
  selected.value = index
  await nextTick()
  editor.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
const {
  path,
  root,
  busy: testing,
  result,
  error: testError,
  notice: testNotice,
  run,
  cancel,
} = useExtraction()
const exampleResults = ref<
  {
    ruleId: string
    path: string
    expected: unknown
    actual: unknown
    passed: boolean
  }[]
>([])
function drop(index: number) {
  move(dragIndex.value, index)
  dragIndex.value = -1
}
function removeRule(index: number) {
  rules.value.splice(index, 1)
  selected.value = -1
}
const editing = computed(() => rules.value[selected.value])
let initialized = false
watch(
  library,
  (value) => {
    if (value && !initialized) {
      rules.value = JSON.parse(JSON.stringify(value.rules))
      initialized = true
    }
  },
  { immediate: true },
)
const saveMutation = useMutation({
  mutationFn: () => api('/extraction/rules', { rules: rules.value }, 'PUT'),
})
const examplesMutation = useMutation({
  mutationFn: () =>
    api<typeof exampleResults.value>(
      '/extraction/examples',
      { rules: rules.value },
      'POST',
    ),
})
const busy = computed(
  () => saveMutation.isPending.value || examplesMutation.isPending.value,
)
function edit(value: Rule) {
  rules.value[selected.value] = value
  rules.value.sort((a, b) => a.order - b.order)
  selected.value = rules.value.findIndex((rule) => rule.id === value.id)
  notice.value = '已应用到编辑链，尚未保存'
  result.value = null
}
const templateId = ref('')
function add(kind: 'script' | 'regex', template = false) {
  const descriptor = template
    ? library.value?.catalog.find((item) => item.id === templateId.value)
    : undefined
  const rule: Rule = {
    id: `rule-${Date.now().toString(36)}`,
    name: descriptor?.name ?? (kind === 'script' ? '新脚本规则' : '新正则规则'),
    description: descriptor?.description,
    target: descriptor?.target ?? 'title',
    source: descriptor?.sources[0] ?? 'filename',
    order: Math.max(0, ...rules.value.map((rule) => rule.order)) + 10,
    enabled: true,
    kind,
    overwrite: false,
    config: descriptor
      ? JSON.parse(JSON.stringify(descriptor.config))
      : kind === 'script'
        ? { code: emptyScript, params: {} }
        : { pattern: '(?P<value>.+)', capture: 'value' },
    examples: [],
  }
  rules.value.push(rule)
  void selectRule(rules.value.length - 1)
}
function move(from: number, to: number) {
  const id = editing.value?.id
  moveRule(rules.value, from, to)
  selected.value = rules.value.findIndex((rule) => rule.id === id)
  result.value = null
}
function reset() {
  if (!library.value) return
  rules.value = JSON.parse(JSON.stringify(library.value.defaults))
  selected.value = -1
  notice.value = '已载入默认链，保存后生效'
  result.value = null
}
async function save() {
  error.value = ''
  try {
    await saveMutation.mutateAsync()
    await client.invalidateQueries({ queryKey: keys.ruleLibrary })
    if (library.value)
      rules.value = JSON.parse(JSON.stringify(library.value.rules))
    notice.value = '规则链已保存，新的提取调用立即生效'
  } catch (e) {
    error.value = (e as Error).message
  }
}
async function examples() {
  error.value = ''
  try {
    exampleResults.value = await examplesMutation.mutateAsync()
    if (!exampleResults.value.length)
      notice.value = '请先在规则编辑器添加测试样例'
  } catch (e) {
    error.value = (e as Error).message
  }
}
</script>
<template>
  <section class="rules-page" aria-label="识别规则页面">
    <PageHeader
      title="识别规则"
      eyebrow="PATH EXTRACTION RULES"
      description="每条规则提取一个字段，按顺序执行，先命中优先。"
      ><template #actions
        ><button class="button" @click="$emit('back')">
          返回识别工作区
        </button></template
      ></PageHeader
    >
    <QueryState
      :loading="libraryQuery.isPending.value"
      :error="libraryQuery.error.value"
      @retry="libraryQuery.refetch()"
    >
      <div class="toolbar">
        <button class="button" @click="add('regex')" :disabled="!library">
          新增正则规则</button
        ><button class="button" @click="add('script')" :disabled="!library">
          新增脚本规则</button
        ><button class="button" @click="reset" :disabled="!library">
          恢复默认链</button
        ><button
          class="button primary"
          :disabled="busy || !library"
          @click="save"
        >
          保存规则链
        </button>
      </div>
      <div class="toolbar">
        <select v-model="templateId" aria-label="脚本模板">
          <option value="">选择可编辑的脚本模板</option>
          <option
            v-for="item in library?.catalog"
            :key="item.id"
            :value="item.id"
          >
            {{ targets[item.target] }} · {{ item.name }}
          </option></select
        ><button
          class="button"
          :disabled="!templateId"
          @click="add('script', true)"
        >
          从模板添加
        </button>
      </div>
      <p v-if="error" class="notice error" role="alert">{{ error }}</p>
      <p v-if="notice" class="notice" role="status">{{ notice }}</p>
      <section class="pane">
        <header class="pane-head">
          <h2>
            执行顺序 <span class="count">{{ rules.length }}</span>
          </h2>
          <span class="tiny muted"
            >拖动排序，或使用 ↑ ↓ 按钮；保存时不允许重复顺序。</span
          >
        </header>
        <p v-if="!rules.length" class="padding muted">
          规则链为空，提取结果将全部未知。
        </p>
        <article
          v-for="(rule, index) in rules"
          :key="`${index}-${rule.id}`"
          class="rule-row"
          :class="{ selected: index === selected }"
          draggable="true"
          @dragstart="dragIndex = index"
          @dragover.prevent
          @drop.prevent="drop(index)"
          @dragend="dragIndex = -1"
        >
          <input
            v-model="rule.enabled"
            type="checkbox"
            :aria-label="`启用 ${rule.name}`"
          /><button class="rule-name" @click="selectRule(index)">
            <strong>{{ rule.order }} · {{ rule.name }}</strong
            ><span class="tiny muted"
              >{{ sources[rule.source] }} → {{ targets[rule.target] }} ·
              {{ rule.overwrite ? '允许覆盖' : '仅补全未知值' }}</span
            >
          </button>
          <div class="row-actions">
            <button
              class="button small"
              :aria-label="`上移 ${rule.name}`"
              :disabled="index === 0"
              @click="move(index, index - 1)"
            >
              ↑</button
            ><button
              class="button small"
              :aria-label="`下移 ${rule.name}`"
              :disabled="index === rules.length - 1"
              @click="move(index, index + 1)"
            >
              ↓</button
            ><button
              class="text-button red"
              :aria-label="`删除 ${rule.name}`"
              @click="removeRule(index)"
            >
              删除
            </button>
          </div>
        </article>
        <div v-if="editing && library" ref="editor">
          <RuleEditor
            :key="selected"
            :rule="editing"
            :source-scripts="library.sourceScripts"
            @update="edit"
          />
        </div>
      </section>
      <form class="pane trial" @submit.prevent="run(rules)">
        <h2>试跑当前编辑链</h2>
        <p class="tiny muted">先在编辑器应用修改；试跑不保存规则或历史轨迹。</p>
        <label for="trial-path">完整文件路径</label
        ><input
          id="trial-path"
          v-model="path"
          required
          placeholder="/source/The.Matrix.1999.tmdb-603.mkv"
        /><label for="trial-root">解析边界（可选）</label
        ><input
          id="trial-root"
          v-model="root"
          placeholder="留空按源目录自动确定"
        />
        <div class="toolbar">
          <button class="button primary" :disabled="testing">试跑路径</button
          ><button v-if="testing" class="button" type="button" @click="cancel">
            取消</button
          ><button
            class="button"
            type="button"
            :disabled="busy || testing"
            @click="examples"
          >
            运行所有样例
          </button>
        </div>
      </form>
      <p v-if="testError" class="notice error" role="alert">{{ testError }}</p>
      <p v-if="testNotice" class="notice">{{ testNotice }}</p>
      <section v-if="exampleResults.length" class="pane padding">
        <h2>样例测试</h2>
        <p
          v-for="(item, index) in exampleResults"
          :key="index"
          class="sample"
          :class="{ red: !item.passed }"
        >
          {{ item.passed ? '通过' : '失败' }} · {{ item.ruleId }} ·
          {{ item.path }}<br />预期 {{ JSON.stringify(item.expected) }}，实际
          {{ JSON.stringify(item.actual) }}
        </p>
      </section>
      <ExtractionResult v-if="result" :result="result" />
    </QueryState>
  </section>
</template>
<style scoped>
.rules-page {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.heading,
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.heading {
  justify-content: space-between;
}
.heading h1 {
  margin: 8px 0;
}
h2 {
  font-size: 17px;
  margin: 0;
}
.pane-head {
  height: auto;
  min-height: 60px;
  gap: 12px;
  flex-wrap: wrap;
}
.rule-row {
  padding: 14px 20px;
  display: flex;
  gap: 12px;
  align-items: center;
  border-top: 1px solid var(--line);
}
.rule-row.selected {
  background: #f6f3ff;
}
.rule-row > input {
  width: auto;
}
.rule-name {
  flex: 1;
  text-align: left;
  border: 0;
  background: none;
  cursor: pointer;
  min-width: 0;
  display: grid;
  gap: 7px;
  color: inherit;
}
.rule-name strong {
  font-size: 13px;
  overflow-wrap: anywhere;
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.trial,
.padding {
  padding: 20px;
}
.trial label {
  margin-top: 16px;
}
.trial .toolbar {
  margin-top: 20px;
}
.sample {
  font-size: 13px;
  overflow-wrap: anywhere;
  line-height: 1.8;
}
@media (max-width: 600px) {
  .heading {
    align-items: flex-start;
  }
  .rule-row {
    padding: 14px 12px;
    flex-wrap: wrap;
  }
  .row-actions {
    margin-left: auto;
  }
  .rule-name {
    min-width: 180px;
  }
  .toolbar .button {
    flex: 1;
  }
  .toolbar {
    gap: 8px;
  }
}
</style>
