import { ref } from 'vue'
import { useMutation } from '@tanstack/vue-query'
export function useAction() {
  const message = ref(''),
    success = ref('')
  const mutation = useMutation({
    mutationFn: (action: () => Promise<unknown>) => action(),
  })
  async function act<T>(action: () => Promise<T>): Promise<T | undefined> {
    if (mutation.isPending.value) return
    message.value = ''
    success.value = ''
    try {
      return (await mutation.mutateAsync(action)) as T
    } catch (error) {
      message.value = (error as Error).message
    }
  }
  return { act, busy: mutation.isPending, message, success }
}
