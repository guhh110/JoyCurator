<script setup lang="ts">
import { CheckCircle2, CircleAlert, Info, X } from '@lucide/vue'
import { ToastClose, ToastDescription, ToastProvider, ToastRoot, ToastViewport } from 'reka-ui'
import { dismissToast, toasts, toastTarget } from '../../app/toast'
</script>

<template>
  <Teleport :to="toastTarget">
    <ToastProvider label="操作提示" :duration="4500" swipe-direction="right">
      <ToastRoot v-for="toast in toasts" :key="toast.id" :open="true" :duration="toast.duration"
        class="feedback-toast" :class="`feedback-${toast.tone}`" type="foreground"
        @update:open="open => { if (!open) dismissToast(toast.id) }">
        <component :is="toast.tone === 'error' ? CircleAlert : toast.tone === 'success' ? CheckCircle2 : Info"
          class="feedback-icon" :size="18" aria-hidden="true" />
        <ToastDescription class="feedback-message">{{ toast.message }}</ToastDescription>
        <ToastClose class="feedback-close" aria-label="关闭提示"><X :size="15" /></ToastClose>
      </ToastRoot>
      <ToastViewport class="feedback-viewport" label="操作提示（{hotkey}）" />
    </ToastProvider>
  </Teleport>
</template>

<style>
.feedback-viewport { position:fixed; z-index:1000; top:max(18px, env(safe-area-inset-top)); right:max(18px, env(safe-area-inset-right)); display:flex; flex-direction:column; gap:8px; width:min(420px, calc(100vw - 32px)); max-height:calc(100dvh - 36px); margin:0; padding:0; list-style:none; pointer-events:none; outline:none; }
.feedback-toast { display:flex; align-items:flex-start; gap:10px; padding:13px 12px 13px 15px; background:#fff; color:#333543; border:1px solid #e4e1eb; border-radius:10px; box-shadow:0 8px 32px #2920441f; font-family:inherit; pointer-events:auto; transform:translateX(var(--reka-toast-swipe-move-x, 0)); }
.feedback-message { min-width:0; flex:1; font-size:13px; line-height:1.6; overflow-wrap:anywhere; white-space:pre-line; }
.feedback-icon { flex-shrink:0; margin-top:2px; color:var(--purple); }
.feedback-success .feedback-icon { color:var(--green); }
.feedback-error .feedback-icon { color:var(--red); }
.feedback-close { display:flex; align-items:center; justify-content:center; flex-shrink:0; width:24px; height:24px; padding:0; border:0; border-radius:5px; background:transparent; color:var(--muted); }
.feedback-close:hover { background:#f4f2f8; }
.feedback-toast[data-state='open'] { animation:feedback-enter .16s ease-out; }
.feedback-toast[data-swipe='cancel'] { transform:translateX(0); transition:transform .16s ease-out; }
@keyframes feedback-enter { from { opacity:0; translate:0 -6px; } to { opacity:1; translate:0 0; } }
@media(prefers-reduced-motion:reduce) { .feedback-toast[data-state='open'] { animation:none; } }
@media(max-width:600px) { .feedback-viewport { top:12px; right:12px; width:calc(100vw - 24px); } }
@media(pointer:coarse) { .feedback-close { width:36px; height:36px; } }
</style>
