import type { Router } from 'vue-router'
import { loadSession, type Session } from './session'
export function installGuards(
  router: Router,
  readSession: () => Promise<Session> = loadSession,
) {
  router.beforeEach(async (to) => {
    let value: Session
    try {
      value = await readSession()
    } catch {
      return to.name === 'login'
        ? true
        : { name: 'login', query: { redirect: to.fullPath } }
    }
    if (!value.username && !to.meta.public)
      return { name: 'login', query: { redirect: to.fullPath } }
    if (value.username && to.name === 'login') return { name: 'workbench' }
  })
  router.afterEach((to) => {
    if (typeof document !== 'undefined')
      document.title = `${String(to.meta.title || '媒体整理')} · JoyCurator`
  })
}
