<script setup lang="ts" generic="T">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'

const props = withDefaults(defineProps<{
  items: T[]
  itemKey: (item: T) => string | number
  label: string
  estimatedHeight?: number
}>(), { estimatedHeight: 44 })
const viewport = ref<HTMLElement>()
const virtualizer = useVirtualizer(computed(() => ({
  count: props.items.length,
  getScrollElement: () => viewport.value ?? null,
  getItemKey: (index: number) => props.itemKey(props.items[index]!),
  estimateSize: () => props.estimatedHeight,
  overscan: 5,
})))
const visible = computed(() => virtualizer.value.getVirtualItems())
const height = computed(() => virtualizer.value.getTotalSize())
function measure(element: Element | ComponentPublicInstance | null) {
  if (!element || element instanceof HTMLElement) virtualizer.value.measureElement(element)
}
let observer: ResizeObserver | undefined
onMounted(() => {
  let width = viewport.value?.clientWidth
  observer = new ResizeObserver(() => {
    const nextWidth = viewport.value?.clientWidth
    if (nextWidth !== width) { width = nextWidth; virtualizer.value.measure() }
  })
  if (viewport.value) observer.observe(viewport.value)
})
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <div ref="viewport" class="ui-virtual-list" role="list" :aria-label="label" tabindex="0">
    <div class="ui-virtual-list-content" :style="{ height: `${height}px` }">
      <div v-for="item in visible" :key="itemKey(items[item.index]!)" :ref="measure" :data-index="item.index" class="ui-virtual-list-item" role="listitem" :aria-posinset="item.index + 1" :aria-setsize="items.length" :style="{ transform: `translateY(${item.start}px)` }">
        <slot :item="items[item.index]!" :index="item.index" />
      </div>
    </div>
    <slot v-if="!items.length" name="empty" />
  </div>
</template>

<style>
.ui-virtual-list { min-height:0;min-width:0;overflow:auto;overflow-anchor:none;overscroll-behavior:contain;scrollbar-width:thin; }
.ui-virtual-list-content { position:relative;width:100%; }
.ui-virtual-list-item { position:absolute;top:0;left:0;width:100%; }
</style>
