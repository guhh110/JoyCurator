import { describe, expect, it } from 'vitest'
import {
  activeConfirmation,
  confirmAction,
  dismissConfirmation,
  resolveConfirmation,
} from '../../../frontend/src/app/confirm'

describe('global confirmation', () => {
  it('resolves true when the confirm action is clicked', async () => {
    const result = confirmAction({ title: '删除？', description: '确认删除。' })

    resolveConfirmation(true)

    await expect(result).resolves.toBe(true)
  })

  it('does not let the dialog close event override a confirm click', async () => {
    const result = confirmAction({ title: '删除？', description: '确认删除。' })

    dismissConfirmation()
    resolveConfirmation(true)
    await Promise.resolve()

    await expect(result).resolves.toBe(true)
    expect(activeConfirmation.value).toBeNull()
  })
})
