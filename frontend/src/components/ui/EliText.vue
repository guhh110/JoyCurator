<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import Tooltip from './Tooltip.vue'

const props = withDefaults(defineProps<{
  text: string
  side?: 'top' | 'right' | 'bottom' | 'left'
}>(), { side: 'top' })
const element = ref<HTMLElement>()
const truncated = ref(false)
let observer: ResizeObserver | undefined
let disposed = false
function measure() {
  if (disposed) return
  const node = element.value
  truncated.value = !!node && node.clientWidth > 0 && node.scrollWidth > node.clientWidth
}
watch(() => props.text, measure, { flush: 'post' })
onMounted(() => {
  observer = new ResizeObserver(measure)
  if (element.value) observer.observe(element.value)
  measure()
  void document.fonts?.ready.then(measure)
})
onBeforeUnmount(() => {
  disposed = true
  observer?.disconnect()
})
</script>

<template>
  <Tooltip :content="text" :side="side" :disabled="!truncated" class="eli-text">
    <span ref="element" class="eli-text-content" :tabindex="truncated ? 0 : undefined" @pointerenter="measure" @focus="measure">{{ text }}</span>
  </Tooltip>
</template>

<style>
.eli-text { min-width:0; max-width:100%; overflow:hidden; }
.eli-text-content { display:block; min-width:0; max-width:100%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
</style>
