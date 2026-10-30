<script setup lang="ts">
import { watch } from 'vue'
import { DropdownMenuContent, DropdownMenuPortal, DropdownMenuRoot, DropdownMenuTrigger } from 'reka-ui'

const open = defineModel<boolean>('open', { default: false })
const props = withDefaults(defineProps<{ label?: string; align?: 'start' | 'end'; disabled?: boolean }>(), { align: 'start' })
watch(() => props.disabled, disabled => { if (disabled) open.value = false })
</script>

<template>
  <DropdownMenuRoot v-model:open="open" :modal="false">
    <DropdownMenuTrigger as-child :disabled="disabled"><slot name="trigger" /></DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent class="ui-action-menu" :align="align" :side-offset="6" :collision-padding="12" loop>
        <div v-if="label" class="ui-action-menu-label">{{ label }}</div>
        <slot />
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<style>
.ui-action-menu { z-index:110; min-width:160px; max-width:calc(100vw - 24px); max-height:var(--reka-dropdown-menu-content-available-height); overflow:auto; padding:4px; border:1px solid var(--line); border-radius:8px; background:#fff; color:#333543; box-shadow:0 8px 24px #2920441a; outline:none; }
.ui-action-menu-label { padding:6px 10px 4px; color:var(--muted); font-size:11px; }
.ui-action-menu-item { display:flex; align-items:center; min-height:34px; padding:8px 10px; border-radius:6px; font-size:13px; outline:none; cursor:pointer; user-select:none; overflow-wrap:anywhere; }
.ui-action-menu-item[data-highlighted] { background:#f4f2fb; color:var(--purple-dark); }
.ui-action-menu-item[data-disabled] { opacity:.45; cursor:not-allowed; }
</style>
