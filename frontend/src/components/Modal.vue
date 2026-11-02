<script setup lang="ts">
import { onBeforeUnmount, onMounted, onUnmounted, ref } from 'vue'
import { registerToastTarget } from '../app/toast'
import { X } from '@lucide/vue'
const props = defineProps<{ title: string; wide?: boolean; locked?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>()
let previous: HTMLElement | null = null
let releaseToasts: (() => void) | undefined
let pressedOutside = false
function isOutside(event: MouseEvent) {
  const element = dialog.value
  if (!element || event.target !== element) return false
  const bounds = element.getBoundingClientRect()
  return event.clientX < bounds.left || event.clientX > bounds.right ||
    event.clientY < bounds.top || event.clientY > bounds.bottom
}
function closeOnBackdrop(event: MouseEvent) {
  const startedOutside = pressedOutside
  pressedOutside = false
  if (startedOutside && !props.locked && isOutside(event)) emit('close')
}
onMounted(() => {
  previous = document.activeElement as HTMLElement
  dialog.value?.showModal()
  if (dialog.value) releaseToasts = registerToastTarget(dialog.value)
})
onBeforeUnmount(() => releaseToasts?.())
onUnmounted(() => previous?.focus())
</script>
<template>
  <dialog
    ref="dialog"
    class="modal"
    :class="{ wide }"
    :aria-label="title"
    @cancel.prevent="!locked && emit('close')"
    @pointerdown.capture="pressedOutside = isOutside($event)"
    @pointercancel="pressedOutside = false"
    @click="closeOnBackdrop"
  >
    <header class="modal-head">
      <div>
        <span class="eyebrow">JOY / CURATOR</span>
        <h2>{{ title }}</h2>
        <slot name="subtitle" />
      </div>
      <button
        class="icon-button"
        :disabled="locked"
        aria-label="关闭弹窗"
        @click="emit('close')"
      >
        <X :size="20" />
      </button>
    </header>
    <div class="modal-body"><slot /></div>
    <footer v-if="$slots.footer" class="modal-foot">
      <slot name="footer" />
    </footer>
  </dialog>
</template>
