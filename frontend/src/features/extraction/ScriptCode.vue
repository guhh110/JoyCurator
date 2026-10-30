<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, ref, watch } from 'vue'
import type { ScriptDiagnostic } from './scriptDiagnostics'
const ScriptEditor = defineAsyncComponent(() => import('./ScriptEditor.vue'))
const props = defineProps<{
  modelValue: string
  readonly?: boolean
  activeLine?: number
  focusKey?: number
  label?: string
  id?: string
  parameterNames?: string[]
  diagnostics?: ScriptDiagnostic[]
  disabled?: boolean
}>()
defineEmits<{ 'update:modelValue': [value: string] }>()
const lines = computed(() => props.modelValue.split('\n'))
const source = ref<HTMLElement>()
watch(
  () => [props.activeLine, props.focusKey],
  async () => {
    await nextTick()
    const active = source.value?.querySelector<HTMLElement>('[data-active="true"]')
    if (source.value && active) {
      source.value.scrollTo({ top: Math.max(0, active.offsetTop - source.value.clientHeight / 2), behavior: 'smooth' })
    }
  },
  { immediate: true },
)
</script>
<template>
  <div
    v-if="readonly"
    ref="source"
    class="script-read"
    :aria-label="label || '脚本源码'"
  >
    <div
      v-for="(line, index) in lines"
      :key="index"
      class="source-line"
      :data-active="activeLine === index + 1"
    >
      <span class="line-number">{{ index + 1 }}</span
      ><code>{{ line || ' ' }}</code>
    </div>
  </div>
  <ScriptEditor v-else :id="id" :model-value="modelValue" :label="label" :active-line="activeLine" :focus-key="focusKey" :parameter-names="parameterNames" :diagnostics="diagnostics" :disabled="disabled" @update:model-value="$emit('update:modelValue', $event)" />
</template>
<style scoped>
.script-read {
  border: 1px solid var(--line);
  border-radius: 8px;
  overflow: hidden;
  background: #f7f8fb;
  font-family: monospace;
  font-size: 12px;
  line-height: 20px;
  min-width: 0;
}
.script-read {
  position: relative;
  max-height: 420px;
  overflow: auto;
  padding: 10px 0;
}
.source-line {
  display: flex;
  min-width: max-content;
  line-height: 20px;
  white-space: pre;
}
.source-line[data-active='true'] {
  background: #eee5ff;
  box-shadow: inset 3px 0 var(--purple);
}
.line-number {
  position: sticky;
  left: 0;
  min-width: 44px;
  padding: 0 10px;
  text-align: right;
  background: #f0f0f5;
  color: var(--muted);
  user-select: none;
}
.source-line code {
  padding-right: 16px;
  font: inherit;
  color: var(--ink);
}
</style>
