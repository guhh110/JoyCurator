import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, it, expect } from 'vitest'
import ScriptCode from '../../../frontend/src/features/extraction/ScriptCode.vue'
import RuleEditor from '../../../frontend/src/features/extraction/RuleEditor.vue'
import ExtractionResult from '../../../frontend/src/features/extraction/ExtractionResult.vue'
import { createRuleDraft } from '../../../frontend/src/features/extraction/useRuleDraft'
import { annotation, extraction } from '../fixtures/pipeline'
describe('editable scripts and trace evidence', () => {
  it('renders script editor with parameters and stage contract', async () => {
    const html = await renderToString(
      h(RuleEditor, { modelValue: createRuleDraft(annotation()) }),
    )
    expect(html).toContain('script-code-host')
    expect(html).toContain('参数 JSON')
    expect(html).toContain('候选身份')
    expect(html).toContain('UTF-16')
    expect(html).not.toContain('覆盖')
  })
  it('highlights historical source line', async () => {
    const html = await renderToString(
      h(ScriptCode, {
        modelValue: 'function extract(ctx) {\n return []\n}',
        readonly: true,
        activeLine: 2,
      }),
    )
    expect(html).toContain('data-active="true"')
    expect(html).toContain('return []')
  })
  it('shows per-file results, candidates and event locations using shared snapshots', async () => {
    const result = extraction()
    result.documents[0]!.candidates = [
      {
        id: 'title:0',
        ruleId: 'title',
        role: 'title',
        value: 'Film',
        spans: [{ sourceId: 'name', start: 0, end: 4 }],
        strength: 'explicit',
        status: 'valid',
        reason: '片名证据',
      },
    ]
    result.files[0]!.decisions.title = {
      candidates: [{ fileId: 'file-0', candidateId: 'title:0' }],
      reason: '标题候选',
    }
    result.steps = [
      {
        ruleId: 'title',
        ruleName: '片名',
        stage: 'annotate',
        fileId: 'file-0',
        ruleConfig: annotation(),
        order: 10,
        inputs: [],
        status: 'matched',
        extractedValue: [],
        before: null,
        after: null,
        applied: true,
        reason: '',
        startedAt: '',
        durationMs: 1,
        attempts: [
          {
            input: '',
            matched: true,
            trace: [
              {
                phase: 'annotate',
                operation: 'trace',
                line: 2,
                column: 5,
                message: '读取候选',
              },
            ],
          },
        ],
      },
    ]
    const html = await renderToString(h(ExtractionResult, { result }))
    expect(html).toContain('L2:5')
    expect(html).toContain('读取候选')
    expect(html).toContain('片名证据')
    expect(html).toContain('标题候选')
    expect(html).toContain('脚本快照')
  })
})
