import { computed, nextTick, onDeactivated, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { DraggableEvent } from 'vue-draggable-plus'
import { api, type Category, type Settings } from '../../api'
import { keys } from '../../app/query'
import { useRuntime } from '../../app/runtime'
import { useAction } from '../../app/useAction'
import { notify } from '../../app/toast'
export function useSettingsForm() {
  const runtime = useRuntime(),
    route = useRoute(),
    router = useRouter()
  const { act, busy, message, success } = useAction()
  const settings = ref<Settings | null>(null),
    token = ref(''),
    clearToken = ref(false),
    directoryTarget = ref<'sourceRoot' | 'targetRoot' | null>(null)
  const running = runtime.executing
  const sections = ['folders', 'tmdb', 'rules']
  const settingsSection = computed({
    get: () =>
      sections.includes(String(route.query.section))
        ? String(route.query.section)
        : 'folders',
    set: (value) => {
      void router.push({ name: 'settings', query: { section: value } })
    },
  })
  watch(
    runtime.settingsQuery.data,
    (value) => {
      if (value && !settings.value)
        settings.value = JSON.parse(JSON.stringify(value))
    },
    { immediate: true },
  )
  const categoryDragging = ref(false)
  const categoryKeys = new WeakMap<Category, number>()
  let nextCategoryKey = 0
  function categoryKey(category: Category) {
    if (!categoryKeys.has(category))
      categoryKeys.set(category, ++nextCategoryKey)
    return categoryKeys.get(category)!
  }
  function moveCategory(index: number, offset: number, event: Event) {
    const list = settings.value?.rules.categories,
      target = index + offset
    if (
      !list ||
      busy.value ||
      running.value ||
      categoryDragging.value ||
      target < 0 ||
      target >= list.length
    )
      return
    const handle = event.currentTarget as HTMLElement,
      [category] = list.splice(index, 1)
    list.splice(target, 0, category!)
    notify(`已移至第 ${target + 1} 条，保存设置后生效。`)
    void nextTick(() => handle.focus())
  }
  function finishCategorySort(event: DraggableEvent) {
    categoryDragging.value = false
    if (event.oldIndex !== event.newIndex && event.newIndex !== undefined)
      notify(`已移至第 ${event.newIndex + 1} 条，保存设置后生效。`)
  }
  async function loadSettings() {
    const result = await runtime.settingsQuery.refetch()
    if (result.error) throw result.error
    if (result.data) settings.value = JSON.parse(JSON.stringify(result.data))
  }
  async function saveSettings() {
    if (!settings.value) return
    await act(async () => {
      const { hasToken: _, roots: __, ...value } = settings.value!
      value.rules.categories = value.rules.categories.map((category) => ({
        ...category,
        yearMin: Number(category.yearMin) || 0,
        yearMax: Number(category.yearMax) || 0,
      }))
      await api(
        '/settings',
        {
          ...value,
          ...(clearToken.value
            ? { tmdbToken: '' }
            : token.value.trim()
              ? { tmdbToken: token.value.trim() }
              : {}),
        },
        'PUT',
      )
      token.value = ''
      clearToken.value = false
      await runtime.client.invalidateQueries({
        queryKey: keys.settings,
        refetchType: 'none',
      })
      await loadSettings()
      success.value = '设置已保存'
    })
  }
  function chooseDirectory(paths: string[]) {
    if (settings.value && directoryTarget.value)
      settings.value[directoryTarget.value] = paths[0] || ''
    directoryTarget.value = null
  }
  function parts(event: Event) {
    return (event.target as HTMLInputElement).value
      .split(/[,，]/)
      .map((s) => s.trim())
      .filter(Boolean)
  }
  onDeactivated(() => {
    directoryTarget.value = null
  })
  return {
    settings,
    token,
    clearToken,
    directoryTarget,
    settingsSection,
    running,
    act,
    busy,
    message,
    success,
    categoryDragging,
    categoryKey,
    moveCategory,
    finishCategorySort,
    loadSettings,
    saveSettings,
    chooseDirectory,
    parts,
    settingsQuery: runtime.settingsQuery,
  }
}
