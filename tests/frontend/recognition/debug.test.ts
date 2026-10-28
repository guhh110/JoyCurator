import { effectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useExtraction } from '../../../frontend/src/features/extraction/useExtraction'
import { valueText } from '../../../frontend/src/features/extraction/model'
import type { ExtractionResult } from '../../../frontend/src/features/extraction/model'

const scopes: ReturnType<typeof effectScope>[] = []
function workspace() {
  const scope = effectScope()
  scopes.push(scope)
  return { scope, state: scope.run(useExtraction)! }
}
function result(title: string): ExtractionResult {
  return {
    path: `/nas/${title}.mkv`,
    root: '/',
    metadata: {
      title,
      year: null,
      mediaType: null,
      tmdbId: null,
      season: null,
      episodes: [],
      releaseTag: null,
    },
    steps: [],
  }
}
function response(title: string) {
  return new Response(JSON.stringify(result(title)), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}
afterEach(() => {
  for (const scope of scopes) scope.stop()
  scopes.length = 0
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('recognition debug request lifecycle', () => {
  it('submits a simulated path, mode and optional boundary with an abort signal', async () => {
    const fetch = vi.fn().mockResolvedValue(response('Film'))
    vi.stubGlobal('fetch', fetch)
    const { state } = workspace()
    state.path.value = '/nas/Film.mkv'
    state.root.value = '/nas'

    await state.run()
    expect(fetch).toHaveBeenCalledOnce()
    const [url, options] = fetch.mock.calls[0]!
    expect(url).toBe('/api/extraction/test')
    expect(JSON.parse(options.body)).toEqual({
      path: '/nas/Film.mkv',
      root: '/nas',
    })
    expect(options.signal).toBeInstanceOf(AbortSignal)
    expect(state.result.value?.metadata.title).toBe('Film')
    expect(state.busy.value).toBe(false)
  })
  it('cancels and ignores a response that arrives after cancellation', async () => {
    const request = deferred<Response>()
    const fetch = vi.fn().mockReturnValue(request.promise)
    vi.stubGlobal('fetch', fetch)
    const { state } = workspace()
    state.path.value = '/nas/Film.mkv'
    const running = state.run()
    state.cancel()
    request.resolve(response('Film'))
    await running
    expect(fetch.mock.calls[0]![1].signal.aborted).toBe(true)
    expect(state.result.value).toBeNull()
    expect(state.busy.value).toBe(false)
    expect(state.notice.value).toBe('已取消调试')
  })
  it('does not replace a newer result with an older request', async () => {
    const old = deferred<Response>(),
      next = deferred<Response>()
    const fetch = vi
      .fn()
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(next.promise)
    vi.stubGlobal('fetch', fetch)
    const { state } = workspace()
    state.path.value = '/nas/Old.mkv'
    const first = state.run()
    state.path.value = '/nas/New.mkv'
    const second = state.run()
    next.resolve(response('New'))
    await second
    old.resolve(response('Old'))
    await first
    expect(state.result.value?.metadata.title).toBe('New')
    expect(fetch.mock.calls[0]![1].signal.aborted).toBe(true)
  })
  it('aborts when the owning component scope is disposed', async () => {
    const request = deferred<Response>()
    const fetch = vi.fn().mockReturnValue(request.promise)
    vi.stubGlobal('fetch', fetch)
    const { state, scope } = workspace()
    const running = state.run()
    scope.stop()
    request.resolve(response('Late'))
    await running
    expect(fetch.mock.calls[0]![1].signal.aborted).toBe(true)
    expect(state.result.value).toBeNull()
  })
  it('stops after 90 seconds and retains the timeout message after a late response', async () => {
    vi.useFakeTimers()
    const request = deferred<Response>()
    const fetch = vi.fn().mockReturnValue(request.promise)
    vi.stubGlobal('fetch', fetch)
    const { state } = workspace()
    const running = state.run()
    await vi.advanceTimersByTimeAsync(90_000)
    expect(state.busy.value).toBe(false)
    expect(state.error.value).toContain('90 秒')
    expect(fetch.mock.calls[0]![1].signal.aborted).toBe(true)
    request.resolve(response('Late'))
    await running
    expect(state.result.value).toBeNull()
    expect(state.error.value).toContain('90 秒')
  })
  it('shows validation errors without inventing a debug result', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message: '路径无效' }), {
          status: 400,
        }),
      ),
    )
    const { state } = workspace()
    await state.run()
    expect(state.error.value).toBe('路径无效')
    expect(state.result.value).toBeNull()
  })
  it('renders tri-state path checks and structured values clearly', () => {
    expect(valueText(null)).toBe('未知')
    expect(valueText(0)).toBe('0')
    expect(valueText('movie')).toBe('movie')
    expect(valueText({ episodes: [1, 3] })).toContain('"episodes"')
  })
})
