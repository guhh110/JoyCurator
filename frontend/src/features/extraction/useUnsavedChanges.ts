import { onBeforeUnmount, onMounted, type Ref } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
import { confirmAction } from '../../app/confirm'

export function useUnsavedChanges(dirty: Readonly<Ref<boolean>>, subject: string) {
  let pending: Promise<boolean> | undefined
  function confirmLeave() {
    if (!dirty.value) return true
    pending ??= confirmAction({
      title: `放弃${subject}的修改？`,
      description: `${subject}有未保存的修改，离开后会丢失。`,
      confirmLabel: '放弃修改并离开',
      cancelLabel: '继续编辑',
      destructive: true,
    }).finally(() => { pending = undefined })
    return pending
  }
  onBeforeRouteLeave(confirmLeave)
  onBeforeRouteUpdate((to, from) =>
    to.params.ruleId !== from.params.ruleId ? confirmLeave() : true,
  )
  function beforeUnload(event: BeforeUnloadEvent) {
    if (!dirty.value) return
    event.preventDefault()
    event.returnValue = ''
  }
  onMounted(() => window.addEventListener('beforeunload', beforeUnload))
  onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
}
