import { createSSRApp, h, type Component } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import RecognitionRules from '../../../frontend/src/features/extraction/RecognitionRules.vue'
import RuleEdit from '../../../frontend/src/features/extraction/RuleEdit.vue'
import { keys } from '../../../frontend/src/app/query'
import type { Rule } from '../../../frontend/src/features/extraction/model'
const titleRule: Rule = {
  id: 'title',
  name: '标题规则',
  order: 10,
  kind: 'script',
  target: 'title',
  source: 'filename',
  enabled: true,
  overwrite: false,
  config: { code: 'function extract(ctx) { return ctx.text; }' },
  examples: [],
}
async function render(rules: Rule[], component: Component, props = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(keys.ruleLibrary, {
    rules,
    defaults: rules,
    catalog: [],
    targets: ['title'],
    sources: ['filename'],
    sourceScripts: {},
    scriptLanguage: 'javascript',
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:any(.*)*', component: { render: () => null } }],
  })
  const app = createSSRApp({ render: () => h(component, props) })
  app.use(VueQueryPlugin, { queryClient: client }).use(router)
  await router.push('/')
  await router.isReady()
  try {
    return await renderToString(app)
  } finally {
    client.clear()
  }
}
describe('recognition rule pages', () => {
  it('lists rules by field without trial or editor clutter', async () => {
    const html = await render([titleRule], RecognitionRules)
    expect(html).toContain('按字段排列的识别规则')
    expect(html).toContain('标题规则')
    expect(html).toContain('保存')
    expect(html).not.toContain('完整文件路径')
    expect(html).not.toContain('JavaScript 脚本')
  })
  it('shows every field even when the chain is empty', async () => {
    const html = await render([], RecognitionRules)
    expect(html).toContain('季号')
    expect(html).toContain('0 条规则')
  })
  it('edits one rule with its own trial on a separate page', async () => {
    const html = await render([titleRule], RuleEdit, { ruleId: 'title' })
    expect(html).toContain('JavaScript 脚本')
    expect(html).toContain('用这条规则试跑')
    expect(html).toContain('保存')
  })
})
