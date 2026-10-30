import { snippetCompletion, type Completion, type CompletionSource } from '@codemirror/autocomplete'
import { completionPath, scopeCompletionSource } from '@codemirror/lang-javascript'
import { syntaxTree } from '@codemirror/language'
import type { EditorState } from '@codemirror/state'
import { scriptContextFields, scriptEntry, scriptFunctions, scriptMetadataFields, type ScriptFieldReference } from './scriptReference'

function fieldCompletion(field: ScriptFieldReference): Completion {
  return { label: field.name, type: 'property', detail: `${field.type} · ${field.summary}`, info: `只读。${field.description}` }
}
const contextFields = scriptContextFields.map(fieldCompletion)
const metadataFields = scriptMetadataFields.map(fieldCompletion)

const functions: Completion[] = [
  ...scriptFunctions.map(fn => snippetCompletion(fn.snippet, {
    label: fn.name, type: 'function', detail: fn.signature, info: `${fn.summary}${fn.description}`,
  })),
  snippetCompletion(scriptEntry.snippet, {
    label: scriptEntry.name, type: 'function', detail: '规则入口', info: scriptEntry.description,
  }),
]

function contextName(state: EditorState) {
  for (const declaration of syntaxTree(state).topNode.getChildren('FunctionDeclaration')) {
    const name = declaration.getChild('VariableDefinition')
    if (!name || state.sliceDoc(name.from, name.to) !== 'extract') continue
    const parameter = declaration.getChild('ParamList')?.getChild('VariableDefinition')
    if (parameter) return state.sliceDoc(parameter.from, parameter.to)
  }
  return 'ctx'
}

// Only expose globals actually available in the server runtime, not browser APIs.
const standardScope = scopeCompletionSource({
  Array, Object, String, Number, Boolean, Math, JSON, RegExp, Date, Error,
  Set, Map, parseInt, parseFloat, isNaN, isFinite,
})
const contextMembers = { text: 'text', filename: 'filename', directories: [], hasParent: false, source: 'filename', metadata: { episodes: [] } }

export function scriptCompletions(parameterNames: () => readonly string[] = () => []): CompletionSource {
  return context => {
    const path = completionPath(context)
    if (!path) return null
    const name = contextName(context.state)
    let options: readonly Completion[]
    if (!path.path.length) {
      options = [{ label: name, type: 'variable', detail: '只读规则上下文', boost: 5 }, ...functions]
    } else if (path.path[0] === name && path.path.length === 1) {
      options = contextFields
    } else if (path.path[0] === name && path.path.length === 2 && path.path[1] === 'metadata') {
      options = metadataFields
    } else if (path.path[0] === name && path.path.length === 2 && path.path[1] === 'params') {
      options = parameterNames().filter(key => /^[A-Za-z_$][\w$]*$/.test(key))
        .map(label => ({ label, type: 'property', detail: '规则参数 · 只读' }))
    } else {
      return path.path[0] === name ? scopeCompletionSource({ [name]: contextMembers })(context) : standardScope(context)
    }
    return { from: context.pos - path.name.length, options, validFor: /^[\w$]*$/ }
  }
}

export { standardScope as scriptStandardCompletions }
