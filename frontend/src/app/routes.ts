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
      { path: '', redirect: { name: 'automatic-recognition' } },
      {
        path: 'records',
        name: 'records',
        component: () => import('../pages/RecordsPage.vue'),
        meta: { title: '整理记录', group: 'records' },
      },
      {
        path: 'recognition',
        name: 'automatic-recognition',
        component: () => import('../pages/AutomaticRecognitionPage.vue'),
        meta: { title: '整理工作区', group: 'workspace', keepAlive: true },
      },
      {
        path: 'recognition/rules',
        name: 'recognition-rules',
        component: () => import('../pages/recognition/RulesPage.vue'),
        meta: { title: '识别规则', group: 'config' },
      },
      {
        // `new` creates a rule; ?target=&kind=&template= describe it.
        path: 'recognition/rules/:ruleId',
        name: 'recognition-rule-edit',
        component: () => import('../pages/recognition/RuleEditPage.vue'),
        props: (route) => ({
          ruleId: String(route.params.ruleId),
          ...Object.fromEntries(
            ['target', 'kind', 'template', 'line'].flatMap((key) =>
              typeof route.query[key] === 'string'
                ? [[key, route.query[key]]]
                : [],
            ),
          ),
        }),
        meta: { title: '编辑规则', group: 'config' },
      },
      {
        path: 'recognition/history/:traceId?',
        name: 'extraction-history',
        component: () => import('../pages/recognition/HistoryPage.vue'),
        props: true,
        meta: { title: '识别历史', group: 'records' },
      },
      {
        path: 'recognition/debug',
        name: 'recognition-debug',
        component: () => import('../pages/recognition/DebugPage.vue'),
        meta: { title: '识别调试', group: 'config' },
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('../pages/SettingsPage.vue'),
        meta: { title: '设置', group: 'config', keepAlive: true },
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
