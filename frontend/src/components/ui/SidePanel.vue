<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { DialogRoot, DialogPortal, DialogOverlay, DialogContent, DialogTitle, DialogClose } from 'reka-ui'
import { X } from '@lucide/vue'
import Button from './Button.vue'
import { registerToastTarget } from '../../app/toast'

const open = defineModel<boolean>('open', { default: false })
defineProps<{ title: string; bodyClass?: string }>()
const surface = ref<HTMLElement>()
let releaseToastTarget: (() => void) | undefined
let returnFocus: HTMLElement | undefined
watch(open, value => {
  if (!value || typeof document === 'undefined') return
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  // A menu item disappears after opening the panel; return to its menu trigger.
  const triggerId = active?.closest('[role="menu"]')?.getAttribute('aria-labelledby')
  returnFocus = (triggerId ? document.getElementById(triggerId) : undefined) || active
}, { immediate: true, flush: 'sync' })
watch(surface, element => {
  releaseToastTarget?.()
  releaseToastTarget = element ? registerToastTarget(element) : undefined
}, { flush: 'post' })
onBeforeUnmount(() => releaseToastTarget?.())
function restoreFocus(event: Event) {
  event.preventDefault()
  if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true })
}
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="ui-side-panel-overlay" />
      <DialogContent class="ui-side-panel" :aria-describedby="undefined" @close-auto-focus="restoreFocus">
        <div ref="surface" class="ui-side-panel-surface">
          <header class="ui-side-panel-header">
            <DialogTitle class="ui-side-panel-title">{{ title }}</DialogTitle>
            <DialogClose as-child><Button variant="ghost" icon :aria-label="`关闭${title}`"><X :size="18" /></Button></DialogClose>
          </header>
          <div class="ui-side-panel-body" :class="bodyClass"><slot /></div>
          <footer v-if="$slots.footer" class="ui-side-panel-footer"><slot name="footer" /></footer>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style>
.ui-side-panel-overlay { position:fixed;inset:0;z-index:90;background:#211c3733; }
.ui-side-panel { position:fixed;inset:0 0 0 auto;z-index:91;width:min(580px,100vw);height:100dvh;outline:none;background:#fff;box-shadow:-12px 0 50px #29204420; }
.ui-side-panel-surface { display:flex;flex-direction:column;height:100%;min-height:0;min-width:0; }
.ui-side-panel-header { display:flex;align-items:center;justify-content:space-between;gap:16px;flex:none;padding:16px 24px;border-bottom:1px solid var(--line); }
.ui-side-panel-title { margin:0;font-size:16px;font-weight:600;color:#393547; }
.ui-side-panel-body { flex:1;min-height:0;min-width:0;overflow:auto;overscroll-behavior:contain;padding:24px; }
.ui-side-panel-footer { display:flex;align-items:center;justify-content:flex-end;gap:10px;flex:none;padding:16px 24px max(16px,env(safe-area-inset-bottom));border-top:1px solid var(--line);background:#fff; }
.ui-side-panel-footer>span:first-child { margin-right:auto;font-size:12px;color:var(--muted); }
@media(max-width:600px) {
  .ui-side-panel-header { padding:12px 16px; }
  .ui-side-panel-body { padding:20px 16px; }
  .ui-side-panel-footer { padding:12px 16px max(12px,env(safe-area-inset-bottom));gap:8px; }
  .ui-side-panel-footer>.ui-button { padding:0 11px; }
}
</style>
