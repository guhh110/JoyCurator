import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createRenderer, defineComponent, getCurrentInstance, h, nextTick, ssrContextKey, type Component, type VNode } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { activeConfirmation, dismissConfirmation, resolveConfirmation } from '../../../frontend/src/app/confirm'
import { keys } from '../../../frontend/src/app/query'
import type { Rule, RuleLibrary } from '../../../frontend/src/features/extraction/model'
import type { RuleDraft } from '../../../frontend/src/features/extraction/useRuleDraft'
import RuleEdit from '../../../frontend/src/features/extraction/RuleEdit.vue'
import RecognitionRules from '../../../frontend/src/features/extraction/RecognitionRules.vue'

vi.mock('../../../frontend/src/features/extraction/RuleEditor.vue', () => ({ default: { props: ['modelValue', 'disabled', 'diagnostics', 'focus'], render: () => null } }))
vi.mock('../../../frontend/src/features/extraction/RuleFieldTable.vue', () => ({ default: { props: ['drafts'], render: () => null } }))
vi.mock('../../../frontend/src/components/ui/Tooltip.vue', () => ({ default: { render: () => null } }))
vi.mock('../../../frontend/src/components/ui/EliText.vue', () => ({ default: { render: () => null } }))

// Vitest compiles SFCs for SSR in this Node suite. Exercise the real setup and
// router lifecycle with a minimal host render; browser checks cover the UI.
for (const page of [RuleEdit, RecognitionRules]) {
  page.render = () => {
    const state = getCurrentInstance()!.setupState
    return h('button', { onClick: state.save }, '保存')
  }
}

interface Node { tag: string; props: Record<string, any>; children: Node[]; parent?: Node; text?: string; getRootNode(): object }
const renderRoot = {}
const node = (tag: string): Node => ({ tag, props: {}, children: [], getRootNode: () => renderRoot })
const renderer = createRenderer<Node, Node>({
  createElement: node, createText: text => ({ ...node('#text'), text }), createComment: () => node('#comment'),
  insert(child, parent, anchor) {
    if (child.parent) child.parent.children = child.parent.children.filter(item => item !== child)
    child.parent = parent
    const index = anchor ? parent.children.indexOf(anchor) : -1
    if (index < 0) parent.children.push(child)
    else parent.children.splice(index, 0, child)
  },
  remove(child) { if (child.parent) child.parent.children = child.parent.children.filter(item => item !== child) },
  setText(child, text) { child.text = text },
  setElementText(child, text) { child.text = text; child.children = [] },
  patchProp(child, key, _previous, value) { child.props[key] = value },
  parentNode: child => child.parent ?? null,
  nextSibling: child => child.parent?.children[child.parent.children.indexOf(child) + 1] ?? null,
})
const rule: Rule = { id: 'title', name: 'Title', kind: 'script', target: 'title', source: 'filename', enabled: true, overwrite: false, order: 10, config: { code: 'function extract(ctx) { return ctx.text }' }, examples: [{ path: '/Film.mkv', expected: 'Film' }] }
const cleanup: (() => void)[] = []
let browser: EventTarget
beforeEach(() => { browser = new EventTarget(); vi.stubGlobal('window', browser) })
afterEach(async () => {
  if (activeConfirmation.value) resolveConfirmation(false)
  await nextTick()
  cleanup.splice(0).forEach(fn => fn())
  vi.unstubAllGlobals()
})
const ok = (value: unknown) => ({ ok: true, status: 200, json: async () => value })

async function mount(component: Component = RuleEdit) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } })
  const library: RuleLibrary = { rules: [rule, { ...rule, id: 'second', name: 'Second', order: 20 }], defaults: [rule], catalog: [], targets: ['title'], sources: ['filename'], sourceScripts: {}, scriptLanguage: 'javascript' }
  client.setQueryData(keys.ruleLibrary, JSON.parse(JSON.stringify(library)))
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/rules/:ruleId', component, props: true },
    { path: '/records', name: 'records', component: { render: () => h('p', 'Records') } },
    { path: '/rules', name: 'recognition-rules', component: RecognitionRules },
  ] })
  const app = renderer.createApp(defineComponent({ render: () => h(RouterView) }))
  app.provide(ssrContextKey, {})
  app.use(VueQueryPlugin, { queryClient: client }).use(router)
  await router.push(component === RuleEdit ? '/rules/title' : '/rules')
  await router.isReady()
  const root = node('root')
  // Custom Vue renderers do not assign the DOM renderer's container field.
  ;(app as unknown as { _container: Node })._container = root
  const vm = app.mount(root)
  cleanup.push(() => { app.unmount(); client.clear() })
  await nextTick()
  function findComponent(name: string, vnode: VNode = vm.$.subTree): VNode | undefined {
    if ((vnode.type as { __name?: string }).__name === name) return vnode
    if (vnode.component) { const found = findComponent(name, vnode.component.subTree); if (found) return found }
    if (Array.isArray(vnode.children)) {
      for (const child of vnode.children) if (child && typeof child === 'object') {
        const found = findComponent(name, child as VNode)
        if (found) return found
      }
    }
  }
  function state() { return findComponent(component === RuleEdit ? 'RuleEdit' : 'RecognitionRules')!.component!.setupState }
  function draft() { return state().draft as RuleDraft }
  async function edit() { draft().rule.name = 'Changed'; await nextTick() }
  function allNodes(parent = root): Node[] { return [parent, ...parent.children.flatMap(child => allNodes(child))] }
  function text(parent: Node): string { return (parent.text ?? '') + parent.children.map(text).join('') }
  async function save() {
    const button = allNodes().find(item => item.tag === 'button' && /^保存/.test(text(item).trim()))!
    return button.props.onClick()
  }
  return { router, client, draft, edit, save, state, unmount: () => app.unmount() }
}

