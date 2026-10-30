import { parse, type FunctionDeclaration } from 'acorn'
import type { Step } from './model'
import type { RuleDraft } from './useRuleDraft'

export interface ScriptDiagnostic {
  phase: 'extract' | 'source'
  from: number
  to: number
  line: number
  column: number
  message: string
}

// Parse only. Draft scripts must never execute in the browser.
export function scriptDiagnostics(code: string, phase: ScriptDiagnostic['phase'] = 'extract'): ScriptDiagnostic[] {
  function problem(from: number, to: number, message: string): ScriptDiagnostic[] {
    const prefix = code.slice(0, from).split('\n')
    return [{ phase, from, to, line: prefix.length, column: prefix.at(-1)!.length + 1, message }]
  }
  if (new TextEncoder().encode(code).length > 32768)
    return problem(0, 0, '脚本不能超过 32 KiB')
  try {
    const tree = parse(code, { ecmaVersion: 'latest', sourceType: 'script' })
    const functions = tree.body.filter((node): node is FunctionDeclaration =>
      node.type === 'FunctionDeclaration' && node.id?.name === 'extract',
    )
    if (!functions.length) return problem(0, 0, '脚本必须定义 function extract(ctx)')
    const invalid = functions.find(fn => fn.async || fn.generator || fn.params.length !== 1 || fn.params[0]?.type !== 'Identifier')
    if (invalid) return problem(invalid.start, invalid.end, 'extract 必须是同步函数，且只接受一个普通参数')
    return []
  } catch (error) {
    const syntax = error as SyntaxError & { pos: number }
    const from = Math.max(0, Math.min(syntax.pos ?? 0, code.length))
    return problem(from, Math.min(from + 1, code.length), `语法错误：${syntax.message.replace(/ \(\d+:\d+\)$/, '')}`)
  }
}

export function draftScriptDiagnostics(draft: RuleDraft): ScriptDiagnostic[] {
  return [
    ...(draft.rule.kind === 'script' ? scriptDiagnostics(draft.rule.config.code ?? '') : []),
    ...(draft.rule.source === 'cleanFilename' ? scriptDiagnostics(draft.sourceCode, 'source') : []),
  ]
}

export function runtimeDiagnostics(step: Step | undefined): ScriptDiagnostic[] {
  return step?.attempts.flatMap(attempt => (attempt.trace ?? [])
    .filter(event => event.operation === 'error' && event.line > 0 && (event.phase === 'extract' || event.phase === 'source'))
    .map(event => {
      const phase = event.phase as ScriptDiagnostic['phase']
      const code = (phase === 'source' ? step.ruleConfig.config.sourceCode : step.ruleConfig.config.code) ?? ''
      const lines = code.split('\n')
      const line = Math.min(event.line, lines.length)
      const column = Math.max(1, Math.min(event.column, lines[line - 1]!.length + 1))
      const from = lines.slice(0, line - 1).reduce((offset, text) => offset + text.length + 1, 0) + column - 1
      return { phase, from, to: Math.min(from + 1, code.length), line, column, message: event.message }
    })) ?? []
}
