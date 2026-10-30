import { effectScope, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { clearToasts, dismissToast, notify, registerToastTarget, toasts, toastTarget, useToastFeedback } from '../../../frontend/src/app/toast'

afterEach(() => { clearToasts(); vi.unstubAllGlobals() })

describe('operation toast feedback', () => {
  it('ignores empty messages and deduplicates repeated operations with a fresh timer identity', () => {
    notify('')
    notify('   ')
    expect(toasts.value).toHaveLength(0)
    const first = notify('保存失败', 'error')
    const second = notify('保存失败', 'error')
    expect(toasts.value).toHaveLength(1)
    expect(second).not.toBe(first)
    expect(toasts.value[0]).toMatchObject({ id: second, tone: 'error', duration: 7000 })
    dismissToast(first!)
    expect(toasts.value).toHaveLength(1)
    dismissToast(second!)
    expect(toasts.value).toHaveLength(0)
  })
  it('bounds the visible stack and clears all messages on session teardown', async () => {
    for (let i = 0; i < 5; i++) notify(`操作 ${i}`, 'success')
    expect(toasts.value.map(item => item.message)).toEqual(['操作 2', '操作 3', '操作 4'])
    expect(toasts.value.every(item => item.duration === 4500)).toBe(true)
    vi.stubGlobal('localStorage', { removeItem: vi.fn() })
    const { clearSession } = await import('../../../frontend/src/app/session')
    await clearSession()
    expect(toasts.value).toHaveLength(0)
  })
  it('emits feedback on changes rather than replaying stale messages when remounted', () => {
    const error = ref('previous error'), success = ref('previous success')
    const scope = effectScope()
    scope.run(() => useToastFeedback(error, success))
    expect(toasts.value).toHaveLength(0)
    error.value = ''
    error.value = '保存失败'
    success.value = '已保存'
    expect(toasts.value.map(item => item.tone)).toEqual(['error', 'success'])
    expect(error.value).toBe('保存失败')
    const first = toasts.value[0]!.id
    error.value = ''; error.value = '保存失败'
    expect(toasts.value).toHaveLength(2)
    expect(toasts.value.at(-1)!.id).not.toBe(first)
    scope.stop(); clearToasts()
    success.value = 'late'
    expect(toasts.value).toHaveLength(0)
  })
  it('keeps feedback in the newest native modal and restores the previous target on close', () => {
    const outer = { id: 'outer' } as HTMLElement
    const inner = { id: 'inner' } as HTMLElement
    expect(toastTarget.value).toBe('body')
    const closeOuter = registerToastTarget(outer)
    const closeInner = registerToastTarget(inner)
    expect(toastTarget.value).toBe(inner)
    closeInner()
    expect(toastTarget.value).toBe(outer)
    closeOuter()
    expect(toastTarget.value).toBe('body')
    closeOuter()
    expect(toastTarget.value).toBe('body')
  })
  it('does not render operation banners in page flow and uses a fixed dismissible host', () => {
    const source = (path: string) => readFileSync(new URL(`../../../frontend/src/${path}`, import.meta.url), 'utf8')
    for (const path of ['pages/AutomaticRecognitionPage.vue', 'pages/SettingsPage.vue', 'pages/LoginPage.vue',
      'layouts/AppLayout.vue', 'pages/recognition/DebugPage.vue', 'features/extraction/RecognitionRules.vue',
      'features/extraction/RuleEdit.vue', 'features/extraction/ExtractionHistory.vue', 'features/extraction/ExtractionHistoryDetail.vue']) {
      expect(source(path)).not.toMatch(/<p\s+v-if="(?:notice|message|error)[^"]*"[^>]*class="notice/)
    }
    expect(source('App.vue')).toContain('<ToastHost />')
    expect(source('components/ui/ToastHost.vue')).toContain('position:fixed')
    expect(source('components/ui/ToastHost.vue')).toContain('<ToastClose')
    expect(source('components/ui/ToastHost.vue')).toContain(':duration="toast.duration"')
    expect(source('components/Modal.vue')).toContain('registerToastTarget(dialog.value)')
  })
})
