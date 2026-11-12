import { computed, onActivated, onDeactivated, onUnmounted, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api } from '../../api'
import type { MediaLayout, Recognition, RecognitionItem } from '../../model'
import { recognitionEdits, recognitionGroups, sourceSummaries } from '../../recognition'
import { useRuntime } from '../../app/runtime'
import { useAction } from '../../app/useAction'
import { keys } from '../../app/query'
import { confirmAction } from '../../app/confirm'
import { useWorkspacePreview } from './useWorkspacePreview'
import { useScrapeBatchPreview } from '../scrape/useScrapeBatchPreview'
import { useReidentification } from './useReidentification'
import { useConfirmation } from './useConfirmation'

export const parentDirectory = (path: string) => path.slice(0, path.lastIndexOf('/')) || '/'
export const fileLocked = (file: RecognitionItem) => !!file.execution?.locked || !!file.execution?.completed
export const isConfirmed = (file: RecognitionItem) => !!file.confirmation && !!file.entry
export function useWorkspace(purpose: 'organize' | 'scrape' = 'organize') {
  const runtime = useRuntime()
  const { act: perform, busy, message: error, success: notice } = useAction()
  const active = ref(true), task = ref<Recognition | null>(null)
  const step = ref<1 | 2 | 3>(1), browser = ref(false), pendingPaths = ref<string[]>([])
  const directory = ref(''), query = ref(''), status = ref<'pending' | 'confirmed'>('pending')
  let alive = true, baseline: ReturnType<typeof recognitionEdits> = []
  const recognizing = computed(() => task.value?.state === 'running')
  const locked = computed(() => busy.value || recognizing.value)
  const files = computed(() => task.value?.items.filter(file => !file.removed) || [])
  const editable = computed(() => files.value.filter(file => !fileLocked(file)))
  const pendingCount = computed(() => editable.value.filter(file => !isConfirmed(file)).length)
  const ready = computed(() => editable.value.filter(file => isConfirmed(file) && file.state === 'ready'))
  const selected = computed(() => ready.value.filter(file => file.selected))
  const directories = computed(() => recognitionGroups(files.value, 'directories').map(group => ({ ...group, pending: group.files.filter(file => !isConfirmed(file) && !fileLocked(file)).length })))
  const current = computed(() => directories.value.find(group => group.id === directory.value))
  const scopeFiles = computed(() => directory.value ? current.value?.files || [] : files.value)
  const currentFiles = computed(() => scopeFiles.value.filter(file => (status.value === 'confirmed' ? isConfirmed(file) : !isConfirmed(file)) && file.path.toLowerCase().includes(query.value.toLowerCase())))
  const sources = computed(() => sourceSummaries(task.value?.items || [], task.value?.paths || []))
  const workGroups = computed(() => recognitionGroups(editable.value.filter(isConfirmed)))
  const modes = { copy: '复制', move: '移动', hardlink: '硬链接' }
  function acceptTask(value: Recognition) {
    task.value = JSON.parse(JSON.stringify(value))
    baseline = recognitionEdits(value.items)
  }
  const taskQuery = useQuery({
    queryKey: computed(() => keys.recognition(task.value?.id || 'latest', purpose)),
    queryFn: ({ signal }) => api<Recognition | null>(task.value?.id ? `/recognitions/${task.value.id}` : `/recognitions?purpose=${purpose}`, undefined, 'GET', { signal }),
    enabled: computed(() => active.value && !busy.value), staleTime: 1000,
    refetchInterval: query => query.state.data?.state === 'running' || runtime.queue.value?.active ? 1200 : false,
  })
  watch(taskQuery.data, value => {
    if (!alive || busy.value || value === undefined) return
    if (value && (!task.value || value.id === task.value.id && value.revision >= task.value.revision)) acceptTask(value)
  }, { immediate: true })
  async function act(fn: () => Promise<void>) {
    await perform(async () => {
      await runtime.client.cancelQueries({ queryKey: ['recognitions'] })
      await fn()
      if (alive && task.value) runtime.client.setQueryData(keys.recognition(task.value.id, purpose), JSON.parse(JSON.stringify(task.value)))
    })
  }
  const previewState = useWorkspacePreview({ task, busy, act, notice, runtime })
  const scrapePreviewState = useScrapeBatchPreview({ task, busy, act, notice, runtime })
  function invalidatePreviews() { previewState.invalidate(); scrapePreviewState.invalidate() }
  watch(() => files.value.map(file => `${file.path}:${!!file.execution?.locked}:${!!file.execution?.completed}`).join('|'), invalidatePreviews)
  watch(() => runtime.settingsQuery?.data.value?.version, invalidatePreviews)
  const confirmation = useConfirmation({ task, busy, scope: currentFiles, act, accept: acceptTask, invalidate: invalidatePreviews })
  async function discardDraft() {
    if (busy.value || confirmation.saving.value) return false
    if (confirmation.dirty.value && !await confirmAction({ title: '放弃未确认修改？', description: '已确认的文件会保留。', confirmLabel: '放弃并继续', cancelLabel: '继续编辑' })) return false
    confirmation.close()
    return true
  }
  async function loadDirectory() {
    if (!task.value || recognizing.value) return
    const usable = scopeFiles.value.filter(file => !fileLocked(file))
    if (!confirmation.open.value) await confirmation.start(usable, file => !isConfirmed(file))
    else if (!confirmation.stale.value) await confirmation.add(usable, file => !isConfirmed(file))
  }
  async function chooseDirectory(path: string) {
    if (locked.value || confirmation.loading.value || directory.value === path && confirmation.open.value) return
    // Directory, confirmation status and search are independent filters.
    directory.value = path
    await loadDirectory()
  }
  async function reloadDraft() {
    if (!await discardDraft()) return
    await poll()
    await loadDirectory()
  }
  async function goStep(value: 1 | 2 | 3) {
    if (locked.value || confirmation.loading.value || value === step.value) return
    if (step.value === 2 && !await discardDraft()) return
    if (value === 2 && !files.value.length || value === 3 && !workGroups.value.length) return
    invalidatePreviews(); step.value = value
    if (value === 2) await loadDirectory()
  }
  watch(() => [task.value?.id, task.value?.state] as const, async (value, old) => {
    if (!task.value) return
    const newTask = value[0] !== old?.[0]
    if (newTask) confirmation.close()
    if (recognizing.value) { step.value = 1; confirmation.close(); invalidatePreviews(); return }
    if (newTask || old?.[1] === 'running') {
      directory.value = directories.value.find(group => group.pending)?.id || directories.value[0]?.id || ''
      status.value = pendingCount.value ? 'pending' : 'confirmed'
      step.value = files.value.length ? 2 : 1
      await loadDirectory()
    }
  }, { immediate: true })
  async function confirmGroup(key: string) {
    const group = confirmation.group(key), count = group.chosen.value.length
    if (!await group.confirm()) return
    notice.value = `已确认 ${count} 个文件`
    await loadDirectory()
  }
  async function save(items?: RecognitionItem[]) {
    if (!task.value) return
    invalidatePreviews()
    const original = task.value
    const changes = recognitionEdits(items ?? original.items).filter(edit => {
      const file = original.items.find(file => file.path === edit.path)
      return file && !fileLocked(file) && JSON.stringify(baseline.find(value => value.path === edit.path)) !== JSON.stringify(edit)
    })
    if (!changes.length) return
    try { acceptTask(await api<Recognition>(`/recognitions/${original.id}`, { revision: original.revision, items: changes }, 'PUT')) }
    catch (value) {
      const saved = await api<Recognition>(`/recognitions/${original.id}`).catch(() => null)
      if (alive && saved) acceptTask(saved)
      throw value
    }
  }
  function openBrowser() { browser.value = true }
  function addPaths(paths: string[]) {
    browser.value = false
    const existing = files.value.length ? task.value?.paths || [] : []
    pendingPaths.value = purpose === 'scrape' ? paths.slice(0, 1) : [...new Set([...pendingPaths.value, ...paths])].filter(path => !existing.includes(path))
  }
  async function scanSources() {
    if (!pendingPaths.value.length || locked.value) return
    const replacing = purpose === 'scrape' && task.value && pendingPaths.value[0] !== task.value.paths[0]
    if (replacing && !await confirmAction({ title: '更换刮削目录？', description: '将为新目录打开新的工作区。', confirmLabel: '更换目录' })) return
    await act(async () => {
      const startNew = purpose === 'organize' && task.value?.state === 'completed' && !files.value.length
      const value = await api<Recognition>('/recognitions', { paths: pendingPaths.value, purpose, id: replacing || startNew ? '' : task.value?.id || '' })
      if (!alive) return
      acceptTask(value); pendingPaths.value = []; invalidatePreviews()
      await runtime.refreshQueue()
    })
    if (alive && step.value === 2) await loadDirectory()
  }
  async function removeSource(path: string) {
    if (!await confirmAction({ title: '移出来源？', description: path, confirmLabel: '移出来源', destructive: true })) return
    await act(async () => {
      if (!task.value) return
      acceptTask(await api<Recognition>(`/recognitions/${task.value.id}/sources/remove`, { path, revision: task.value.revision }))
      invalidatePreviews(); confirmation.close()
      directory.value = directories.value.find(group => group.pending)?.id || directories.value[0]?.id || ''
      notice.value = '来源已移出'
    })
  }
  async function poll() {
    const result = await taskQuery.refetch()
    if (result.data && alive && (!task.value || result.data.id === task.value.id) && result.data.revision >= (task.value?.revision ?? 0)) acceptTask(result.data)
  }
  async function retry() {
    await act(async () => { if (task.value) acceptTask(await api<Recognition>(`/recognitions/${task.value.id}/retry`, {})); invalidatePreviews() })
  }
  async function cancelRecognition() { await act(async () => { if (task.value) await api(`/recognitions/${task.value.id}/cancel`, {}); await poll() }); if (step.value === 2) await loadDirectory() }
  async function selectFile(file: RecognitionItem) {
    if (fileLocked(file) || !isConfirmed(file) || file.state !== 'ready') return
    await act(async () => { file.selected = !file.selected; await save() })
  }
  async function selectReady(value: boolean) { await act(async () => { for (const file of ready.value) file.selected = value; await save() }) }
  async function saveLayouts(changes: { paths: string[]; layout: MediaLayout }[]) {
    await act(async () => {
      for (const change of changes) for (const file of editable.value) if (change.paths.includes(file.path)) file.layout = change.layout
      await save()
      notice.value = '输出目录已保存'
    })
  }
  const reidentification = useReidentification({ task, locked, commit: async (paths, id, revision) => {
    let submitted = false
    if (!await discardDraft()) return false
    await act(async () => {
      if (task.value?.id !== id || task.value.revision !== revision) throw new Error('工作区已更新，请重新选择文件')
      acceptTask(await api<Recognition>(`/recognitions/${id}/reidentify`, { revision, paths }))
      invalidatePreviews(); submitted = true; notice.value = `已开始重新识别 ${paths.length} 个文件`
      await runtime.refreshQueue()
    })
    return submitted
  } })
  async function execute() {
    if (purpose === 'scrape') await scrapePreviewState.execute()
    else await previewState.execute()
    if (!error.value) { await poll(); step.value = 2; confirmation.close(); await loadDirectory() }
  }
  onActivated(() => { active.value = true; if (step.value === 2) void loadDirectory() })
  onDeactivated(() => { active.value = false; browser.value = false; confirmation.close(); reidentification.close(); invalidatePreviews(); if (step.value === 3) step.value = 2 })
  onUnmounted(() => { alive = false })
  return { purpose, runtime, task, taskQuery, loading: taskQuery.isPending, busy, locked, recognizing, error, notice,
    step, browser, pendingPaths, files, editable, pendingCount, ready, selected, sources, directories, directory, current, scopeFiles, currentFiles, query, status, workGroups, modes,
    confirmation, previewState, scrapePreviewState, reidentification, act, save, fileLocked, isConfirmed,
    openBrowser, addPaths, scanSources, removeSource, goStep, chooseDirectory, reloadDraft, loadDirectory, discardDraft, confirmGroup, poll, retry, cancelRecognition, selectFile, selectReady, saveLayouts, execute, invalidatePreviews }
}
export type Workspace = ReturnType<typeof useWorkspace>
