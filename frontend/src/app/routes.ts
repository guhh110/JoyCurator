import type { RouteRecordRaw } from 'vue-router'
export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('../pages/LoginPage.vue'),
    meta: { title: '登录', public: true },
  },
  {
    path: '/',
    component: () => import('../layouts/AppLayout.vue'),
    children: [
      { path: '', redirect: { name: 'workbench' } },
      {
        path: 'organize',
        name: 'workbench',
        component: () => import('../pages/WorkbenchPage.vue'),
        meta: { title: '整理工作台', group: 'organize', keepAlive: true },
      },
      {
        path: 'records',
        name: 'records',
        component: () => import('../pages/RecordsPage.vue'),
        meta: { title: '整理记录', group: 'organize' },
      },
      {
        path: 'recognition',
        name: 'automatic-recognition',
        component: () => import('../pages/AutomaticRecognitionPage.vue'),
        meta: { title: '识别工作区', group: 'recognition', keepAlive: true },
      },
      {
        path: 'recognition/rules',
        name: 'recognition-rules',
        component: () => import('../pages/recognition/RulesPage.vue'),
        meta: { title: '识别规则', group: 'recognition' },
      },
      {
        path: 'recognition/history/:traceId?',
        name: 'extraction-history',
        component: () => import('../pages/recognition/HistoryPage.vue'),
        props: true,
        meta: { title: '执行历史', group: 'recognition' },
      },
      {
        path: 'recognition/debug',
        name: 'recognition-debug',
        component: () => import('../pages/recognition/DebugPage.vue'),
        meta: { title: '识别调试', group: 'recognition' },
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('../pages/SettingsPage.vue'),
        meta: { title: '设置', group: 'system', keepAlive: true },
      },
      {
        path: ':pathMatch(.*)*',
        name: 'not-found',
        component: () => import('../pages/NotFoundPage.vue'),
        meta: { title: '页面不存在' },
      },
    ],
  },
]