describe('restore default rule confirmation', () => {
  it('leaves the draft, validation results and saved chain untouched when cancelled', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const { state, client } = await mount(RecognitionRules)
    state().drafts[0].rule.name = 'Unsaved name'
    state().activeMenu = 'title'
    state().checkedKey = 'existing-validation'
    state().exampleResults = [{ ruleId: 'title', path: '/Film.mkv', expected: 'Film', actual: 'Film', passed: true, error: '' }]
    const before = JSON.stringify(state().drafts)
    const saved = JSON.stringify(client.getQueryData(keys.ruleLibrary))
    const pending = state().reset()
    expect(activeConfirmation.value?.options.title).toBe('恢复默认规则？')
    expect(activeConfirmation.value?.options.description).toContain('保存')
    expect(state().activeMenu).toBe('')
    expect(state().busy).toBe(true)
    expect(state().canSubmit).toBe(false)
    resolveConfirmation(false)
    await pending
    expect(JSON.stringify(state().drafts)).toBe(before)
    expect(JSON.stringify(client.getQueryData(keys.ruleLibrary))).toBe(saved)
    expect(state().busy).toBe(false)
    expect(state().checkedKey).toBe('existing-validation')
    expect(state().exampleResults).toHaveLength(1)
    expect(fetch).not.toHaveBeenCalled()
  })
  it('replaces only the draft after confirmation and requires a separate save', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const { state, client } = await mount(RecognitionRules)
    state().drafts[0].rule.name = 'Unsaved name'
    state().error = 'Earlier error'
    state().checkedKey = 'existing-validation'
    state().exampleResults = [{ ruleId: 'title', path: '/Film.mkv', expected: 'Film', actual: 'Film', passed: true, error: '' }]
    const saved = JSON.stringify(client.getQueryData(keys.ruleLibrary))
    const pending = state().reset()
    await state().reset()
    await state().save()
    resolveConfirmation(true)
    await pending
    expect(activeConfirmation.value).toBeNull()
    expect(state().drafts).toHaveLength(1)
    expect(state().drafts[0].rule).toEqual(rule)
    expect(state().dirty).toBe(true)
    expect(state().notice).toBe('默认规则已载入，保存后生效')
    expect(state().error).toBe('')
    expect(state().checkedKey).toBe('')
    expect(state().exampleResults).toEqual([])
    expect(JSON.stringify(client.getQueryData(keys.ruleLibrary))).toBe(saved)
    expect(fetch).not.toHaveBeenCalled()
  })
  it('does not restore defaults while saving or checking examples', async () => {
    const { state } = await mount(RecognitionRules)
    for (const flag of ['submitting', 'checking']) {
      state()[flag] = true
      await state().reset()
      expect(activeConfirmation.value).toBeNull()
      expect(state().drafts).toHaveLength(2)
      state()[flag] = false
    }
  })
  it('ignores a late confirmation after the rules page unmounts', async () => {
    const { state, unmount } = await mount(RecognitionRules)
    const page = state()
    const pending = page.reset()
    unmount()
    resolveConfirmation(true)
    await pending
    expect(page.drafts).toHaveLength(2)
    expect(page.notice).toBe('')
  })
})

