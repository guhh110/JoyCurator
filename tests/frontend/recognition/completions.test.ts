import { CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { EditorState } from '@codemirror/state'
import { javascript } from '@codemirror/lang-javascript'
import { describe, expect, it } from 'vitest'
import { scriptCompletions, scriptStandardCompletions } from '../../../frontend/src/features/extraction/scriptCompletions'
import { scriptContextFields, scriptFunctions, scriptMetadataFields } from '../../../frontend/src/features/extraction/scriptReference'

function context(code: string) {
  const pos = code.indexOf('|')
  const doc = code.slice(0, pos) + code.slice(pos + 1)
  return new CompletionContext(EditorState.create({ doc, extensions: [javascript()] }), pos, true)
}
function labels(result: CompletionResult | null) {
  return result?.options.map(option => option.label) || []
}
describe('rule script completions', () => {
  const complete = scriptCompletions()
  it('offers only real readonly context fields after the context dot', () => {
    const result = complete(context('function extract(ctx) { return ctx.| }')) as CompletionResult
    expect(labels(result)).toEqual(['text', 'filename', 'directories', 'hasParent', 'source', 'metadata', 'params'])
    expect(result.from).toBe('function extract(ctx) { return ctx.'.length)
  })
  it('completes nullable metadata and follows a renamed extract parameter', () => {
    const result = complete(context('function extract(input) { return input.metadata.se| }')) as CompletionResult
    expect(labels(result)).toContain('season')
    expect(result.options.find(option => option.label === 'season')?.detail).toContain('null')
    expect(labels(result)).not.toContain('filename')
  })
  it('refreshes parameter fields without exposing invalid dot-property names', () => {
    let names = ['min', 'max', 'invalid-key']
    const source = scriptCompletions(() => names)
    expect(labels(source(context('function extract(ctx) { return ctx.params.| }')) as CompletionResult)).toEqual(['min', 'max'])
    names = ['prefix']
    expect(labels(source(context('function extract(ctx) { return ctx.params.| }')) as CompletionResult)).toEqual(['prefix'])
  })
  it('does not suggest API names in strings or comments', () => {
    expect(complete(context('function extract(ctx) { return "ctx.|" }'))).toBeNull()
    expect(complete(context('function extract(ctx) { // ctx.|\n}'))).toBeNull()
  })
  it('offers helper signatures and snippet insertion without browser globals', () => {
    const result = complete(context('function extract(ctx) { re| }')) as CompletionResult
    const helper = result.options.find(option => option.label === 'reSearch')!
    expect(helper.detail).toContain('Match | null')
    expect(typeof helper.apply).toBe('function')
    const standard = labels(scriptStandardCompletions(context('function extract(ctx) { | }')) as CompletionResult)
    expect(standard).toContain('JSON')
    expect(standard).not.toContain('fetch')
    expect(standard).not.toContain('window')
    expect(standard).not.toContain('document')
  })
  it('completes string methods for the filename field', () => {
    const result = complete(context('function extract(ctx) { return ctx.filename.| }')) as CompletionResult
    expect(labels(result)).toContain('replace')
    expect(labels(result)).toContain('split')
  })
  it('uses the same helper and field documentation as the visible reference', () => {
    const helpers = complete(context('function extract(ctx) { | }')) as CompletionResult
    for (const fn of scriptFunctions) {
      const completion = helpers.options.find(item => item.label === fn.name)!
      expect(completion.detail).toBe(fn.signature)
      expect(completion.info).toBe(fn.summary + fn.description)
    }
    for (const [path, fields] of [['ctx', scriptContextFields], ['ctx.metadata', scriptMetadataFields]] as const) {
      const result = complete(context(`function extract(ctx) { ${path}.| }`)) as CompletionResult
      for (const field of fields) {
        expect(result.options.find(item => item.label === field.name)?.info).toBe(`只读。${field.description}`)
      }
    }
  })
})
