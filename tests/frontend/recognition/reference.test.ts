import { h, type Ref } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ScriptReference from '../../../frontend/src/features/extraction/ScriptReference.vue'
import { scriptFunctions, scriptReturnTypes } from '../../../frontend/src/features/extraction/scriptReference'
import { scriptDiagnostics } from '../../../frontend/src/features/extraction/scriptDiagnostics'
import { targets, type Target } from '../../../frontend/src/features/extraction/model'

interface ReferenceState {
  open: Ref<boolean>
  category: Ref<string>
  copyStatus: Ref<Record<string, string>>
  copying: Ref<Record<string, boolean>>
  copyExample(name: string, code: string): Promise<void>
}

// Keep the actual SSR template and setup; select a reference section before
// rendering so this Node suite can check content without adding a DOM runtime.
async function renderReference(category = 'functions', target: Target = 'year', phase = 'extract') {
  let state!: ReferenceState
  const component = {
    ...ScriptReference,
    setup(props: unknown, context: unknown) {
      state = (ScriptReference as any).setup(props, context)
      state.open.value = true
      state.category.value = category
      return state
    },
  }
  const html = await renderToString(h(component, { label: '测试脚本', editorId: 'test-code', target, phase }))
  return { html, state }
}

afterEach(() => vi.unstubAllGlobals())

describe('discoverable script reference', () => {
  it.each(['extract', 'source'] as const)('keeps one collapsed reference entry after the %s hint, not in the heading', async phase => {
    const html = await renderToString(h(ScriptReference, { label: 'JavaScript 脚本', editorId: 'test-code', target: 'year', phase }, { default: () => h('div', { id: 'editor-slot' }) }))
    const heading = html.match(/<div class="reference-heading"[^>]*>(.*?)<\/div>/s)![1]!
    const hint = html.match(/<p class="reference-hint"[^>]*>(.*?)<\/p>/s)![1]!
    expect(heading).not.toContain('<button')
    expect(hint).toContain('脚本参考')
    expect(hint).toContain(phase === 'source' ? '返回清理后的字符串' : '返回字段值，未命中返回')
    expect(hint.indexOf('脚本参考')).toBeGreaterThan(hint.indexOf('返回'))
    expect(html.match(/aria-controls=/g)).toHaveLength(1)
    expect(html).not.toContain('5 个辅助函数')
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('display:none')
    expect(html).toContain('id="editor-slot"')
    expect(html).toContain('for="test-code"')
  })
  it('documents all helpers with complete valid examples and their applicable fields', async () => {
    const { html } = await renderReference()
    expect(scriptFunctions.map(fn => fn.name)).toEqual(['reSearch', 'reFindAll', 'reSub', 'trace', 'print'])
    for (const fn of scriptFunctions) {
      expect(html).toContain(`复制 ${fn.name} 示例`)
      expect(html).toContain(fn.example.usage)
      expect(scriptDiagnostics(fn.example.code)).toEqual([])
    }
    expect(html).toContain('UTF-16')
    expect(html).toContain('groups')
    expect(html).toContain('captures')
    expect(html).toContain('Go RE2')
  })
  it('keeps the entry connected to the expanded panel and selected category', async () => {
    const { html } = await renderReference('runtime')
    const panelId = html.match(/<section[^>]* id="([^"]+)"/)![1]!
    expect(html).toContain(`aria-controls="${panelId}"`)
    expect(html).toContain('aria-expanded="true"')
    expect(html).not.toContain('display:none')
    expect(html).toContain('执行限制')
  })
  it('explains readonly inputs, metadata and runtime boundaries', async () => {
    const { html: context } = await renderReference('context')
    for (const field of ['text', 'filename', 'directories', 'hasParent', 'source', 'metadata', 'params']) expect(context).toContain(`ctx.${field}`)
    expect(context).toContain('只读')
    expect(context).toContain('未知单值为 null')
    const { html: runtime } = await renderReference('runtime')
    for (const text of ['Go RE2', 'JavaScript', '250 ms', '32 KiB', '128', 'fetch', '反向引用']) expect(runtime).toContain(text)
    expect(runtime).toContain('&quot;\\\\d+&quot;')
  })
  it.each(Object.keys(targets) as Target[])('shows the current return contract for %s', async target => {
    const { html } = await renderReference('returns', target)
    expect(html).toContain(`当前字段：${targets[target]}`)
    expect(html).toContain(scriptReturnTypes[target].description.replaceAll('"', '&quot;'))
    expect(html).toContain('matchedText')
    expect(html).toContain('captures')
    expect(html).toContain('未命中统一返回')
    expect(html).not.toContain('来源清理返回示例')
  })
  it('distinguishes source-cleaning returns from extraction returns', async () => {
    const { html } = await renderReference('returns', 'year', 'source')
    expect(html).toContain('必须返回清理后的字符串')
    expect(html).toContain('不要返回')
    expect(html).not.toContain('当前字段：年份')
    expect(html).not.toContain('详细返回值示例')
    const { html: functions } = await renderReference('functions', 'year', 'source')
    expect(functions).toContain('以下示例用于字段提取')
  })
  it('copies the complete example and reports denied or unavailable clipboard access without changing code', async () => {
    const { state } = await renderReference()
    const example = scriptFunctions[0]!
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await state.copyExample(example.name, example.example.code)
    expect(writeText).toHaveBeenCalledWith(example.example.code)
    expect(state.copyStatus.value.reSearch).toContain('已复制示例')
    expect(state.copying.value.reSearch).toBe(false)

    writeText.mockRejectedValue(new Error('Permission denied'))
    await state.copyExample(example.name, example.example.code)
    expect(state.copyStatus.value.reSearch).toContain('手动复制')
    expect(state.copying.value.reSearch).toBe(false)

    vi.stubGlobal('navigator', {})
    await state.copyExample(example.name, example.example.code)
    expect(state.copyStatus.value.reSearch).toContain('手动复制')
  })
})
