import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, it, expect } from 'vitest'
import ActionMenu from '../../../frontend/src/components/ui/ActionMenu.vue'
import RuleFieldTable from '../../../frontend/src/features/extraction/RuleFieldTable.vue'
import { createRuleDraft } from '../../../frontend/src/features/extraction/useRuleDraft'
import { pipelineRules } from '../../../frontend/src/features/extraction/model'
import { pipeline } from '../fixtures/pipeline'
describe('pipeline actions', () => {
  it('uses native accessible disabled menu triggers', async () => {
    const html = await renderToString(
      h(
        ActionMenu,
        { disabled: true },
        { trigger: () => h('button', { type: 'button' }, '新增规则') },
      ),
    )
    expect(html).toMatch(/<button[^>]*disabled/)
    expect(html).toContain('aria-haspopup="menu"')
  })
  it('keeps stage editors and limits removal to annotations', async () => {
    const html = await renderToString(
      h(RuleFieldTable, {
        drafts: pipelineRules(pipeline()).map(createRuleDraft),
        errors: [],
      }),
    )
    expect(html).toContain('分词')
    expect(html).toContain('建议脚本')
    expect(html.match(/>\s*删除\s*</g)).toHaveLength(1)
    expect(html).not.toContain('覆盖')
  })
})
