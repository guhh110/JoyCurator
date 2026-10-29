<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
const props = defineProps<{
  modelValue: string
  readonly?: boolean
  activeLine?: number
  focusKey?: number
  label?: string
  id?: string
}>()
defineEmits<{ 'update:modelValue': [value: string] }>()
const lines = computed(() => props.modelValue.split('\n'))
const gutter = ref<HTMLElement>(),
  source = ref<HTMLElement>()
function scroll(event: Event) {
  if (gutter.value)
    gutter.value.scrollTop = (event.target as HTMLTextAreaElement).scrollTop
}
watch(
  () => [props.activeLine, props.focusKey],
  async () => {
    await nextTick()
    source.value
      ?.querySelector('[data-active="true"]')
      ?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  },
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
  <div v-else class="script-edit">
    <pre ref="gutter" class="line-numbers" aria-hidden="true">{{
      lines.map((_, index) => index + 1).join('\n')
    }}</pre>
    <textarea
      :id="id"
      :value="modelValue"
      :aria-label="label || 'JavaScript 脚本'"
      spellcheck="false"
      wrap="off"
      rows="18"
      autocapitalize="off"
      autocomplete="off"
      @input="
        $emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)
      "
      @scroll="scroll"
    />
  </div>
</template>
<style scoped>
.script-edit,
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
.script-edit {
  display: flex;
}
.line-numbers {
  margin: 0;
  padding: 12px 10px;
  min-width: 44px;
  text-align: right;
  overflow: hidden;
  max-height: 384px;
  background: #eeeff5;
  color: var(--muted);
  user-select: none;
  font: inherit;
  line-height: 20px;
}
.script-edit textarea {
  border: 0;
  border-radius: 0;
  margin: 0;
  min-width: 0;
  flex: 1;
  padding: 12px;
  resize: vertical;
  min-height: 160px;
  max-height: 640px;
  white-space: pre;
  font: inherit;
  line-height: 20px;
  tab-size: 4;
}
.script-read {
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
