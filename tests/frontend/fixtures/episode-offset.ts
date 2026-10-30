import { createApp, defineComponent, h, ref } from 'vue'
import EpisodeOffsetDialog from '../../../frontend/src/features/automatic/EpisodeOffsetDialog.vue'
import { useEpisodeOffset } from '../../../frontend/src/features/automatic/useEpisodeOffset'
import type { Recognition, RecognitionItem } from '../../../frontend/src/api'
import '../../../frontend/src/style.css'

// Isolated UI fixture: real dialog and state, synthetic media, no workspace writes.
const source = '/media/source/inbox/4k'
const items: RecognitionItem[] = [1, 2, 4].map(number => ({
  path: `${source}/${number}.mp4`, size: 1024, mtime: '1', group: 'tv:1', season: 1, episodes: [number],
  probe: { resolution: '', codec: '' }, metadata: { title: '示例剧集', year: 2024, season: 1, episodes: [number], mediaType: 'tv', tmdbId: 1, releaseTag: null },
  entry: { tmdbId: 1, type: 'tv', title: '示例剧集', originalTitle: '', year: 2024, overview: '', posterPath: '' },
  candidates: [], state: 'ready', reason: '', selected: false, removed: false,
}))
createApp(defineComponent({ setup() {
  const task = ref<Recognition | null>({ id: 'fixture', revision: 1, state: 'completed', paths: [source], items, completed: 3, total: 3, error: '', created: '' })
  const locked = ref(false)
  const editor = useEpisodeOffset({ task, locked, saveError: ref(''), commit: async () => true })
  const open = () => editor.open(items, [{ number: 1, name: '第 1 季', count: 24 }, { number: 2, name: '第 2 季', count: 12 }], 1, source)
  open()
  return () => h('main', { style: 'min-height:100vh;padding:32px;background:#f6f7fb' }, [
    h('p', { style: 'color:#7d7789;margin-bottom:16px' }, '隔离预览 · 示例数据，不连接工作区'),
    h('button', { class: 'button', onClick: open }, '打开批量修正'),
    editor.context.value ? h(EpisodeOffsetDialog, { editor, locked: false }) : null,
  ])
} })).mount('#app')