describe('unsaved rule navigation', () => {
  it('keeps drafts on cancelled navigation and blocks refresh only while dirty', async () => {
    const { router, client, edit, draft } = await mount()
    const clean = new Event('beforeunload', { cancelable: true })
    browser.dispatchEvent(clean)
    expect(clean.defaultPrevented).toBe(false)
    await edit()
    const dirty = new Event('beforeunload', { cancelable: true })
    Object.defineProperty(dirty, 'returnValue', { value: '', writable: true })
    browser.dispatchEvent(dirty)
    expect(dirty.defaultPrevented).toBe(true)
    client.setQueryData(keys.ruleLibrary, old => ({ ...(old as RuleLibrary), rules: [rule] }))
    await nextTick()
    expect(draft().rule.name).toBe('Changed')
    const leaving = router.push('/records')
    await vi.waitFor(() => expect(activeConfirmation.value).not.toBeNull())
    dismissConfirmation()
    await leaving
    expect(router.currentRoute.value.path).toBe('/rules/title')
    expect(draft().rule.name).toBe('Changed')
  })
  it('guards reused rule routes and allows navigation after explicit confirmation', async () => {
    const { router, edit, draft } = await mount()
    await edit()
    const update = router.push('/rules/second')
    await vi.waitFor(() => expect(activeConfirmation.value?.options.title).toBe('放弃这条规则的修改？'))
    resolveConfirmation(false)
    await update
    expect(draft().rule.name).toBe('Changed')
    const confirmed = router.push('/rules/second')
    await vi.waitFor(() => expect(activeConfirmation.value).not.toBeNull())
    resolveConfirmation(true)
    await confirmed
    await nextTick()
    expect(draft().rule.id).toBe('second')
    expect(draft().rule.name).toBe('Second')
  })
  it('guards rule-list changes', async () => {
    const { router, state } = await mount(RecognitionRules)
    state().drafts[0].rule.enabled = false
    await nextTick()
    const leaving = router.push('/records')
    await vi.waitFor(() => expect(activeConfirmation.value?.options.title).toBe('放弃规则列表的修改？'))
    resolveConfirmation(true)
    await leaving
    expect(router.currentRoute.value.path).toBe('/records')
  })
  it('blocks browser back until confirmed', async () => {
    const { router, edit } = await mount()
    await router.push('/records')
    await router.push('/rules/title')
    await edit()
    router.back()
    await vi.waitFor(() => expect(activeConfirmation.value).not.toBeNull())
    resolveConfirmation(false)
    await vi.waitFor(() => expect(activeConfirmation.value).toBeNull())
    expect(router.currentRoute.value.path).toBe('/rules/title')
    await nextTick()
  })
  it('does not prompt for query-only navigation and removes the refresh handler on unmount', async () => {
    const { router, edit } = await mount()
    await edit()
    await router.push('/rules/title?line=2')
    expect(activeConfirmation.value).toBeNull()
    const leaving = router.push('/records')
    await vi.waitFor(() => expect(activeConfirmation.value).not.toBeNull())
    resolveConfirmation(true)
    await leaving
    const refresh = new Event('beforeunload', { cancelable: true })
    browser.dispatchEvent(refresh)
    expect(refresh.defaultPrevented).toBe(false)
  })
})

describe('rule save preflight', () => {
  it('does not save a null-valued sample that failed because of an execution error', async () => {
    const fetch = vi.fn().mockResolvedValue(ok([{ ruleId: 'title', path: '/Film.mkv', expected: null, actual: null, passed: false, error: '规则 title：sample failure' }]))
    vi.stubGlobal('fetch', fetch)
    const { edit, save } = await mount()
    await edit()
    await save()
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/extraction/examples'])
  })
  it('does not PUT a chain whose examples fail', async () => {
    const fetch = vi.fn().mockResolvedValue(ok([{ ruleId: 'title', path: '/Film.mkv', expected: 'Film', actual: 'Other', passed: false, error: '' }]))
    vi.stubGlobal('fetch', fetch)
    const { edit, save } = await mount()
    await edit()
    await save()
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/extraction/examples'])
  })
  it('validates once before PUT and marks the exact saved draft clean', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(ok([])).mockResolvedValueOnce(ok({ ok: true }))
      .mockResolvedValue(ok({ rules: [rule], sourceScripts: {}, catalog: [] }))
    vi.stubGlobal('fetch', fetch)
    const { edit, save, router } = await mount()
    await edit()
    await save()
    expect(fetch.mock.calls.slice(0, 2).map(call => [call[0], call[1].method])).toEqual([
      ['/api/extraction/examples', 'POST'], ['/api/extraction/rules', 'PUT'],
    ])
    expect(JSON.parse(fetch.mock.calls[1]![1].body).rules[0].name).toBe('Changed')
    await router.push('/records')
    expect(activeConfirmation.value).toBeNull()
  })
  it('rejects saving a draft edited while its examples are running', async () => {
    let finish!: (value: unknown) => void
    const fetch = vi.fn(() => new Promise(resolve => { finish = resolve }))
    vi.stubGlobal('fetch', fetch)
    const { edit, save, draft } = await mount()
    await edit()
    const pending = save()
    await nextTick()
    draft().rule.name = 'Changed again'
    finish(ok([]))
    await pending
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('keeps a dirty draft when the save request fails', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(ok([]))
      .mockResolvedValueOnce({ ok: false, status: 500, json: async () => ({ message: '保存失败' }) })
    vi.stubGlobal('fetch', fetch)
    const { edit, save, router, draft, state } = await mount()
    await edit()
    await save()
    expect(state().error).toBe('保存失败')
    expect(draft().rule.name).toBe('Changed')
    const leaving = router.push('/records')
    await vi.waitFor(() => expect(activeConfirmation.value).not.toBeNull())
    resolveConfirmation(false)
    await leaving
    expect(router.currentRoute.value.path).toBe('/rules/title')
  })
})
