<script setup lang="ts">
import { computed } from 'vue'
import ScriptCode from './ScriptCode.vue'
import { sources, targets, emptyScript } from './model'
import { draftRule, type RuleDraft } from './useRuleDraft'
const draft = defineModel<RuleDraft>({ required: true })
defineProps<{
  focus?: { phase: string; line: number; request: number }
}>()
const emit = defineEmits<{ retarget: [] }>()
const error = computed(() => {
  try {
    draftRule(draft.value)
    return ''
  } catch (e) {
    return (e as Error).message
  }
})
function kindChanged() {
  draft.value.rule.config =
    draft.value.rule.kind === 'regex'
      ? { pattern: '(?P<value>.+)', capture: 'value' }
      : { code: emptyScript }
  draft.value.paramsText = '{}'
}
</script>
<template>
  <div class="editor">
    <div class="editor-section">
      <template v-if="draft.rule.kind === 'script'">
        <label for="rule-code" class="sr-only">完整脚本（JavaScript）</label>
        <ScriptCode
          id="rule-code"
          v-model="draft.rule.config.code!"
          label="JavaScript 脚本"
          :active-line="focus?.phase === 'extract' ? focus.line : undefined"
          :focus-key="focus?.phase === 'extract' ? focus.request : undefined"
        />
        <p class="script-help tiny muted">
          function extract(ctx) 返回本字段值，未命中返回 null。reSearch /
          reFindAll / reSub 会自动记录过程，trace("说明", 数据) 补充依据。
        </p>
      </template>
      <template v-else>
        <label for="rule-pattern">Go 正则表达式</label
        ><input
          id="rule-pattern"
          v-model="draft.rule.config.pattern"
          required
          spellcheck="false"
        />
        <label class="check"
          ><input
            v-model="draft.constant"
            type="checkbox"
          />使用固定输出值</label
        >
        <template v-if="draft.constant"
          ><label for="rule-value">固定值 JSON（例如 "tv"、0、[1,2]）</label
          ><input
            id="rule-value"
            v-model="draft.constantText"
            required
            spellcheck="false"
        /></template>
        <template v-else
          ><label for="rule-capture">命名捕获组</label
          ><input
            id="rule-capture"
            v-model="draft.rule.config.capture"
            required
            placeholder="value"
            spellcheck="false"
        /></template>
      </template>
    </div>
    <details class="auxiliary">
      <summary>基本 <span class="muted">名称、字段、说明</span></summary>
      <div class="grid">
        <div>
          <label for="rule-name">规则名称</label
          ><input id="rule-name" v-model="draft.rule.name" required />
        </div>
        <div>
          <label for="rule-target">提取字段</label
          ><select
            id="rule-target"
            v-model="draft.rule.target"
            @change="emit('retarget')"
          >
            <option v-for="(label, key) in targets" :key="key" :value="key">
              {{ label }}
            </option>
          </select>
        </div>
      </div>
      <label for="rule-description">规则说明</label
      ><textarea
        id="rule-description"
        v-model="draft.rule.description"
        rows="2"
      />
    </details>
    <details
      class="auxiliary"
      :open="focus?.phase === 'source' || undefined"
    >
      <summary>高级 <span class="muted">来源、覆盖、提取方式、参数、ID</span></summary>
      <div class="grid">
        <div>
          <label for="rule-source">输入来源</label
          ><select id="rule-source" v-model="draft.rule.source">
            <option v-for="(label, key) in sources" :key="key" :value="key">
              {{ label }}
            </option>
          </select>
        </div>
        <div>
          <label for="rule-kind">提取方式</label
          ><select
            id="rule-kind"
            v-model="draft.rule.kind"
            @change="kindChanged"
          >
            <option value="script">JavaScript 脚本</option>
            <option value="regex">自定义正则</option>
          </select>
        </div>
        <div>
          <label for="rule-id">规则 ID</label
          ><input id="rule-id" v-model="draft.rule.id" required />
        </div>
      </div>
      <label class="check"
        ><input
          v-model="draft.rule.overwrite"
          type="checkbox"
        />允许覆盖已有值（默认只补全未知字段）</label
      >
      <template v-if="draft.rule.kind === 'script'">
        <label for="rule-params">参数 JSON（ctx.params）</label
        ><textarea
          id="rule-params"
          v-model="draft.paramsText"
          rows="3"
          spellcheck="false"
        />
      </template>
      <template v-if="draft.rule.source === 'cleanFilename'">
        <label>来源清理脚本（匹配前执行）</label>
        <ScriptCode
          v-model="draft.sourceCode"
          label="来源清理脚本"
          :active-line="focus?.phase === 'source' ? focus.line : undefined"
          :focus-key="focus?.phase === 'source' ? focus.request : undefined"
        />
      </template>
    </details>
    <details class="auxiliary">
      <summary>测试样例 <span class="muted">path / root / expected</span></summary>
      <label for="rule-examples" class="sr-only">本规则测试样例 JSON</label
      ><textarea
        id="rule-examples"
        v-model="draft.examplesText"
        rows="8"
        spellcheck="false"
        placeholder='[{"path":"/Show.S02E03.mkv","expected":2}]'
      />
      <p class="tiny muted">未命中用 null。顶部「运行样例」检查整条草稿链。</p>
    </details>
    <p v-if="error" class="editor-error" role="alert">
      配置未完成：{{ error }}。修改会保留，修正后可试跑或保存。
    </p>
  </div>
</template>
<style scoped>
.editor {
  min-width: 0;
}
.editor-section {
  padding: 20px;
}
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}
.editor-section > label {
  margin-top: 18px;
}
.editor-section > label:first-child {
  margin-top: 0;
}
textarea {
  font-family: monospace;
  font-size: 12px;
  line-height: 1.7;
}
.script-help {
  line-height: 1.8;
  margin: 12px 0;
}
.auxiliary {
  border-top: 1px solid var(--line);
  padding: 13px 20px;
}
.auxiliary > label {
  margin-top: 14px;
}
.check {
  margin-top: 14px;
}
.auxiliary summary {
  cursor: pointer;
  font-size: 12px;
}
.auxiliary summary span {
  margin-left: 6px;
  font-size: 10px;
}
.auxiliary[open] summary {
  margin-bottom: 12px;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.check input {
  width: auto;
}
.editor-error {
  color: #a8443d;
  background: #fff4ef;
  padding: 12px 20px;
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  border-top: 1px solid #f2d9ce;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
@media (max-width: 760px) {
  .grid {
    grid-template-columns: 1fr;
  }
  .editor-section {
    padding: 16px;
  }
      }
</style>
