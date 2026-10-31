<script setup lang="ts">
import { onBeforeUnmount, onMounted, onUnmounted, ref } from 'vue'
import { registerToastTarget } from '../app/toast'
import { X } from '@lucide/vue'
defineProps<{ title: string; wide?: boolean; locked?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>()
let previous: HTMLElement | null = null
let releaseToasts: (() => void) | undefined
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
