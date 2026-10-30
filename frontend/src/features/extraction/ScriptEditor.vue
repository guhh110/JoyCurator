<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { Compartment, EditorSelection, EditorState, StateEffect, StateField, Transaction } from '@codemirror/state'
import { lintGutter, setDiagnostics } from '@codemirror/lint'
import Tooltip from '../../components/ui/Tooltip.vue'
import { CircleHelp } from '@lucide/vue'
import type { ScriptDiagnostic } from './scriptDiagnostics'
import { Decoration, EditorView, drawSelection, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers, tooltips, type DecorationSet } from '@codemirror/view'
import { bracketMatching, defaultHighlightStyle, foldGutter, foldKeymap, indentOnInput, syntaxHighlighting } from '@codemirror/language'
import { javascript, javascriptLanguage } from '@codemirror/lang-javascript'
import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { highlightSelectionMatches, searchKeymap } from '@codemirror/search'
import { scriptCompletions, scriptStandardCompletions } from './scriptCompletions'

const props = defineProps<{
  modelValue: string
  activeLine?: number
  focusKey?: number
  label?: string
  id?: string
  parameterNames?: string[]
  diagnostics?: ScriptDiagnostic[]
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const host = ref<HTMLElement>()
const helpId = useId()
let editor: EditorView | undefined
const editable = new Compartment()
const markLine = StateEffect.define<number>()
const highlightedLine = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(value, transaction) {
    value = value.map(transaction.changes)
    for (const effect of transaction.effects) {
      if (effect.is(markLine)) {
        const line = transaction.state.doc.line(Math.min(effect.value, transaction.state.doc.lines))
        value = Decoration.set([Decoration.line({ class: 'cm-trace-line' }).range(line.from)])
      }
    }
    return value
  },
  provide: field => EditorView.decorations.from(field),
})

function locate() {
  if (!editor || !props.activeLine || props.activeLine < 1) return
  const line = editor.state.doc.line(Math.min(Math.floor(props.activeLine), editor.state.doc.lines))
  editor.dispatch({
    selection: EditorSelection.range(line.from, line.to),
    effects: [markLine.of(line.number), EditorView.scrollIntoView(line.from, { y: 'center' })],
  })
  editor.contentDOM.focus({ preventScroll: true })
}
function updateDiagnostics() {
  if (!editor) return
  const length = editor.state.doc.length
  editor.dispatch(setDiagnostics(editor.state, (props.diagnostics ?? []).map(problem => ({
    from: Math.min(problem.from, length), to: Math.min(problem.to, length),
    severity: 'error' as const, message: problem.message,
  }))))
}
onMounted(() => {
  editor = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        lineNumbers(), highlightActiveLineGutter(), foldGutter(), drawSelection(),
        highlightActiveLine(), history(), indentOnInput(), bracketMatching(), closeBrackets(),
        syntaxHighlighting(defaultHighlightStyle), javascript(), highlightSelectionMatches(),
        autocompletion(), javascriptLanguage.data.of({ autocomplete: scriptCompletions(() => props.parameterNames || []) }),
        javascriptLanguage.data.of({ autocomplete: scriptStandardCompletions }),
        keymap.of([...closeBracketsKeymap, ...completionKeymap, ...defaultKeymap, ...historyKeymap, ...searchKeymap, ...foldKeymap, indentWithTab]),
        highlightedLine, tooltips({ position: 'fixed' }),
        lintGutter(), editable.of([EditorView.editable.of(!props.disabled), EditorState.readOnly.of(!!props.disabled)]),
        EditorView.contentAttributes.of({
          'aria-label': props.label || 'JavaScript 脚本', 'aria-describedby': helpId,
          spellcheck: 'false', autocapitalize: 'off', ...(props.id ? { id: props.id } : {}),
        }),
        EditorView.updateListener.of(update => {
          if (update.docChanged) emit('update:modelValue', update.state.doc.toString())
        }),
        EditorView.theme({
          '&': { fontSize: '12px', backgroundColor: '#f7f8fb', color: '#252735' },
          '&.cm-focused': { outline: 'none' },
          '.cm-scroller': { fontFamily: "'SFMono-Regular', Consolas, monospace", lineHeight: '20px', height: '380px', minHeight: '160px', maxHeight: '640px', resize: 'vertical', overflow: 'auto', scrollbarGutter: 'stable' },
          '.cm-content': { padding: '12px 0' }, '.cm-line': { padding: '0 12px' },
          '.cm-gutters': { backgroundColor: '#eeeff5', color: '#7d7789', borderRight: '1px solid #e3e3ec' },
          '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: '#ece9f6' },
          '.cm-trace-line': { backgroundColor: '#eee5ff', boxShadow: 'inset 3px 0 #7261cf' },
          '.cm-tooltip': { border: '1px solid #e3dfed', borderRadius: '7px', backgroundColor: '#fff', color: '#252735', boxShadow: '0 6px 24px #29204420' },
          '.cm-tooltip-autocomplete ul li[aria-selected]': { backgroundColor: '#eee9fc', color: '#5d4db8' },
          '.cm-completionDetail': { marginLeft: '12px', color: '#7d7789' },
        }),
      ],
    }),
  })
  updateDiagnostics()
  locate()
})
watch(() => props.modelValue, value => {
  if (!editor || editor.state.doc.toString() === value) return
  editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value }, annotations: Transaction.addToHistory.of(false) })
})
watch(() => [props.activeLine, props.focusKey], locate, { flush: 'post' })
watch(() => props.diagnostics, updateDiagnostics, { flush: 'post' })
watch(() => props.disabled, value => editor?.dispatch({ effects: editable.reconfigure([EditorView.editable.of(!value), EditorState.readOnly.of(!!value)]) }))
onBeforeUnmount(() => { editor?.destroy(); editor = undefined })
</script>

<template>
  <div class="script-code-editor">
    <div ref="host" class="script-code-host" :aria-label="label || 'JavaScript 脚本'" />
    <div class="script-code-help">
      <span>JavaScript · {{ diagnostics?.length ? `${diagnostics.length} 处错误` : '输入时自动补全' }}</span>
      <Tooltip content="Ctrl + Space 手动提示 · Enter 采纳 · Esc 关闭 · Tab 缩进；Esc 后按 Tab 移出编辑器。支持查找和撤销。">
        <button type="button" class="help-button" aria-label="编辑器快捷键"><CircleHelp :size="14" /></button>
      </Tooltip>
      <span :id="helpId" class="sr-only">Ctrl + Space 手动提示，Enter 采纳，Esc 关闭，Tab 缩进；Esc 后按 Tab 移出编辑器。</span>
    </div>
  </div>
</template>

<style scoped>
.script-code-editor { min-width:0; border:1px solid var(--line); border-radius:8px; overflow:hidden; background:#f7f8fb; }
.script-code-host { min-width:0; }
.script-code-help { display:flex; align-items:center; justify-content:space-between; gap:8px; margin:0; padding:5px 12px; border-top:1px solid var(--line); color:var(--muted); font-size:11px; line-height:1.6; }
.help-button { display:inline-flex; padding:4px; border:0; background:transparent; color:var(--muted); cursor:help; }
.sr-only { position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip:rect(0, 0, 0, 0); white-space:nowrap; border:0; }
</style>
