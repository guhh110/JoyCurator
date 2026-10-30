<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { ChevronDown, Plus, Save } from '@lucide/vue'
import { DropdownMenuItem } from 'reka-ui'
import ActionMenu from '../../components/ui/ActionMenu.vue'
import { confirmAction } from '../../app/confirm'
import PageHeader from '../../components/ui/PageHeader.vue'
import QueryState from '../../components/ui/QueryState.vue'
import { keys } from '../../app/query'
import { useUnsavedChanges } from './useUnsavedChanges'
import { useRuleExamples } from './useRuleExamples'
import RuleExamples from './RuleExamples.vue'
import { draftScriptDiagnostics } from './scriptDiagnostics'
import { useRuleLibrary } from './queries'
import { api } from '../../api'
import type { Rule, Target } from './model'
import { fieldOrder, targets } from './model'
import RuleFieldTable from './RuleFieldTable.vue'
import { useRuleDraft } from './useRuleDraft'
import { useToastFeedback } from '../../app/toast'

// Rule list page: manages the chain (enable, order inside a field, add, delete,
// defaults, examples). Editing a rule happens on its own page.
defineEmits<{ back: [] }>()
const router = useRouter()
const libraryQuery = useRuleLibrary(),
  client = useQueryClient()
const library = libraryQuery.data
const { drafts, dirty, validation, load, move, remove, markSaved } =
  useRuleDraft()
const unsaved = computed(() => !!library.value && dirty.value)
const error = ref(''),
  notice = ref('')
useToastFeedback(error, notice, 'info')
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
const syntaxProblems = computed(() => drafts.value.flatMap(draft => draftScriptDiagnostics(draft)))
const saveMutation = useMutation({
  mutationFn: (rules: Rule[]) => api('/extraction/rules', { rules }, 'PUT'),
})
const { results: exampleResults, checkedKey, busy: checking, stale: examplesStale, run: checkExamples, reset: resetExamples } = useRuleExamples(() => validation.value.errors.length ? undefined : validation.value.rules)
const saving = saveMutation.isPending
const submitting = ref(false)
const resetting = ref(false)
const activeMenu = ref('')
const busy = computed(() => submitting.value || checking.value || resetting.value)
const canSubmit = computed(
  () => !!library.value && !validation.value.errors.length && !syntaxProblems.value.length && !busy.value,
)
let disposed = false
onBeforeUnmount(() => { disposed = true })

// Leaving with unsaved list changes would drop them; ask first.
useUnsavedChanges(unsaved, '规则列表')
function open(key: string) {
  const draft = drafts.value.find((item) => item.key === key)
  if (!draft) return
  void router.push({
    name: 'recognition-rule-edit',
    params: { ruleId: draft.rule.id },
  })
}
function add(target: Target, kind: 'script' | 'regex', templateId?: string) {
  if (!library.value || busy.value) return
  void router.push({
    name: 'recognition-rule-edit',
    params: { ruleId: 'new' },
    query: { target, kind, ...(templateId ? { template: templateId } : {}) },
  })
}
async function reset() {
  if (!library.value || busy.value) return
  activeMenu.value = ''
  resetting.value = true
  try {
    const confirmed = await confirmAction({
      title: '恢复默认规则？',
      description: '将用默认规则替换当前列表草稿，当前未保存的修改会丢失。不会立即修改已生效的规则，点击「保存」后才生效。',
      confirmLabel: '替换当前草稿',
      cancelLabel: '保留当前草稿',
      destructive: true,
    })
    if (!confirmed || disposed || !library.value || submitting.value || checking.value) return
    load(library.value.defaults, library.value.sourceScripts, false)
    resetExamples()
    error.value = ''
    notice.value = ''
    notice.value = '默认规则已载入，保存后生效'
  } finally {
    resetting.value = false
  }
}
async function save() {
  if (!canSubmit.value) return
  submitting.value = true
  error.value = ''
  notice.value = ''
  const rules = JSON.parse(JSON.stringify(validation.value.rules)) as Rule[]
  try {
    if (!await checkExamples(rules)) return
    await saveMutation.mutateAsync(rules)
    markSaved()
    client.setQueryData(keys.ruleLibrary, { ...library.value!, rules })
    notice.value = '规则链已保存并生效'
    void client.invalidateQueries({ queryKey: keys.ruleLibrary })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    submitting.value = false
  }
}
async function examples() {
  if (!canSubmit.value) return
  await checkExamples(validation.value.rules)
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
          返回整理工作区
        </button></template
      ></PageHeader
    >
    <QueryState
      :loading="libraryQuery.isPending.value"
      :error="libraryQuery.error.value"
      @retry="libraryQuery.refetch()"
    >
      <div class="toolbar">
        <ActionMenu :open="activeMenu === 'new'" :disabled="!library || busy" label="为哪个字段新增？" @update:open="value => { if (value) activeMenu = 'new'; else if (activeMenu === 'new') activeMenu = '' }">
          <template #trigger><button class="button" type="button"><Plus :size="14" />新增规则<ChevronDown :size="13" /></button></template>
          <DropdownMenuItem
            v-for="target in fieldOrder"
            :key="target"
            class="ui-action-menu-item"
            :disabled="!library || busy"
            @select="add(target, 'script')"
          >
            {{ targets[target] }}
          </DropdownMenuItem>
        </ActionMenu>
        <button
          class="button"
          :disabled="!canSubmit"
          @click="examples"
        >
          {{ checking ? '样例运行中…' : '运行全部样例' }}
        </button>
        <button class="restore-defaults" type="button" :disabled="!library || busy" @click="reset">恢复默认规则</button>
        <span class="save-status" :class="{ dirty }" role="status"
          ><i />{{
            checking ? '正在验证样例…' : saving ? '正在保存…' : dirty ? '有未保存修改' : '与生效链一致'
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
      <p v-if="syntaxProblems.length" class="notice error" role="alert">规则包含脚本错误，请进入编辑页修正后保存。</p>
      <RuleExamples :results="exampleResults" :checked="!!checkedKey" :busy="checking" :stale="examplesStale" />

      <fieldset class="list-area" :disabled="busy">
        <RuleFieldTable
          v-model:open-menu="activeMenu"
          :drafts="drafts"
          :errors="validationErrors"
          :catalog="library?.catalog ?? []"
          :disabled="busy"
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
.restore-defaults { padding:8px; border:0; border-radius:6px; background:transparent; color:var(--muted); font-size:12px; }
.restore-defaults:hover:not(:disabled) { background:#f4f2fb; color:var(--purple-dark); }
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
