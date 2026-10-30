<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { MoreHorizontal, Plus, Save } from '@lucide/vue'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import { keys } from '../../app/query'
import { useRuleLibrary } from './queries'
import { api } from '../../api'
import type { Rule, Target } from './model'
import { fieldOrder, targets } from './model'
import RuleFieldTable from './RuleFieldTable.vue'
import { useRuleDraft } from './useRuleDraft'

// Rule list page: manages the chain (enable, order inside a field, add, delete,
// defaults, examples). Editing a rule happens on its own page.
defineEmits<{ back: [] }>()
const router = useRouter()
const libraryQuery = useRuleLibrary(),
  client = useQueryClient()
const library = libraryQuery.data
const { drafts, dirty, validation, load, move, remove, markSaved } =
  useRuleDraft()
const error = ref(''),
  notice = ref('')
let initialized = false
watch(
  library,
  (value) => {
    if (value && !initialized) {
      if (load(value.rules, value.sourceScripts))
        notice.value = '已按字段顺序整理执行顺序，保存后生效'
      initialized = true
    }
  },
  { immediate: true },
)
const validationErrors = computed(() =>
  validation.value.errors.map((item) => item.key),
)
const saveMutation = useMutation({
  mutationFn: (rules: Rule[]) => api('/extraction/rules', { rules }, 'PUT'),
})
const examplesMutation = useMutation({
  mutationFn: (rules: Rule[]) =>
    api<
      {
        ruleId: string
        path: string
        expected: unknown
        actual: unknown
        passed: boolean
      }[]
    >('/extraction/examples', { rules }, 'POST'),
})
const saving = saveMutation.isPending
const canSubmit = computed(
  () => !!library.value && !validation.value.errors.length && !saving.value,
)
const exampleResults = ref<
  NonNullable<typeof examplesMutation.data.value>
>([])
const failedExamples = computed(() =>
  exampleResults.value.filter((item) => !item.passed),
)
function ruleName(id: string) {
  return drafts.value.find((item) => item.rule.id === id)?.rule.name ?? id
}

