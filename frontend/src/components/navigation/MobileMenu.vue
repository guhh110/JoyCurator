<script setup lang="ts">
import {
  DialogRoot,
  DialogPortal,
  DialogOverlay,
  DialogContent,
  DialogTitle,
  DialogClose,
} from 'reka-ui'
import { X } from '@lucide/vue'
import AppSidebar from './AppSidebar.vue'
defineProps<{ open: boolean }>()
defineEmits<{ 'update:open': [value: boolean] }>()
</script>
<template>
  <DialogRoot :open="open" @update:open="$emit('update:open', $event)"
    ><DialogPortal
      ><DialogOverlay class="mobile-menu-overlay" /><DialogContent
        class="mobile-menu-panel"
        :aria-describedby="undefined"
        ><DialogTitle class="sr-only">页面导航</DialogTitle
        ><DialogClose
          class="icon-button mobile-menu-close"
          aria-label="关闭导航"
          ><X :size="20" /></DialogClose
        ><AppSidebar
          @navigate="
            $emit('update:open', false)
          " /></DialogContent></DialogPortal
  ></DialogRoot>
</template>
<style scoped>
.mobile-menu-overlay {
  position: fixed;
  inset: 0;
  background: #25213666;
  z-index: 80;
}
.mobile-menu-panel {
  position: fixed;
  left: 0;
  top: 0;
  bottom: 0;
  width: min(290px, 88vw);
  z-index: 81;
  outline: 0;
  box-shadow: 12px 0 50px #241c4526;
}
.mobile-menu-close {
  position: absolute;
  right: 10px;
  top: 12px;
  z-index: 1;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
}
</style>
