<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { LogOut, Menu, ChevronRight, X } from '@lucide/vue'
import { api } from '../api'
import { provideRuntime } from '../app/runtime'
import { session, clearSession } from '../app/session'
import AppSidebar from '../components/navigation/AppSidebar.vue'
import MobileMenu from '../components/navigation/MobileMenu.vue'
import TaskToast from '../components/jobs/TaskToast.vue'
import RunDetails from '../components/jobs/RunDetails.vue'
const runtime = provideRuntime(),
  route = useRoute(),
  router = useRouter()
const mobileMenu = ref(false),
  scroll = ref<HTMLElement>(),
  leaving = ref(false)
const groupName = computed(() =>
  route.meta.group === 'recognition'
    ? '自动识别'
    : route.meta.group === 'system'
      ? '系统设置'
      : '文件整理',
)
watch(
  () => route.fullPath,
  () => {
    mobileMenu.value = false
    runtime.detailId.value = ''
    runtime.undoing.value = null
    scroll.value?.scrollTo({ top: 0, left: 0 })
  },
)
async function logout() {
  if (leaving.value) return
  leaving.value = true
  try {
    await api('/logout', {})
    await clearSession()
    await router.replace({ name: 'login' })
  } catch (error) {
    runtime.error.value = (error as Error).message
  } finally {
    leaving.value = false
  }
}
</script>
<template>
  <div class="routed-shell">
    <aside class="desktop-sidebar"><AppSidebar /></aside>
    <div class="routed-main">
      <header class="routed-topbar">
        <button
          class="icon-button menu-toggle"
          aria-label="打开导航"
          @click="mobileMenu = true"
        >
          <Menu :size="21" />
        </button>
        <div class="route-breadcrumb">
          <span>{{ groupName }}</span
          ><ChevronRight :size="14" /><strong>{{ route.meta.title }}</strong>
        </div>
        <div class="account">
          <span class="status-dot" /><span>{{ session.username }}</span
          ><button
            class="icon-button"
            aria-label="退出登录"
            :disabled="leaving"
            @click="logout"
          >
            <LogOut :size="16" />
          </button>
        </div>
      </header>
      <div ref="scroll" class="routed-scroll">
        <main
          class="routed-content"
          :class="{ 'routed-workbench': route.name === 'workbench' }"
        >
          <div
            v-if="runtime.error.value"
            class="notice error global-notice"
            role="alert"
          >
            <span>{{ runtime.error.value }}</span
            ><button
              class="icon-button"
              aria-label="关闭错误提示"
              @click="runtime.error.value = ''"
            >
              <X :size="16" />
            </button>
          </div>
          <RouterView v-slot="{ Component }"
            ><KeepAlive
              include="WorkbenchPage,AutomaticRecognitionPage,SettingsPage"
              ><component :is="Component" /></KeepAlive
          ></RouterView>
        </main>
      </div>
    </div>
    <MobileMenu v-model:open="mobileMenu" /><TaskToast /><RunDetails />
  </div>
</template>
<style scoped>
.routed-shell {
  height: 100dvh;
  display: flex;
  overflow: hidden;
}
.desktop-sidebar {
  width: 230px;
  flex-shrink: 0;
  border-right: 1px solid var(--line);
  background: #fff;
}
.routed-main {
  min-width: 0;
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}
.routed-topbar {
  height: 61px;
  min-height: 61px;
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 0 28px;
  border-bottom: 1px solid var(--line);
  background: #fff;
}
.route-breadcrumb {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  color: var(--muted);
  flex: 1;
}
.route-breadcrumb strong {
  color: var(--ink);
  font-weight: 500;
}
.routed-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  overscroll-behavior: contain;
}
.routed-content {
  max-width: 1560px;
  margin: 0 auto;
  padding: 27px 28px 40px;
  min-width: 0;
}
.routed-workbench {
  height: calc(100dvh - 61px);
  min-height: 480px;
  padding-top: 12px;
  padding-bottom: 12px;
}
.menu-toggle {
  display: none;
}
@media (max-width: 900px) {
  .desktop-sidebar {
    width: 205px;
  }
  .routed-topbar {
    padding: 0 20px;
  }
  .routed-content {
    padding-left: 20px;
    padding-right: 20px;
  }
}
@media (max-width: 700px) {
  .desktop-sidebar {
    display: none;
  }
  .menu-toggle {
    display: flex;
  }
  .routed-topbar {
    padding: 0 15px;
    height: 59px;
    min-height: 59px;
  }
  .route-breadcrumb {
    gap: 6px;
    font-size: 11px;
  }
  .route-breadcrumb > span,
  .route-breadcrumb > svg {
    display: none;
  }
  .routed-content {
    padding: 20px 15px 28px;
  }
  .routed-workbench {
    height: calc(100dvh - 59px);
    padding: 13px;
    min-height: 440px;
  }
  .account > span:not(.status-dot) {
    max-width: 90px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
</style>
