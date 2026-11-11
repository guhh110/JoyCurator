<script setup lang="ts">
import { LoaderCircle } from '@lucide/vue'
withDefaults(defineProps<{
  variant?: 'default' | 'primary' | 'ghost'
  size?: 'sm' | 'md'
  icon?: boolean
  loading?: boolean
  disabled?: boolean
  type?: 'button' | 'submit' | 'reset'
}>(), { variant: 'default', size: 'md', type: 'button' })
</script>
<template>
  <button :type="type" class="ui-button" :class="[`ui-button-${variant}`, `ui-button-${size}`, { 'ui-button-icon': icon }]" :disabled="disabled || loading" :aria-busy="loading || undefined">
    <LoaderCircle v-if="loading" :size="15" class="spin" aria-hidden="true" /><slot />
  </button>
</template>
<style>
.ui-button{display:inline-flex;align-items:center;justify-content:center;gap:7px;flex:none;min-width:0;height:36px;min-height:36px;margin:0;padding:0 13px;border:1px solid #e3dfec;border-radius:8px;background:#fff;color:#393547;font-size:12px;font-weight:550;line-height:1.2;white-space:nowrap;box-shadow:0 1px 2px #29204404;cursor:pointer;transition:background .15s,border-color .15s}
.ui-button:hover:not(:disabled){background:#f8f6fd;border-color:#c9bfe7}.ui-button:disabled{opacity:.45;cursor:not-allowed}.ui-button-primary{background:var(--purple);border-color:var(--purple);color:#fff}.ui-button-primary:hover:not(:disabled){background:var(--purple-dark);border-color:var(--purple-dark)}.ui-button-ghost{border-color:transparent;background:transparent;box-shadow:none;color:var(--purple-dark)}.ui-button-ghost:hover:not(:disabled),.ui-button-ghost[aria-expanded=true]{background:#f0edf9;border-color:transparent}.ui-button-sm{height:30px;min-height:30px;padding:0 9px;font-size:11px}.ui-button-icon{width:36px;padding:0}.ui-button-sm.ui-button-icon{width:30px}.ui-button>svg{flex:none}.ui-button:focus-visible{outline:3px solid #b8adeb;outline-offset:2px}
</style>
