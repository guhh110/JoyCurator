<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { ChevronDown, Copy } from '@lucide/vue'
import { targets, type Target } from './model'
import { scriptContextFields, scriptFunctions, scriptMetadataFields, scriptReturnTypes, scriptRuntimeNotes } from './scriptReference'

const props = defineProps<{
  label: string
  editorId: string
  target: Target
  phase?: 'extract' | 'source'
}>()
const panelId = useId()
const open = ref(false)
const categories = [
  { id: 'functions', label: '内置函数' },
  { id: 'context', label: '输入上下文' },
  { id: 'returns', label: '返回值' },
  { id: 'runtime', label: '运行规则' },
] as const
const category = ref<(typeof categories)[number]['id']>('functions')
const returnType = computed(() => scriptReturnTypes[props.target])
const copyStatus = ref<Record<string, string>>({})
const copying = ref<Record<string, boolean>>({})

async function copyExample(name: string, code: string) {
  copying.value[name] = true
  copyStatus.value[name] = ''
  try {
    await navigator.clipboard.writeText(code)
    copyStatus.value[name] = '已复制示例，不会改动当前脚本。'
  } catch {
    copyStatus.value[name] = '无法访问剪贴板，请选中上方示例手动复制。'
  } finally {
    copying.value[name] = false
  }
}
</script>

<template>
  <div class="script-reference-editor">
    <div class="reference-heading">
      <label :for="editorId">{{ label }}</label>
    </div>
    <slot />
    <p class="reference-hint">
      使用 <code>extract(ctx)</code>
      <template v-if="phase === 'source'">返回清理后的字符串。</template>
      <template v-else>返回字段值，未命中返回 <code>null</code>。</template>
      <button class="reference-trigger" type="button" :aria-expanded="open" :aria-controls="panelId" @click="open = !open">
        脚本参考<ChevronDown :size="13" aria-hidden="true" :class="{ expanded: open }" />
      </button>
    </p>
    <section v-show="open" :id="panelId" class="reference-panel" :aria-label="`${label}参考`">
      <div class="reference-navigation" role="group" aria-label="参考分类">
        <button v-for="item in categories" :key="item.id" type="button" :aria-pressed="category === item.id" @click="category = item.id">{{ item.label }}</button>
      </div>
      <div class="reference-body">
        <template v-if="category === 'functions'">
          <p class="reference-intro">辅助函数可直接调用，无需导入。展开查看参数、返回值和完整示例。</p>
          <p v-if="phase === 'source'" class="reference-note">以下示例用于字段提取。用于来源清理时，需改为返回字符串，不能返回 null。</p>
          <details v-for="fn in scriptFunctions" :key="fn.name" class="function-reference">
            <summary><code>{{ fn.name }}</code><span>{{ fn.summary }}</span></summary>
            <div class="function-body">
              <code class="signature">{{ fn.name }}{{ fn.signature }}</code>
              <p>{{ fn.description }}</p>
              <div class="example-heading"><span>{{ fn.example.usage }}</span><button class="reference-link copy-button" type="button" :aria-label="`复制 ${fn.name} 示例`" :disabled="copying[fn.name]" @click="copyExample(fn.name, fn.example.code)"><Copy :size="13" aria-hidden="true" />{{ copying[fn.name] ? '复制中…' : '复制示例' }}</button></div>
              <pre tabindex="0" :aria-label="`${fn.name} 示例代码`"><code>{{ fn.example.code }}</code></pre>
              <p v-if="copyStatus[fn.name]" class="copy-status" role="status">{{ copyStatus[fn.name] }}</p>
            </div>
          </details>
          <details class="match-reference">
            <summary>Match 匹配对象包含什么？</summary>
            <dl>
              <dt><code>text</code></dt><dd>完整匹配文本。</dd>
              <dt><code>groups</code></dt><dd>捕获组数组，索引 0 是完整匹配，后续索引对应各捕获组。</dd>
              <dt><code>captures</code></dt><dd>命名捕获字典，如 <code>match.captures.year</code>。未参与匹配的捕获组值为空字符串。</dd>
              <dt><code>start / end</code></dt><dd>UTF-16 索引，起点包含、终点不包含，可直接用于 <code>ctx.text.slice(match.start, match.end)</code>。</dd>
            </dl>
          </details>
        </template>
        <template v-else-if="category === 'context'">
          <p class="reference-intro"><code>ctx</code> 及其嵌套内容全部只读。通过返回值提供结果，不要修改输入。</p>
          <dl class="field-reference">
            <template v-for="field in scriptContextFields" :key="field.name">
              <dt><code>ctx.{{ field.name }}</code><span>{{ field.type }} · {{ field.summary }}</span></dt>
              <dd>{{ field.description }}</dd>
            </template>
          </dl>
          <details class="match-reference">
            <summary>ctx.metadata 的全部字段</summary>
            <dl class="field-reference">
              <template v-for="field in scriptMetadataFields" :key="field.name">
                <dt><code>{{ field.name }}</code><span>{{ field.type }} · {{ field.summary }}</span></dt>
                <dd>{{ field.description }}</dd>
              </template>
            </dl>
          </details>
        </template>
        <template v-else-if="category === 'returns'">
          <template v-if="phase === 'source'">
            <h3>当前脚本：来源清理</h3>
            <p>必须返回清理后的字符串，而不是当前字段值；不需要清理时返回 <code>ctx.text</code>，不要返回 <code>null</code>。</p>
            <pre tabindex="0" aria-label="来源清理返回示例"><code>function extract(ctx) {
  return reSub("[._]+", ctx.text, " ").trim();
}</code></pre>
          </template>
          <template v-else>
            <h3>当前字段：{{ targets[target] }}</h3>
            <p><code>{{ returnType.type }}</code> · {{ returnType.description }}</p>
            <pre tabindex="0" aria-label="字段返回值示例"><code>return {{ returnType.example }};</code></pre>
            <p>未命中统一返回 <code>null</code>。每条规则只返回当前目标字段，不能一次返回整份元数据。</p>
            <h3>需要保留命中依据？</h3>
            <p>也可返回结果对象。<code>value</code> 必填，仍需满足当前字段类型；<code>matchedText</code> 是可选字符串，<code>captures</code> 是可选字符串字典。不接受其他结果字段。</p>
            <pre tabindex="0" aria-label="详细返回值示例"><code>return {
  value: {{ returnType.example }},
  matchedText: ctx.text,
  captures: { source: ctx.text }
};</code></pre>
            <p class="reference-note">这些固定值仅用于说明返回格式，不是完整提取算法。返回有效值不代表一定写入：默认只补全未知字段，是否覆盖由规则配置决定。</p>
          </template>
        </template>
        <dl v-else class="runtime-reference">
          <template v-for="note in scriptRuntimeNotes" :key="note.title"><dt>{{ note.title }}</dt><dd>{{ note.text }}</dd></template>
        </dl>
      </div>
    </section>
  </div>
