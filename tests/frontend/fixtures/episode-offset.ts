import { createApp, defineComponent, h, ref } from 'vue'
import EpisodeOffsetDialog from '../../../frontend/src/features/automatic/EpisodeOffsetDialog.vue'
import { useEpisodeOffset } from '../../../frontend/src/features/automatic/useEpisodeOffset'
import type { Recognition, RecognitionItem } from '../../../frontend/src/api'
import '../../../frontend/src/style.css'
import ToastHost from '../../../frontend/src/components/ui/ToastHost.vue'
import { notify } from '../../../frontend/src/app/toast'

// Isolated UI fixture: real dialog and state, synthetic media, no workspace writes.
const source = '/media/source/inbox/4k'
const posterPath = '/iDdFfZB5sexPhDOvacEhWWvgxel.jpg'
const numbers = new URLSearchParams(location.search).has('many') ? Array.from({ length: 103 }, (_, i) => i + 1) : [1, 2, 4]
const items: RecognitionItem[] = numbers.map(number => ({
  path: `${source}/${number}.mp4`, size: 1024, mtime: '1', group: 'tv:1', season: 1, episodes: [number + 9], metadata: { title: '示例剧集', year: 2024, season: 1, episodes: [number], mediaType: 'tv', tmdbId: 1, releaseTag: null },
  entry: { tmdbId: 1, type: 'tv', title: '老友记', originalTitle: '', year: 1994, overview: '', posterPath },
  candidates: [], state: 'ready', reason: '', selected: false, removed: false,
}))
createApp(defineComponent({ setup() {
  const task = ref<Recognition | null>({ id: 'fixture', revision: 1, state: 'completed', paths: [source], items, completed: items.length, total: items.length, error: '', created: '' })
  const locked = ref(false)
  const editor = useEpisodeOffset({ task, locked, saveError: ref(''), commit: async (items) => {
    task.value = { ...task.value!, revision: task.value!.revision + 1, items }
    notify('季集映射已保存，请检查文件校验状态后重新生成整理预览', 'success')
    return true
  } })
  const open = () => editor.open(task.value!.items, [{ number: 1, name: '第 1 季', count: 24 }, { number: 2, name: '第 2 季', count: 12 }], task.value!.items[0]!.season, source)
  const previewPoster = (path: string) => {
    for (const item of task.value!.items) item.entry!.posterPath = path
    open()
  }
  open()
  return () => h('main', { style: 'min-height:100vh;padding:32px;background:#f6f7fb' }, [
    h('p', { style: 'color:#7d7789;margin-bottom:16px' }, '隔离预览 · 示例数据，不连接工作区'),
    h('button', { class: 'button', onClick: () => previewPoster(posterPath) }, '打开批量修正'),
    h('button', { class: 'button', onClick: () => previewPoster('') }, '无海报检查'),
    h('button', { class: 'button', onClick: () => previewPoster('/joycurator-missing-poster.jpg') }, '海报失败检查'),
    h('button', { class: 'button', onClick: () => notify('设置已保存', 'success') }, '示例成功提示'),
    editor.context.value ? h(EpisodeOffsetDialog, { editor, locked: false }) : null,
    h(ToastHost),
  ])
} })).mount('#app')
