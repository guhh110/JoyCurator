import { createSSRApp, effectScope, nextTick, ref } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import TaskToast from '../../../frontend/src/components/jobs/TaskToast.vue'
import { useTaskProgress } from '../../../frontend/src/components/jobs/useTaskProgress'
import type { QueueStatus, Run } from '../../../frontend/src/model'

const queue = ref<QueueStatus | null>(null)
const inspect = vi.fn(), recordAction = vi.fn(), actionBusy = ref(false)
vi.mock('../../../frontend/src/app/runtime', () => ({ useRuntime: () => ({ queue, inspect, recordAction, actionBusy }) }))
function task(purpose: 'organize' | 'scrape'): Run {
  return { id: purpose, purpose, title: purpose === 'organize' ? '山海情' : '星际穿越', mode: purpose === 'organize' ? 'copy' : 'scrape', state: 'running', created: '', error: '', completed: 1, total: 10, undo: false }
}
function active(): QueueStatus {
  return { active: true, recognizing: false, revision: 1,
    organize: { current: task('organize'), queued: 2, queuedBatches: 1, waitReason: '' },
    scrape: { current: null, queued: 0, queuedBatches: 0, waitReason: '' } }
}
const cleanup: (() => void)[] = []
function progress() {
  const scope = effectScope()
  const state = scope.run(() => useTaskProgress(queue))!
  cleanup.push(() => scope.stop())
  return state
}
async function render(collapsed = false) {
  const app = createSSRApp({ ...TaskToast, setup(props: unknown, context: unknown) {
    // Exercise the real SSR template with either initial disclosure state.
    const state = (TaskToast as any).setup(props, context)
    state.collapsed.value = collapsed
    return state
  } })
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/records', component: { render: () => null } },
    { path: '/scrape/records', component: { render: () => null } },
  ] })
  app.use(router)
  await router.push('/records'); await router.isReady()
  return renderToString(app)
}
beforeEach(() => { vi.useFakeTimers(); vi.clearAllMocks(); queue.value = active(); actionBusy.value = false })
afterEach(() => { cleanup.splice(0).forEach(close => close()); vi.useRealTimers() })

describe('persistent task progress panel', () => {
  it('stays available while tasks exist and keeps the full queue summary and action destinations', async () => {
    const state = progress()
    await vi.advanceTimersByTimeAsync(600_000)
    expect(state.lanes.value).toHaveLength(1)
    const html = await render()
    expect(html.replace(/<[^>]+>/g, '')).toContain('等待 2 个任务 · 1 个批次')
    expect(html).toContain('山海情')
    expect(html).toContain('href="/records"')
    expect(html).toContain('aria-label="收起任务进度"')
    expect(html).toContain('aria-expanded="true"')
    expect(html).toContain('queue-actions')
  })
  it('keeps updating task counts while collapsed without taking task actions', async () => {
    const state = progress()
    state.collapsed.value = true
    queue.value!.organize.current!.completed = 7
    queue.value!.organize.queued = 4
    await nextTick()
    expect(state.collapsed.value).toBe(true)
    expect(state.total.value).toBe(5)
    expect(state.lanes.value[0]!.current!.completed).toBe(7)
    expect(recordAction).not.toHaveBeenCalled()
    const html = await render(true)
    expect(html).toContain('任务进行中 · 5 项')
    expect(html).toContain('aria-label="展开任务进度"')
    expect(html).toContain('aria-expanded="false"')
    expect(html).not.toContain('queue-actions')
    expect(html).not.toContain('详情')
  })
  it('hides when idle and starts expanded for the next set of tasks', async () => {
    const state = progress()
    state.collapsed.value = true
    queue.value = { ...active(), active: false, organize: { current: null, queued: 0, queuedBatches: 0, waitReason: '' } }
    await nextTick()
    expect(state.lanes.value).toHaveLength(0)
    expect(await render()).not.toContain('<section')
    queue.value = active(); await nextTick()
    expect(state.collapsed.value).toBe(false)
    expect((await render())).toContain('aria-label="收起任务进度"')
  })
  it('keeps both channels separate and supports tasks that are only queued', async () => {
    queue.value!.scrape = { current: task('scrape'), queued: 1, queuedBatches: 0, waitReason: '等待资源释放' }
    const state = progress()
    expect(state.total.value).toBe(5)
    const html = await render()
    expect(html).toContain('文件整理'); expect(html).toContain('媒体刮削')
    expect(html).toContain('href="/records"'); expect(html).toContain('href="/scrape/records"')
    expect(html).toContain('等待资源释放')
    queue.value!.organize.current = null; queue.value!.scrape.current = null; await nextTick()
    expect(state.running.value).toBe(0)
    expect(state.total.value).toBe(3)
    expect(await render(true)).toContain('任务排队中 · 3 项')
  })
})
