import { computed, readonly, shallowRef, watch, type Ref } from 'vue'

export type ToastTone = 'success' | 'error' | 'info'
export interface ToastMessage {
  id: number
  message: string
  tone: ToastTone
  duration: number
}

const messages = shallowRef<ToastMessage[]>([])
const targets = shallowRef<HTMLElement[]>([])
let sequence = 0
export const toasts = readonly(messages)
export const toastTarget = computed(() => targets.value.at(-1) ?? 'body')

export function notify(message: string, tone: ToastTone = 'info') {
  if (!message.trim()) return
  const item = { id: ++sequence, message, tone, duration: tone === 'error' ? 7000 : 4500 }
  // A fresh ID restarts the dismissal timer for repeated action results.
  messages.value = [...messages.value.filter(value => value.message !== message || value.tone !== tone), item].slice(-3)
  return item.id
}

export function dismissToast(id: number) {
  messages.value = messages.value.filter(item => item.id !== id)
}

export function clearToasts() {
  messages.value = []
}

// Native dialogs are in the browser top layer. Toasts must be inside the most
// recently opened dialog to remain visible and keyboard-accessible.
export function registerToastTarget(element: HTMLElement) {
  targets.value = [...targets.value, element]
  return () => { targets.value = targets.value.filter(target => target !== element) }
}

export function useToastFeedback(error: Ref<string>, success?: Ref<string>, tone: ToastTone = 'success') {
  watch(error, value => { if (value) notify(value, 'error') }, { flush: 'sync' })
  if (success) watch(success, value => { if (value) notify(value, tone) }, { flush: 'sync' })
}
