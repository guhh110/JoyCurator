<script setup lang="ts">
import { useAttrs } from 'vue'
defineOptions({ inheritAttrs: false })
withDefaults(defineProps<{ type?: string; disabled?: boolean; invalid?: boolean; size?: 'sm' | 'md' }>(), { type: 'text', size: 'md' })
const value = defineModel<string>({ default: '' })
const attrs = useAttrs()
</script>
<template>
  <span class="ui-input-wrap" :class="[attrs.class, `ui-input-${size}`, { 'has-icon': !!$slots.icon, 'is-invalid': invalid, 'is-disabled': disabled }]" :style="attrs.style" :data-invalid="invalid || undefined">
    <span v-if="$slots.icon" class="ui-input-icon"><slot name="icon" /></span>
    <input v-bind="{ ...attrs, style: undefined, class: undefined }" v-model="value" class="ui-input" :type="type" :disabled="disabled" :aria-invalid="invalid || undefined" />
  </span>
</template>
<style>
.ui-input-wrap{position:relative;display:inline-flex;align-items:center;min-width:0;width:100%;height:36px;flex:none;border:1px solid #e3dfec;border-radius:8px;background:#fff;transition:border-color .15s,box-shadow .15s}
.ui-input-wrap>.ui-input{display:block;appearance:none;min-width:0;width:100%;height:100%;min-height:0;margin:0;padding:0 11px;border:0;border-radius:inherit;background:transparent;color:#393547;font-size:12px;line-height:normal;outline:none;box-shadow:none}.ui-input-wrap>.ui-input:focus{border:0;box-shadow:none;outline:none}.ui-input-wrap:focus-within{border-color:#a294d5;box-shadow:0 0 0 3px #7863ca12}.ui-input-wrap.is-invalid{border-color:#d7a0a8;background:#fffcfc}.ui-input-wrap.is-disabled{opacity:.45;background:#f8f7fa}.ui-input-icon{position:absolute;left:11px;display:flex;color:var(--muted);pointer-events:none}.ui-input-wrap.has-icon>.ui-input{padding-left:33px}.ui-input::placeholder{color:#a19aaa}.ui-input-sm{height:32px}.ui-input-sm>.ui-input{padding:0 9px;font-size:12px}.ui-input::-webkit-search-cancel-button{cursor:pointer}
</style>
