import { nextTick, shallowRef } from 'vue'

export interface ConfirmOptions {
  title: string
  description: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
}

interface Request {
  options: ConfirmOptions
  resolve: (confirmed: boolean) => void
}

const queue: Request[] = []
export const activeConfirmation = shallowRef<Request | null>(null)

export function confirmAction(options: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    queue.push({ options, resolve })
    activeConfirmation.value ||= queue[0]!
  })
}

export function resolveConfirmation(confirmed: boolean) {
  const request = queue.shift()
  if (!request) return
  request.resolve(confirmed)
  activeConfirmation.value = null
  void nextTick(() => {
    activeConfirmation.value = queue[0] || null
  })
}

// Reka emits `update:open=false` as part of an action click. Defer treating it
// as cancellation so the action's click handler can settle the request first.
export function dismissConfirmation() {
  const request = activeConfirmation.value
  if (!request) return
  queueMicrotask(() => {
    if (activeConfirmation.value === request) resolveConfirmation(false)
  })
}