</template>

<style scoped>
.script-reference-editor { min-width:0; }
.reference-heading { margin-bottom:8px; }
.reference-heading label { font-size:12px; }
.reference-trigger, .reference-link { display:inline-flex; align-items:center; justify-content:center; gap:5px; padding:3px 0; border:0; background:transparent; color:var(--purple-dark); font-size:12px; }
.reference-trigger { margin-left:6px; padding:0 2px; border-radius:4px; line-height:inherit; white-space:nowrap; }
.reference-trigger:hover, .reference-trigger[aria-expanded='true'] { background:#f0edf9; }
.reference-trigger .expanded { transform:rotate(180deg); }
.reference-link { text-decoration:underline; text-underline-offset:3px; }
.reference-hint { margin:9px 0 0; color:var(--muted); font-size:12px; line-height:1.8; }
.reference-panel { margin-top:12px; border:1px solid var(--line); border-radius:8px; overflow:hidden; background:#fff; }
.reference-navigation { display:flex; flex-wrap:wrap; gap:4px; padding:8px; border-bottom:1px solid var(--line); background:#faf9fc; }
.reference-navigation button { flex:1 1 auto; padding:7px 9px; border:0; border-radius:5px; color:var(--muted); background:transparent; font-size:12px; }
.reference-navigation button[aria-pressed='true'] { color:var(--purple-dark); background:#eee9fc; font-weight:600; }
.reference-body { padding:14px; font-size:12px; line-height:1.8; overflow-wrap:anywhere; }
.reference-body p { margin:8px 0; }
.reference-body .reference-intro { margin:0 0 12px; color:var(--muted); }
.reference-body h3 { margin:16px 0 6px; font-size:12px; }
.reference-body h3:first-child { margin-top:0; }
.function-reference { border-top:1px solid var(--line); }
.function-reference summary { padding:10px 0; cursor:pointer; }
.function-reference summary code { font-weight:600; }
.function-reference summary span { margin-left:10px; color:var(--muted); }
.function-body { padding:0 0 12px; }
code { font-family:'SFMono-Regular', Consolas, monospace; font-size:11px; }
.signature { display:block; overflow-wrap:anywhere; color:var(--purple-dark); }
.example-heading { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:6px; margin-top:12px; color:var(--muted); }
.copy-button { flex-shrink:0; }
pre { max-width:100%; margin:8px 0; padding:12px; border:1px solid var(--line); border-radius:6px; background:#f7f8fb; white-space:pre-wrap; overflow-wrap:anywhere; tab-size:2; }
.copy-status, .reference-note { color:var(--muted); }
.reference-body .reference-note { padding:10px; border-radius:6px; background:#f7f8fb; }
.match-reference { border-top:1px solid var(--line); padding-top:10px; }
.match-reference summary { cursor:pointer; color:var(--purple-dark); }
dl { margin:10px 0 0; }
dt { font-weight:600; }
dd { margin:3px 0 12px; color:var(--muted); }
dd:last-child { margin-bottom:0; }
.field-reference { margin:0 0 14px; }
.field-reference dt { display:flex; flex-wrap:wrap; gap:4px 10px; }
.field-reference dt span { font-weight:400; color:var(--muted); }
.runtime-reference { margin:0; }
@media (max-width:600px) {
  .reference-body { padding:10px; }
  .function-reference summary span { display:block; margin:3px 0 0; }
  .reference-navigation button { flex-basis:calc(50% - 4px); }
}
</style>
