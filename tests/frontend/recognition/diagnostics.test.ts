import { describe, expect, it } from 'vitest'
import { draftScriptDiagnostics, runtimeDiagnostics, scriptDiagnostics } from '../../../frontend/src/features/extraction/scriptDiagnostics'
import { createRuleDraft } from '../../../frontend/src/features/extraction/useRuleDraft'
import type { Rule, Step } from '../../../frontend/src/features/extraction/model'

const code = 'function extract(ctx) {\n  return missing(ctx.text)\n}'
const rule: Rule = { id: 'title', name: 'Title', target: 'title', source: 'filename', kind: 'script', order: 10, enabled: true, overwrite: false, config: { code }, examples: [] }

describe('script diagnostics', () => {
  it('reports syntax positions including incomplete expressions and end of input', () => {
    const broken = 'function extract(ctx) {\n  return ctx.\n}'
    const [error] = scriptDiagnostics(broken)
    expect(error).toMatchObject({ line: 3, column: 1, phase: 'extract' })
    expect(broken.slice(error!.from, error!.to)).toBe('}')
    const unfinished = 'function extract(ctx) {'
    expect(scriptDiagnostics(unfinished)[0]).toMatchObject({ from: unfinished.length, to: unfinished.length })
  })
  it('requires the synchronous single-parameter entry point', () => {
    for (const invalid of ['', 'const x = 1', 'const extract = ctx => null', 'async function extract(ctx) {}', 'function* extract(ctx) {}', 'function extract(ctx = {}) {}', 'function extract(...ctx) {}', 'function extract({text}) {}'])
      expect(scriptDiagnostics(invalid)).toHaveLength(1)
    expect(scriptDiagnostics(code)).toEqual([])
  })
  it('parses without executing code and enforces the UTF-8 byte limit', () => {
    expect(scriptDiagnostics('throw new Error("must not execute"); function extract(ctx) { return null }')).toEqual([])
    expect(scriptDiagnostics(`function extract(ctx) { /*${'中'.repeat(11000)}*/ }`)[0]?.message).toContain('32 KiB')
  })
  it('checks source scripts separately and leaves regex patterns to the server', () => {
    const draft = createRuleDraft({ ...rule, source: 'cleanFilename' })
    draft.sourceCode = 'function extract(ctx) { return ctx. }'
    expect(draftScriptDiagnostics(draft).map(problem => problem.phase)).toEqual(['source'])
    expect(draftScriptDiagnostics(createRuleDraft({ ...rule, kind: 'regex', config: { pattern: '(?P<value>.+)' } }))).toEqual([])
  })
  it('maps server error events to their script and UTF-16 offsets', () => {
    const step = { ruleConfig: rule, attempts: [{ trace: [
      { phase: 'extract', operation: 'trace', line: 1, column: 1, message: 'input' },
      { phase: 'extract', operation: 'error', line: 2, column: 10, message: 'missing is not defined' },
      { phase: 'source', operation: 'error', line: 0, column: 0, message: 'no location' },
    ] }] } as Step
    expect(runtimeDiagnostics(step)).toEqual([{ phase: 'extract', from: code.indexOf('missing'), to: code.indexOf('missing') + 1, line: 2, column: 10, message: 'missing is not defined' }])
    expect(runtimeDiagnostics(undefined)).toEqual([])
  })
})
