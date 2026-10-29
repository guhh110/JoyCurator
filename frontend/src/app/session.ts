import { reactive } from 'vue'
import { api, invalidateRequests } from '../api'
import { keys, queryClient } from './query'
export interface Session {
  initialized: boolean
  username: string
}
export const session = reactive<Session & { error: string }>({
  initialized: true,
  username: '',
  error: '',
})
export async function loadSession() {
  try {
    const value = await queryClient.fetchQuery({
      queryKey: keys.session,
      queryFn: ({ signal }) =>
        api<Session>('/session', undefined, 'GET', { signal }),
      staleTime: 30_000,
    })
    Object.assign(session, value, { error: '' })
    return value
  } catch (error) {
    session.error = (error as Error).message
    throw error
  }
}
export function acceptSession(value: Session) {
  Object.assign(session, value, { error: '' })
  queryClient.setQueryData(keys.session, value)
}
export async function clearSession() {
  invalidateRequests()
  session.username = ''
  await queryClient.cancelQueries()
  queryClient.clear()
  session.username = ''
  for (const key of [
    'joycurator-files',
    'joycurator-entry',
    'joycurator-active',
  ])
    localStorage.removeItem(key)
}
export function safeRedirect(value: unknown): string {
  return typeof value === 'string' &&
    value.startsWith('/') &&
    !value.startsWith('//') &&
    !value.includes('\\') &&
    !/[\u0000-\u001f\u007f]/.test(value) &&
    !value.startsWith('/login')
    ? value
    : '/organize'
}
