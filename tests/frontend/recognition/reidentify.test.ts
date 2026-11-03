import { effectScope, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { canReidentify, useReidentification } from '../../../frontend/src/features/workspace/useReidentification'
import type { Recognition, RecognitionItem } from '../../../frontend/src/model'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach(close => close()))
const file = (path: string, extra: Partial<RecognitionItem> = {}) => ({ path, removed: false, ...extra } as RecognitionItem)
function fixture(commit = vi.fn(async () => true)) {
  const task = ref({ id: 'workspace', revision: 7 } as Recognition)
  const locked = ref(false)
  const scope = effectScope()
  const state = scope.run(() => useReidentification({ task, locked, commit }))!
  cleanups.push(() => scope.stop())
  return { task, locked, state, commit }
}
describe('selected reidentification', () => {
  it('selects across the full group and excludes removed or submitted files', () => {
    const { state } = fixture()
    const items = Array.from({ length: 60 }, (_, i) => file(`/source/${i}.mkv`))
    const reserved = file('/locked', { execution: { locked: true } as RecognitionItem['execution'] })
    const completed = file('/done', { execution: { completed: true } as RecognitionItem['execution'] })
    const removed = file('/removed', { removed: true })
    state.open([...items, reserved, completed, removed])
    expect(state.selected.value).toHaveLength(60)
    expect(canReidentify(reserved)).toBe(false)
    state.selectAll(false); expect(state.selected.value).toEqual([])
    state.selectAll(true); expect(state.selected.value).toHaveLength(60)
  })
  it('submits only checked paths with the opening revision and closes after success', async () => {
    const { state, commit } = fixture()
    state.open([file('/one'), file('/two')])
    state.selected.value = ['/two']
    await state.submit()
    expect(commit).toHaveBeenCalledWith(['/two'], 'workspace', 7)
    expect(state.context.value).toBeNull()
  })
  it('retains the selection after rejection and resets it on reopen', async () => {
    const { state, task, commit } = fixture(vi.fn(async () => false))
    state.open([file('/one'), file('/two')]); state.selected.value = ['/one']
    task.value!.revision = 8
    await state.submit()
    expect(commit).toHaveBeenCalledWith(['/one'], 'workspace', 7)
    expect(state.selected.value).toEqual(['/one'])
    state.close(); state.open([file('/one'), file('/two')])
    expect(state.selected.value).toEqual(['/one', '/two'])
    expect(state.context.value?.revision).toBe(8)
  })
  it('cannot open or submit while the workspace is locked', async () => {
    const { state, locked, commit } = fixture()
    locked.value = true; state.open([file('/one')])
    expect(state.context.value).toBeNull()
    locked.value = false; state.open([file('/one')]); locked.value = true
    await state.submit(); expect(commit).not.toHaveBeenCalled()
  })
})
