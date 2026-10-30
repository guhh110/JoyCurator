<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { TooltipProvider, TooltipRoot, TooltipTrigger, TooltipPortal, TooltipContent, TooltipArrow } from 'reka-ui'

withDefaults(defineProps<{
  content: string
  side?: 'top' | 'right' | 'bottom' | 'left'
  disabled?: boolean
}>(), { side: 'top' })
const anchor = ref<HTMLElement>()
const portalTarget = ref<HTMLElement | string>('body')
const open = ref(false)
onMounted(() => {
  // Native dialogs live in the top layer; their tooltips must stay inside it.
  portalTarget.value = anchor.value?.closest('dialog') || 'body'
})
</script>

<template>
  <span ref="anchor" class="ui-tooltip-anchor">
    <TooltipProvider :delay-duration="250">
      <TooltipRoot :open="!disabled && open" @update:open="open = !disabled && $event">
        <TooltipTrigger as-child><slot /></TooltipTrigger>
        <TooltipPortal :to="portalTarget">
          <TooltipContent v-if="!disabled" class="ui-tooltip-content" :side="side" :side-offset="6" :collision-padding="8" position-strategy="fixed">
            {{ content }}
            <TooltipArrow class="ui-tooltip-arrow" :width="8" :height="4" />
          </TooltipContent>
        </TooltipPortal>
      </TooltipRoot>
    </TooltipProvider>
  </span>
</template>

<style>
.ui-tooltip-anchor { display:inline-flex; align-items:center; flex-shrink:0; }
.ui-tooltip-content { z-index:150; max-width:min(320px, calc(100vw - 24px)); padding:6px 10px; border-radius:6px; background:#302b3c; color:#fff; font-size:12px; line-height:1.5; overflow-wrap:anywhere; box-shadow:0 3px 12px #29204420; }
.ui-tooltip-arrow { fill:#302b3c; }
</style>
