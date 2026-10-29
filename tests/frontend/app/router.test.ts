import { createSSRApp, h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import {
  createMemoryHistory,
  createRouter,
  type RouteRecordRaw,
} from 'vue-router'
import { describe, expect, it } from 'vitest'
import { routes } from '../../../frontend/src/app/routes'
import { installGuards } from '../../../frontend/src/app/routerGuards'
import { safeRedirect } from '../../../frontend/src/app/session'
import { pageNumber } from '../../../frontend/src/app/routing'
import { groupForRoute, navigation } from '../../../frontend/src/app/navigation'
import AppSidebar from '../../../frontend/src/components/navigation/AppSidebar.vue'
const dummy = { render: () => h('div') }
function stubRoutes(records: RouteRecordRaw[]): RouteRecordRaw[] {
  return records.map((record) => ({
    ...record,
    ...('component' in record ? { component: dummy } : {}),
    ...('children' in record && record.children
      ? { children: stubRoutes(record.children) }
      : {}),
  })) as RouteRecordRaw[]
}
function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: stubRoutes(routes),
  })
}
describe('independent pages and grouped navigation', () => {
  it('resolves the three tools and trace details to separate URLs', () => {
    const router = makeRouter()
    const paths = [
      '/recognition/rules',
      '/recognition/history',
      '/recognition/debug',
    ]
    expect(paths.map((path) => router.resolve(path).name)).toEqual([
      'recognition-rules',
      'extraction-history',
      'recognition-debug',
    ])
    const detail = router.resolve('/recognition/history/trace-123?page=2')
    expect(detail.params.traceId).toBe('trace-123')
    expect(detail.query.page).toBe('2')
    for (const path of paths)
      expect(router.resolve(path).matched).toHaveLength(2)
    expect(router.resolve('/missing-page').name).toBe('not-found')
  })
  it('protects bookmarked pages and returns a safe login destination', async () => {
    const router = makeRouter()
    let username = ''
    installGuards(router, async () => ({ initialized: true, username }))
    await router.push('/recognition/history/trace-123?page=2')
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe(
      '/recognition/history/trace-123?page=2',
    )
    username = 'test'
    await router.replace(safeRedirect(router.currentRoute.value.query.redirect))
    expect(router.currentRoute.value.params.traceId).toBe('trace-123')
    await router.push('/login')
    expect(router.currentRoute.value.name).toBe('workbench')
  })
  it('opens the matching accordion group and exposes direct router links', async () => {
    const router = makeRouter()
    await router.push('/recognition/debug')
    const app = createSSRApp(AppSidebar)
    app.use(router)
    const html = await renderToString(app)
    expect(html).toContain('分组导航')
    expect(html).toContain('href="/recognition/rules"')
    expect(html).toContain('href="/recognition/history"')
    expect(html).toContain('href="/recognition/debug"')
    expect(html).toContain('aria-current="page"')
    expect(groupForRoute('recognition-debug')).toBe('recognition')
    expect(navigation.map((group) => group.title)).toEqual([
      '文件整理',
      '自动识别',
      '系统设置',
    ])
  })
  it.each([
    'https://example.invalid',
    '//example.invalid',
    '/\\example.invalid',
    '/\t/example.invalid',
    '/login',
    '/login?redirect=/login',
  ])('rejects unsafe or looping redirects %s', (value) => {
    expect(safeRedirect(value)).toBe('/organize')
  })
  it('preserves a local target and validates pagination parameters', () => {
    expect(safeRedirect('/recognition/rules')).toBe('/recognition/rules')
    expect(pageNumber('2')).toBe(2)
    for (const value of ['-1', '0', 'Infinity', '2.5', 'abc', 1000001, ['2']])
      expect(pageNumber(value)).toBe(1)
  })
})
