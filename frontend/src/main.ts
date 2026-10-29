import { createApp } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import App from './App.vue'
import { router } from './app/router'
import { queryClient } from './app/query'
import { clearSession } from './app/session'
import './style.css'
window.addEventListener('session-expired', () => {
  const redirect = router.currentRoute.value.fullPath
  void clearSession().then(() => {
    if (router.currentRoute.value.name !== 'login')
      void router.replace({ name: 'login', query: { redirect } })
  })
})
createApp(App).use(VueQueryPlugin, { queryClient }).use(router).mount('#app')