// Leaving with unsaved list changes would drop them; ask first.
onBeforeRouteLeave(
  () =>
    !dirty.value ||
    window.confirm('规则列表有未保存的修改，离开后会丢失。继续吗？'),
)
function open(key: string) {
  const draft = drafts.value.find((item) => item.key === key)
  if (!draft) return
  void router.push({
    name: 'recognition-rule-edit',
    params: { ruleId: draft.rule.id },
  })
}
function add(target: Target, kind: 'script' | 'regex', templateId?: string) {
  void router.push({
    name: 'recognition-rule-edit',
    params: { ruleId: 'new' },
    query: { target, kind, ...(templateId ? { template: templateId } : {}) },
  })
}
function reset() {
  if (!library.value) return
  load(library.value.defaults, library.value.sourceScripts, false)
  exampleResults.value = []
  notice.value = '默认规则已载入，保存后生效'
}
async function save() {
  if (!canSubmit.value) return
  error.value = ''
  notice.value = ''
  const rules = JSON.parse(JSON.stringify(validation.value.rules)) as Rule[]
  try {
    await saveMutation.mutateAsync(rules)
    markSaved()
    client.setQueryData(keys.ruleLibrary, { ...library.value!, rules })
    notice.value = '规则链已保存并生效'
    void client.invalidateQueries({ queryKey: keys.ruleLibrary })
  } catch (e) {
    error.value = (e as Error).message
  }
}
async function examples() {
  if (!canSubmit.value) return
  error.value = ''
  try {
    exampleResults.value = await examplesMutation.mutateAsync(
      validation.value.rules,
    )
  } catch (e) {
    error.value = (e as Error).message
  }
}
function closeMenu(event: Event) {
  ;(event.target as HTMLElement).closest('details')?.removeAttribute('open')
}
</script>
<template>
  <section class="rules-page" aria-label="识别规则页面">
    <PageHeader
      title="识别规则"
      eyebrow="PATH EXTRACTION RULES"
      description="按字段管理规则。点规则进入编辑页，排查路径请用「识别调试」。"
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
        <details class="menu-wrap">
          <summary class="button">
            <Plus :size="14" />新增规则
          </summary>
          <div class="menu" @click="closeMenu">
            <span class="menu-title tiny muted">为哪个字段新增？</span>
            <button
              v-for="target in fieldOrder"
              :key="target"
              type="button"
              :disabled="!library || saving"
              @click="add(target, 'script')"
            >
              {{ targets[target] }}
            </button>
          </div>
        </details>
        <button
          class="button"
          :disabled="!canSubmit || examplesMutation.isPending.value"
          @click="examples"
        >
          {{ examplesMutation.isPending.value ? '样例运行中…' : '运行全部样例' }}
        </button>
        <details class="menu-wrap">
          <summary class="button" aria-label="更多操作">
            <MoreHorizontal :size="15" />
          </summary>
          <div class="menu" @click="closeMenu">
            <button type="button" :disabled="!library || saving" @click="reset">
              恢复默认规则
            </button>
          </div>
        </details>
        <span class="save-status" :class="{ dirty }" role="status"
          ><i />{{
            saving ? '正在保存…' : dirty ? '有未保存修改' : '与生效链一致'
          }}</span
        >
        <button
          class="button primary"
          :disabled="!canSubmit || !dirty"
          @click="save"
        >
          <Save :size="14" />保存
        </button>
      </div>

      <p v-if="error" class="notice error" role="alert">{{ error }}</p>
      <p v-if="notice" class="notice" role="status">{{ notice }}</p>
      <div v-if="validation.errors.length" class="banner" role="alert">
        <strong
          >{{ validation.errors.length }} 条规则需修正，修正前无法保存：</strong
        ><button
          v-for="item in validation.errors"
          :key="item.key"
          class="text-button"
          @click="open(item.key)"
        >
          {{ item.name }}：{{ item.message }}
        </button>
      </div>
      <div v-if="exampleResults.length" class="banner" role="status">
        <strong :class="failedExamples.length ? 'red' : 'green'"
          >样例 {{ exampleResults.length - failedExamples.length }}/{{
            exampleResults.length
          }}
          通过</strong
        ><button
          v-for="item in failedExamples"
          :key="item.ruleId + item.path"
          class="text-button"
          @click="router.push({ name: 'recognition-rule-edit', params: { ruleId: item.ruleId } })"
        >
          {{ ruleName(item.ruleId) }}：{{ item.path }} 期望
          {{ JSON.stringify(item.expected) }}，实际
          {{ JSON.stringify(item.actual) }}
        </button>
      </div>

      <fieldset class="list-area" :disabled="saving">
        <RuleFieldTable
          :drafts="drafts"
          :errors="validationErrors"
          :catalog="library?.catalog ?? []"
          :disabled="saving"
          @open="open"
          @add="add"
          @move="move"
          @remove="remove"
        />
      </fieldset>
      <p class="tiny muted foot">
        执行顺序按字段固定：TMDB ID → 季号 → 集数 → 媒体类型 → 片名 → 年份 →
        发布标签；同一字段内先命中的规则生效。启停、排序和删除在点「保存」后生效。
      </p>
    </QueryState>
  </section>
</template>
<style scoped>
.rules-page {
  display: grid;
  gap: 16px;
  min-width: 0;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.toolbar .save-status {
  margin-left: auto;
}
.menu-wrap {
  position: relative;
}
.menu-wrap summary {
  list-style: none;
}
.menu-wrap summary::-webkit-details-marker {
  display: none;
}
.menu {
  position: absolute;
  left: 0;
  top: calc(100% + 6px);
  z-index: 20;
  min-width: 160px;
  padding: 4px;
  display: grid;
  background: #fff;
  border: 1px solid var(--line);
  border-radius: 8px;
  box-shadow: 0 8px 24px #2920441a;
}
.menu-title {
  padding: 6px 10px 4px;
}
.menu button {
  text-align: left;
  border: 0;
  background: none;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 13px;
}
.menu button:hover:not(:disabled) {
  background: #f4f2fb;
}
.save-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--muted);
}
.save-status i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--green);
}
.save-status.dirty i {
  background: #d8973a;
}
.banner {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  align-items: center;
  font-size: 12px;
  padding: 10px 14px;
  border-radius: 10px;
  background: #fbfaff;
  border: 1px solid var(--line);
}
.list-area {
  border: 0;
  margin: 0;
  padding: 0;
  min-width: 0;
}
.foot {
  line-height: 1.8;
  margin: 0;
}
@media (max-width: 640px) {
  .toolbar .save-status {
    margin-left: 0;
    width: 100%;
    order: -1;
  }
}
</style>
