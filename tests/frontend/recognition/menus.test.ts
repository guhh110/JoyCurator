import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, expect, it } from 'vitest'
import ActionMenu from '../../../frontend/src/components/ui/ActionMenu.vue'
import RuleFieldTable from '../../../frontend/src/features/extraction/RuleFieldTable.vue'
import { fieldOrder, targets } from '../../../frontend/src/features/extraction/model'

describe('rule action menu triggers', () => {
  it('uses a native button with menu semantics and respects disabled state', async () => {
    const html = await renderToString(h(ActionMenu, { disabled: true }, {
      trigger: () => h('button', { type: 'button' }, '新增规则'),
    }))
    expect(html).toMatch(/<button[^>]*disabled/)
    expect(html).toContain('aria-haspopup="menu"')
    expect(html).toContain('aria-expanded="false"')
    expect(html).not.toContain('<details')
  })
  it.each(fieldOrder)('opens only the selected %s field menu', async target => {
    const html = await renderToString(h(RuleFieldTable, { drafts: [], errors: [], catalog: [], disabled: false, openMenu: target }))
    const buttons = [...html.matchAll(/<button[^>]*aria-label="为([^"]+)新增规则"[^>]*>/g)].map(match => ({ label: match[1], tag: match[0] }))
    expect(buttons).toHaveLength(7)
    expect(buttons.filter(button => button.tag.includes('aria-expanded="true"')).map(button => button.label)).toEqual([targets[target]])
    expect(html).not.toContain('<details')
  })
  it('keeps every field menu closed while the toolbar menu is selected', async () => {
    const html = await renderToString(h(RuleFieldTable, { drafts: [], errors: [], catalog: [], disabled: false, openMenu: 'new' }))
    expect(html.match(/aria-expanded="false"/g)).toHaveLength(7)
    expect(html).not.toContain('aria-expanded="true"')
  })
})
