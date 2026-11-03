import {
  computed,
  onActivated,
  onDeactivated,
  onUnmounted,
  ref,
  watch,
} from 'vue'

import { api } from '../../api'
import type { Recognition, RecognitionItem } from '../../model'
import {
  recognitionEdits,
  recognitionGroups,
} from '../../recognition'
import { useQuery } from '@tanstack/vue-query'

import { useRuntime } from '../../app/runtime'
import { useAction } from '../../app/useAction'
import { keys } from '../../app/query'
import { confirmAction } from '../../app/confirm'
import { useWorkspaceSources } from './useWorkspaceSources'
import { useSeasonComparison } from './useSeasonComparison'
import { useWorkSelection } from './useWorkSelection'
import { useWorkspacePreview } from './useWorkspacePreview'
import { useReidentification } from './useReidentification'
import { useEpisodeOffset } from './useEpisodeOffset'

export function useWorkspace() {
  const runtime = useRuntime()
  const { act: perform, busy, message: error, success: notice } = useAction()
  const active = ref(true)
  const queue = runtime.queue
  // A mutable edit draft; taskQuery remains the authoritative server snapshot.
  const task = ref<Recognition | null>(null)
  let baseline: ReturnType<typeof recognitionEdits> = []
  function acceptTask(value: Recognition) { task.value = JSON.parse(JSON.stringify(value)); baseline = recognitionEdits(value.items) }
  const browser = ref(false)
  const filter = ref('all'),
    query = ref(''),
    focus = ref(''),
    groupPage = ref(1),
    filePage = ref(1)
  const transfer = ref<RecognitionItem | null>(null),
    transferGroup = ref('new')
  const freshImport = ref(false)
  let alive = true
  const recognizing = computed(() => task.value?.state === 'running')
  const locked = computed(
    () =>
      busy.value ||
      recognizing.value,
  )
  const groups = computed(() => recognitionGroups(task.value?.items || []))
  const { source, sources, scoped, chooseSource, removeSource } = useWorkspaceSources({ task, groups, act, invalidatePreview: () => { preview.value = null }, changeFilter })
  const filtered = computed(() =>
    scoped.value.filter(
      (group) =>
        (filter.value === 'all' ||
          group.files.some((file) => file.state === filter.value)) &&
        `${group.title} ${group.files.map((file) => file.path).join(' ')}`
          .toLowerCase()
          .includes(query.value.toLowerCase()),
    ),
  )
  const visibleGroups = computed(() =>
    filtered.value.slice((groupPage.value - 1) * 30, groupPage.value * 30),
  )
  // Works per status within the chosen source, matching how the filter works.
  const stateCounts = computed(() => {
    const counts: Record<string, number> = { all: scoped.value.length }
    for (const group of scoped.value)
      for (const state of new Set(group.files.map((file) => file.state)))
        counts[state] = (counts[state] || 0) + 1
    return counts
  })
  const current = computed(
    () =>
      scoped.value.find((group) => group.id === focus.value) ||
      filtered.value[0],
  )
  const visibleFiles = computed(
    () =>
      current.value?.files.slice(
        (filePage.value - 1) * 40,
        filePage.value * 40,
      ) || [],
  )
  const selected = computed(
    () =>
      task.value?.items.filter(
        (file) => file.selected && !file.removed && file.state === 'ready' && !file.execution?.locked && !file.execution?.completed,
      ) || [],
  )
  const pendingCount = computed(
    () =>
      task.value?.items.filter(
        (file) => !file.removed && file.state !== 'ready',
      ).length || 0,
  )
  const readyCount = computed(
    () =>
      task.value?.items.filter(
        (file) => !file.removed && file.state === 'ready' && !file.execution?.locked && !file.execution?.completed,
      ).length || 0,
  )
  const modes: Record<string, string> = {
    copy: '复制',
    move: '移动',
    hardlink: '硬链接',
  }
  const taskQuery = useQuery({
    queryKey: computed(() => keys.recognition(task.value?.id || 'latest')),
    queryFn: ({ signal }) =>
      api<Recognition | null>(
        task.value?.id ? `/recognitions/${task.value.id}` : '/recognitions',
        undefined,
        'GET',
        { signal },
      ),
    enabled: computed(() => active.value && !busy.value),
    staleTime: 1000,
    refetchInterval: (query) =>
      query.state.data?.state === 'running' || queue.value?.active
        ? 1200
        : false,
  })
  const loading = taskQuery.isPending
  watch(
    taskQuery.data,
    (value) => {
      if (!alive || busy.value || value === undefined) return
      if (value === null) {
        if (!task.value) task.value = null
        return
      }
      if (
        !task.value ||
        (task.value.id === value.id && value.revision >= task.value.revision)
      )
        acceptTask(value)
    },
    { immediate: true },
  )
  async function act(fn: () => Promise<void>) {
    await perform(async () => {
      await runtime.client.cancelQueries({ queryKey: ['recognitions'] })
      await fn()
      if (alive && task.value)
        runtime.client.setQueryData(
          keys.recognition(task.value.id),
          JSON.parse(JSON.stringify(task.value)),
        )
    })
  }
  const previewState = useWorkspacePreview({ task, busy, act, notice, runtime })
  const { preview, mode, previewPage, previewRows, duplicatePolicy, previewReady, prepare, changeDuplicatePolicy, execute } = previewState
  async function save(items?: RecognitionItem[]) {
    if (!task.value) return
    preview.value = null
    const original = task.value
    const changes = recognitionEdits(items ?? original.items).filter(edit => {
      const file = original.items.find(item => item.path === edit.path)
      return !file?.execution?.locked && !file?.execution?.completed && JSON.stringify(baseline.find(item => item.path === edit.path)) !== JSON.stringify(edit)
    })
    if (!changes.length) return
    try {
      const value = await api<Recognition>(
        `/recognitions/${original.id}`,
        {
          revision: original.revision,
          items: changes,
        },
        'PUT',
      )
      if (alive) acceptTask(value)
    } catch (e) {
      // Reload the authoritative snapshot after a rejected edit (including edits
      // made from another tab), so an optimistic checkbox cannot look saved.
      const value = await api<Recognition>(
        `/recognitions/${original.id}`,
      ).catch(() => null)
      if (alive && value) acceptTask(value)
      throw e
    }
  }
  const { comparisonSeason, groupSeasons, groupSeasonsLoading, groupSeasonsError, detailView, episodeQuery, retryComparison, viewSeason, assignEpisode } = useSeasonComparison({ current, active, locked, runtime, act, save })
  const { searchGroup, searchQuery, searchYear, searchKind, searchResults, searchBusy, editing, editSeason, editEpisodes, seasons, openSearch, search, pick, openEdit, saveEdit, closeSearch, closeEdit } = useWorkSelection({ task, current, focus, filePage, runtime, act, save })
  const offsetEditor = useEpisodeOffset({
    task,
    locked,
    saveError: error,
    commit: async (items, id, revision) => {
      let saved = false
      await act(async () => {
        if (task.value?.id !== id || task.value.revision !== revision)
          throw new Error('工作区已更新，请关闭弹窗后重新打开')
        await save(items)
        if (current.value?.entry?.tmdbId === offsetEditor.context.value?.tmdbId)
          comparisonSeason.value = offsetEditor.season.value
        saved = true
        notice.value = '季集映射已保存，请检查文件校验状态后重新生成整理预览'
      })
      return saved
    },
  })
  const reidentification = useReidentification({
    task, locked,
    commit: async (paths, id, revision) => {
      let submitted = false
      await act(async () => {
        if (task.value?.id !== id || task.value.revision !== revision)
          throw new Error('工作区已更新，请关闭弹窗后重新打开')
        const value = await api<Recognition>(`/recognitions/${id}/reidentify`, { revision, paths })
        if (alive) acceptTask(value)
        preview.value = null
        submitted = true
        notice.value = `已开始重新识别 ${paths.length} 个文件，人工修正会保留`
        await runtime.refreshQueue()
      })
      return submitted
    },
  })
  function openOffset() {
    if (!current.value || groupSeasonsLoading.value) return
    offsetEditor.open(current.value.files, groupSeasons.value, comparisonSeason.value, source.value)
  }
  function openBrowser(fresh = false) {
    freshImport.value = fresh
    browser.value = true
  }
  async function start(paths: string[]) {
    browser.value = false
    await act(async () => {
      const value = await api<Recognition>('/recognitions', {
        paths,
        id: freshImport.value ? '' : task.value?.id || '',
      })
      if (!alive) return
      acceptTask(value)
      await runtime.refreshQueue()
      preview.value = null
      source.value = ''
      focus.value = ''
      filePage.value = 1
      groupPage.value = 1
    })
  }
  async function poll() {
    const [result] = await Promise.all([
      taskQuery.refetch(),
      runtime.queueQuery.refetch(),
    ])
    if (
      result.data &&
      alive &&
      (!task.value || result.data.id === task.value.id) &&
      result.data.revision >= (task.value?.revision ?? 0)
    )
      acceptTask(result.data)
  }
  async function retry() {
    await act(async () => {
      if (task.value)
        acceptTask(await api<Recognition>(`/recognitions/${task.value.id}/retry`, {}))
      preview.value = null
    })
  }
  async function cancelRecognition() {
    await act(async () => {
      if (task.value) await api(`/recognitions/${task.value.id}/cancel`, {})
      await poll()
    })
  }
  function fileLocked(file: RecognitionItem) { return !!file.execution?.locked || !!file.execution?.completed }
  async function selectFile(file: RecognitionItem) {
    if (fileLocked(file)) return
    await act(async () => {
      file.selected = !file.selected
      await save()
    })
  }
  async function selectReady(value: boolean) {
    await act(async () => {
      for (const group of filtered.value)
        for (const file of group.files)
          if (!fileLocked(file)) file.selected = value && file.state === 'ready'
      await save()
    })
  }
  async function selectGroup(id: string) {
    await act(async () => {
      const group = scoped.value.find((g) => g.id === id)
      if (!group) return
      const ready = group.files.filter((file) => file.state === 'ready' && !fileLocked(file))
      const value = !ready.every((file) => file.selected)
      for (const file of ready) file.selected = value
      await save()
    })
  }
  async function removeFile(file: RecognitionItem) {
    if (fileLocked(file)) return
    if (!await confirmAction({
      title: '移出文件？',
      description: `将「${file.path.split('/').pop()}」移出整理工作区？磁盘上的文件不会被删除。`,
      confirmLabel: '移出文件',
      destructive: true,
    })) return
    await act(async () => {
      file.removed = true
      file.selected = false
      await save()
      filePage.value = 1
    })
  }
  async function moveGroup() {
    await act(async () => {
      if (!transfer.value) return
      const file = task.value?.items.find(item => item.path === transfer.value!.path)
      if (!file || fileLocked(file)) throw new Error('文件已提交整理或不在当前工作区')
      const target = groups.value.find(
        (group) => group.id === transferGroup.value,
      )
      file.group =
        target?.id ||
        `custom:${Date.now()}:${Math.random().toString(16).slice(2)}`
      file.entry = target?.entry || null
      file.selected = !!target?.entry
      const id = file.group
      await save()
      transfer.value = null
      focus.value = id
      filePage.value = 1
    })
  }
  function chooseGroup(id: string) {
    focus.value = id
    filePage.value = 1
  }
  function openTransfer(file: RecognitionItem) {
    if (fileLocked(file)) return
    transfer.value = file
    transferGroup.value = 'new'
  }
  function changeFilter() {
    focus.value = ''
    groupPage.value = 1
    filePage.value = 1
  }
  onActivated(() => {
    active.value = true
  })
  onDeactivated(() => {
    active.value = false
    browser.value = false
    closeSearch()
    closeEdit()
    transfer.value = null
    preview.value = null
    offsetEditor.close()
    reidentification.close()
  })
  onUnmounted(() => {
    alive = false
  })

  return {
    reidentification,
    previewState,
    runtime,
    busy,
    error,
    notice,
    task,
    preview,
    browser,
    filter,
    query,
    mode,
    groupPage,
    filePage,
    searchGroup,
    searchQuery,
    searchYear,
    searchKind,
    searchResults,
    searchBusy,
    editing,
    editSeason,
    editEpisodes,
    seasons,
    transfer,
    transferGroup,
    comparisonSeason,
    detailView,
    viewSeason,
    episodeQuery,
    retryComparison,
    assignEpisode,
    groupSeasons,
    groupSeasonsLoading,
    groupSeasonsError,
    recognizing,
    locked,
    groups,
    fileLocked,
    source,
    sources,
    filtered,
    visibleGroups,
    stateCounts,
    current,
    visibleFiles,
    selected,
    pendingCount,
    readyCount,
    modes,
    previewPage,
    previewRows,
    duplicatePolicy,
    changeDuplicatePolicy,
    previewReady,
    taskQuery,
    loading,
    act,
    save,
    openBrowser,
    start,
    poll,
    retry,
    cancelRecognition,
    selectFile,
    selectReady,
    selectGroup,
    removeFile,
    openSearch,
    search,
    pick,
    openEdit,
    saveEdit,
    offsetEditor,
    openOffset,
    moveGroup,
    prepare,
    execute,
    chooseGroup,
    openTransfer,
    closeSearch,
    closeEdit,
    changeFilter,
    chooseSource,
    removeSource,
  }
}
