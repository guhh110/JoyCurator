<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Rule, Source } from './model'
import ScriptCode from './ScriptCode.vue'
import { sources, targets, emptyScript } from './model'
const props = defineProps<{
  rule: Rule
  sourceScripts: Partial<Record<Source, string>>
}>()
const emit = defineEmits<{ update: [value: Rule] }>()
const draft = ref<Rule>(JSON.parse(JSON.stringify(props.rule)) as Rule)
const constant = ref(false),
  constantText = ref(''),
  paramsText = ref('{}'),
  sourceCode = ref(''),
  examplesText = ref('[]'),
  error = ref('')
watch(
  () => props.rule,
  (value) => {
    draft.value = JSON.parse(JSON.stringify(value)) as Rule
    constant.value = Object.hasOwn(value.config, 'value')
    constantText.value = JSON.stringify(value.config.value ?? 'tv')
    paramsText.value = JSON.stringify(value.config.params ?? {}, null, 2)
    examplesText.value = JSON.stringify(value.examples ?? [], null, 2)
    sourceCode.value =
      value.config.sourceCode ?? props.sourceScripts.cleanFilename ?? ''
    error.value = ''
  },
  { immediate: true },
)
function kindChanged() {
  draft.value.config =
    draft.value.kind === 'regex'
      ? { pattern: '(?P<value>.+)', capture: 'value' }
      : { code: emptyScript }
  paramsText.value = '{}'
}
function commit() {
  try {
    const rule = JSON.parse(JSON.stringify(draft.value)) as Rule
    rule.examples = JSON.parse(examplesText.value)
    if (!Array.isArray(rule.examples)) throw new Error('样例必须是 JSON 数组')
    rule.config =
      rule.kind === 'script'
        ? { code: rule.config.code, params: JSON.parse(paramsText.value) }
        : constant.value
          ? {
              pattern: rule.config.pattern,
              value: JSON.parse(constantText.value),
            }
          : { pattern: rule.config.pattern, capture: rule.config.capture }
    if (rule.source === 'cleanFilename')
      rule.config.sourceCode = sourceCode.value
    emit('update', rule)
    error.value = ''
  } catch (e) {
    error.value = (e as Error).message
  }
}
</script>
<template>
  <form class="editor" @submit.prevent="commit">
    <h2>编辑规则</h2>
    <div class="grid">
      <div>
        <label for="rule-id">规则 ID</label
        ><input id="rule-id" v-model="draft.id" required />
      </div>
      <div>
        <label for="rule-name">名称</label
        ><input id="rule-name" v-model="draft.name" required />
      </div>
      <div>
        <label for="rule-target">提取目标（仅一个）</label
        ><select id="rule-target" v-model="draft.target">
          <option v-for="(label, key) in targets" :key="key" :value="key">
            {{ label }}
          </option>
        </select>
      </div>
      <div>
        <label for="rule-kind">规则类型</label
        ><select id="rule-kind" v-model="draft.kind" @change="kindChanged">
          <option value="regex">自定义正则</option>
          <option value="script">JavaScript 脚本</option>
        </select>
      </div>
      <div>
        <label for="rule-source">输入来源</label
        ><select id="rule-source" v-model="draft.source">
          <option v-for="(_, key) in sources" :key="key" :value="key">
            {{ sources[key as keyof typeof sources] }}
          </option>
        </select>
      </div>
      <div>
        <label for="rule-order">执行顺序</label
        ><input
          id="rule-order"
          v-model.number="draft.order"
          type="number"
          min="0"
          required
        />
      </div>
    </div>
    <label for="rule-description">说明</label
    ><input id="rule-description" v-model="draft.description" />
    <template v-if="draft.kind === 'script'">
      <label for="rule-code">完整脚本（JavaScript）</label>
      <ScriptCode
        id="rule-code"
        v-model="draft.config.code!"
        label="JavaScript 脚本"
      />
      <p class="muted tiny">
        定义同步 function extract(ctx)。使用
        ctx.text、ctx.directories、ctx.metadata、ctx.params；返回本字段值或
        null。reSearch / reFindAll / reSub
        会自动记录参数与结果，trace("说明", 中间值) 可记录判断依据。
        不提供 Node.js、网络或文件访问，不支持 async / Promise。
      </p>
      <label for="rule-params">参数 JSON</label
      ><textarea
        id="rule-params"
        v-model="paramsText"
        rows="3"
        spellcheck="false"
      />
    </template>
    <template v-else
      ><label for="rule-pattern">Go 正则表达式</label
      ><input
        id="rule-pattern"
        v-model="draft.config.pattern"
        required
        spellcheck="false" /><label class="check"
        ><input v-model="constant" type="checkbox" />使用固定输出值</label
      ><template v-if="constant"
        ><label for="rule-value">固定值 JSON（例如 "tv"、0、[1,2]）</label
        ><input
          id="rule-value"
          v-model="constantText"
          required
          spellcheck="false" /></template
      ><template v-else
        ><label for="rule-capture">命名捕获组</label
        ><input
          id="rule-capture"
          v-model="draft.config.capture"
          required
          placeholder="value"
          spellcheck="false" /></template
    ></template>
    <details v-if="draft.source === 'cleanFilename'" class="source-cleaner">
      <summary>来源清理脚本（同样可编辑与追踪）</summary>
      <ScriptCode v-model="sourceCode" label="来源清理脚本" />
    </details>
    <div class="checks">
      <label class="check"
        ><input v-model="draft.enabled" type="checkbox" />启用</label
      ><label class="check"
        ><input
          v-model="draft.overwrite"
          type="checkbox"
        />允许覆盖已有目标值</label
      >
    </div>
    <label for="rule-examples">测试样例 JSON</label
    ><textarea
      id="rule-examples"
      v-model="examplesText"
      rows="5"
      spellcheck="false"
      placeholder='[{"path":"/Show.S02E03.mkv","expected":2}]'
    />
    <p class="tiny muted">
      每个样例填写 path、可选 root、expected；预期值为本条规则提取值，未命中写
      null。
    </p>
    <p v-if="error" class="red" role="alert">{{ error }}</p>
    <button class="button primary">应用到当前编辑链</button>
    <p class="tiny muted">应用后可试跑；点击页面顶部“保存规则链”后正式生效。</p>
  </form>
</template>
<style scoped>
.editor {
  padding: 20px;
  border-top: 1px solid var(--line);
}
h2 {
  font-size: 17px;
  margin: 0 0 16px;
}
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}
.source-cleaner {
  margin-top: 18px;
}
.source-cleaner summary {
  cursor: pointer;
  color: var(--purple);
  margin-bottom: 12px;
}
.editor > label {
  margin-top: 16px;
}
textarea {
  font-family: monospace;
  font-size: 12px;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.check input {
  width: auto;
}
.checks {
  display: flex;
  gap: 20px;
  flex-wrap: wrap;
  margin-top: 20px;
}
p {
  line-height: 1.7;
}
@media (max-width: 600px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
</style>
