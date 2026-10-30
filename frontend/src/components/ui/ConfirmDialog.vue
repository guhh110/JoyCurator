<script setup lang="ts">
import {
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
} from 'reka-ui'
import {
  activeConfirmation,
  dismissConfirmation,
  resolveConfirmation,
} from '../../app/confirm'
</script>

<template>
  <AlertDialogRoot
    :open="!!activeConfirmation"
    @update:open="(open) => { if (!open) dismissConfirmation() }"
  >
    <AlertDialogPortal>
      <AlertDialogOverlay class="confirm-overlay" />
      <AlertDialogContent class="confirm-dialog">
        <AlertDialogTitle class="confirm-title">
          {{ activeConfirmation?.options.title }}
        </AlertDialogTitle>
        <AlertDialogDescription class="confirm-description">
          {{ activeConfirmation?.options.description }}
        </AlertDialogDescription>
        <div class="confirm-actions">
          <AlertDialogCancel class="button" @click="resolveConfirmation(false)">
            {{ activeConfirmation?.options.cancelLabel || '取消' }}
          </AlertDialogCancel>
          <AlertDialogAction
            class="button"
            :class="activeConfirmation?.options.destructive ? 'confirm-danger' : 'primary'"
            @click="resolveConfirmation(true)"
          >
            {{ activeConfirmation?.options.confirmLabel || '确认' }}
          </AlertDialogAction>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
</template>
