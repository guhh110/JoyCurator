import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, invalidateRequests } from '../../../frontend/src/api'
import { createQueryClient, keys } from '../../../frontend/src/app/query'
const clients: ReturnType<typeof createQueryClient>[] = []
function client() {
  const value = createQueryClient()
  clients.push(value)
  return value
}
afterEach(() => {
  clients.forEach((value) => value.clear())
  clients.length = 0
  vi.unstubAllGlobals()
})
function response(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}
describe('shared server-state client', () => {
  it('deduplicates requests and refetches saved rules after invalidation', async () => {
    const cache = client()
    let version = 1
    const fetch = vi.fn(async () => response({ version }))
    vi.stubGlobal('fetch', fetch)
    const options = {
      queryKey: keys.ruleLibrary,
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        api<{ version: number }>('/extraction/rules', undefined, 'GET', {
          signal,
        }),
    }
    const [first, second] = await Promise.all([
      cache.fetchQuery(options),
      cache.fetchQuery(options),
    ])
    expect(fetch).toHaveBeenCalledOnce()
    expect(first).toEqual(second)
    version = 2
    await cache.invalidateQueries({ queryKey: keys.ruleLibrary })
    expect((await cache.fetchQuery(options)).version).toBe(2)
    expect(fetch).toHaveBeenCalledTimes(2)
  })
  it('cancels transport with the query signal', async () => {
    const cache = client()
    let signal: AbortSignal | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url, options) =>
          new Promise((_resolve, reject) => {
            signal = options.signal
            signal!.addEventListener('abort', () =>
              reject(new DOMException('aborted', 'AbortError')),
            )
          }),
      ),
    )
    const result = cache
      .fetchQuery({
        queryKey: keys.trace('old'),
        queryFn: ({ signal }) =>
          api('/extraction/traces/old', undefined, 'GET', { signal }),
      })
      .catch((error) => error)
    await cache.cancelQueries({ queryKey: keys.trace('old') })
    expect(signal?.aborted).toBe(true)
    expect(await result).toBeInstanceOf(Error)
  })
  it('keeps page and trace caches independent and clears them on session teardown', async () => {
    const cache = client()
    cache.setQueryData(keys.trace('one'), { id: 'one' })
    cache.setQueryData(keys.trace('two'), { id: 'two' })
    cache.removeQueries({ queryKey: keys.trace('one') })
    expect(cache.getQueryData(keys.trace('one'))).toBeUndefined()
    expect(cache.getQueryData(keys.trace('two'))).toEqual({ id: 'two' })
    cache.clear()
    expect(cache.getQueryCache().getAll()).toHaveLength(0)
  })
  it('sends explicit DELETE even without a request body', async () => {
    const fetch = vi.fn(async () => response({ ok: true }))
    vi.stubGlobal('fetch', fetch)
    await api('/extraction/traces/id', undefined, 'DELETE')
    expect(fetch.mock.calls[0]![1].method).toBe('DELETE')
  })
  it('ignores API responses from a previous authenticated session', async () => {
    let resolve!: (value: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((done) => {
            resolve = done
          }),
      ),
    )
    const request = api('/settings')
    invalidateRequests()
    resolve(response({ sourceRoot: '/private' }))
    await expect(request).rejects.toMatchObject({ name: 'AbortError' })
  })
  it('does not retry rejected credentials or repeat mutations', async () => {
    const cache = client(),
      fetch = vi.fn(async () => response({ message: 'unauthorized' }, 401))
    vi.stubGlobal('fetch', fetch)
    await expect(
      cache.fetchQuery({
        queryKey: keys.ruleLibrary,
        queryFn: () => api('/extraction/rules'),
      }),
    ).rejects.toBeInstanceOf(ApiError)
    expect(fetch).toHaveBeenCalledOnce()
    expect(cache.getDefaultOptions().mutations?.retry).toBe(false)
  })
})
