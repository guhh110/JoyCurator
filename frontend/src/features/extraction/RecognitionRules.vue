<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../../api'
import type { Rule, RuleLibrary } from './model'
import { targets, sources, moveRule } from './model'
import RuleEditor from './RuleEditor.vue'
import ExtractionResult from './ExtractionResult.vue'
import { useExtraction } from './useExtraction'
defineEmits<{ back: [] }>()
const library = ref<RuleLibrary>(),
  rules = ref<Rule[]>([]),
  selected = ref(-1),
  error = ref(''),
  notice = ref(''),
  busy = ref(false)
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
onMounted(async () => {
  try {
    library.value = await api<RuleLibrary>('/extraction/rules')
    rules.value = library.value.rules
  } catch (e) {
    error.value = (e as Error).message
  }
})
function edit(value: Rule) {
  rules.value[selected.value] = value
  rules.value.sort((a, b) => a.order - b.order)
  selected.value = rules.value.findIndex((rule) => rule.id === value.id)
  notice.value = '已应用到编辑链，尚未保存'
  result.value = null
}
function add(kind: 'builtin' | 'regex') {
  const descriptor = library.value?.catalog[0]
  const rule: Rule = {
    id: `rule-${Date.now().toString(36)}`,
    name: kind === 'builtin' ? (descriptor?.name ?? '内置规则') : '新正则规则',
    target: kind === 'builtin' ? (descriptor?.target ?? 'title') : 'title',
    source: 'filename',
    order: Math.max(0, ...rules.value.map((rule) => rule.order)) + 10,
    enabled: true,
    kind,
    overwrite: false,
    config:
      kind === 'builtin'
        ? { handler: descriptor?.handler, params: descriptor?.params }
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
  busy.value = true
  error.value = ''
  try {
    await api('/extraction/rules', { rules: rules.value }, 'PUT')
    notice.value = '规则链已保存，新的提取调用立即生效'
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
async function examples() {
  busy.value = true
  error.value = ''
  try {
    exampleResults.value = await api(
      '/extraction/examples',
      { rules: rules.value },
      'POST',
    )
    if (!exampleResults.value.length)
      notice.value = '请先在规则编辑器添加测试样例'
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <section class="rules-page" aria-label="识别规则页面">
    <header class="heading">
      <div>
        <span class="eyebrow">PATH EXTRACTION RULES</span>
        <h1>识别规则<span class="heading-dot">.</span></h1>
        <p class="muted">每条规则提取一个字段，按顺序执行，先命中优先。</p>
      </div>
      <button class="button" @click="$emit('back')">返回自动识别</button>
    </header>
    <div class="toolbar">
      <button class="button" @click="add('regex')" :disabled="!library">
        新增正则规则</button
      ><button class="button" @click="add('builtin')" :disabled="!library">
        添加内置规则</button
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
          :catalog="library.catalog"
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
        {{ item.passed ? '通过' : '失败' }} · {{ item.ruleId }} · {{ item.path
        }}<br />预期 {{ JSON.stringify(item.expected) }}，实际
        {{ JSON.stringify(item.actual) }}
      </p>
    </section>
    <ExtractionResult v-if="result" :result="result" />
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
