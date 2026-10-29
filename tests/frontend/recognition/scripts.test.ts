import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, expect, it } from 'vitest'
import ScriptCode from '../../../frontend/src/features/extraction/ScriptCode.vue'
import RuleEditor from '../../../frontend/src/features/extraction/RuleEditor.vue'
import ExtractionResult from '../../../frontend/src/features/extraction/ExtractionResult.vue'
import {
  emptyScript,
  type Rule,
  type ExtractionResult as Result,
} from '../../../frontend/src/features/extraction/model'
const rule: Rule = {
  id: 'script',
  name: '脚本',
  target: 'title',
  source: 'filename',
  order: 1,
  enabled: true,
  kind: 'script',
  overwrite: false,
  config: { code: 'function extract(ctx) {\n  return ctx.text;\n}', params: {} },
  examples: [],
}
describe('observable script editor and snapshots', () => {
  it('renders editable complete source and script parameters', async () => {
    const html = await renderToString(
      h(RuleEditor, { rule, sourceScripts: {} }),
    )
    expect(html).toContain('JavaScript 脚本')
    expect(html).toContain('return ctx.text')
    expect(html).toContain('rule-params')
    expect(html).not.toContain('内置处理器')
    expect(emptyScript).toContain('trace(')
    expect(emptyScript).toContain('function extract(ctx)')
    expect(emptyScript).toContain('return null')
  })
  it('renders source line numbers and highlights the referenced line', async () => {
    const html = await renderToString(
      h(ScriptCode, {
        modelValue: rule.config.code!,
        readonly: true,
        activeLine: 2,
      }),
    )
    expect(html).toContain('data-active="true"')
    expect(html).toContain('>2</span>')
    expect(html).toContain('return ctx.text')
  })
  it('shows event locations and the actual historical script snapshot', async () => {
    const result: Result = {
      metadata: {
        tmdbId: null,
        title: 'Film',
        mediaType: null,
        year: null,
        season: null,
        episodes: [],
        releaseTag: null,
      },
      steps: [
        {
          ruleId: rule.id,
          ruleName: rule.name,
          ruleConfig: rule,
          order: 1,
          target: 'title',
          inputs: ['Film'],
          status: 'matched',
          matchedText: 'Film',
          captures: {},
          extractedValue: 'Film',
          before: null,
          after: 'Film',
          applied: true,
          reason: '提取成功',
          startedAt: '2026-01-01',
          durationMs: 1,
          attempts: [
            {
              input: 'Film',
              matched: true,
              trace: [
                {
                  phase: 'extract',
                  line: 2,
                  column: 5,
                  function: 'extract',
                  operation: 'trace',
                  message: '输入文本',
                  data: 'Film',
                },
              ],
            },
          ],
        },
      ],
    }
    const html = await renderToString(h(ExtractionResult, { result }))
    expect(html).toContain('L2:5')
    expect(html).toContain('输入文本')
    expect(html).toContain('本次完整脚本（快照）')
    expect(html).toContain('return ctx.text')
  })
})
