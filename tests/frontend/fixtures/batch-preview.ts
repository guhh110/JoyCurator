import { createApp, h } from 'vue'
import { VueQueryPlugin } from '../../../frontend/node_modules/@tanstack/vue-query/build/modern/index.js'
import { createRouter, createMemoryHistory } from '../../../frontend/node_modules/vue-router/dist/vue-router.js'
import AutomaticRecognitionPage from '../../../frontend/src/pages/AutomaticRecognitionPage.vue'
import ToastHost from '../../../frontend/src/components/ui/ToastHost.vue'
import { provideRuntime } from '../../../frontend/src/app/runtime'
import { createQueryClient } from '../../../frontend/src/app/query'
import '../../../frontend/src/style.css'

const files = [1, 2, 3].map(number => ({ path: `/media/inbox/Show/${number}.mp4`, size: 1024, mtime: '1', group: 'tv:1', season: 1, episodes: [number],
  probe: { resolution: '', codec: '' }, metadata: { title: '示例剧集', season: 1, episodes: [number], mediaType: 'tv', tmdbId: 1, year: 2024, releaseTag: null },
  entry: { title: '示例剧集', originalTitle: '', tmdbId: 1, type: 'tv', year: 2024, overview: '', posterPath: '' }, candidates: [], state: 'ready', reason: '', selected: true, removed: false }))
const scenario = new URLSearchParams(location.search)
let failedPreview = false
// Synthetic transport only: preview interactions cannot execute or alter real files.
window.fetch = async (input, options) => {
  const url = String(input)
  let value: unknown = null
  if (url.startsWith('/api/recognitions')) value = { id: 'fixture', revision: 1, state: 'completed', paths: ['/media/inbox'], items: files, completed: 3, total: 3, created: '', error: '' }
  if (url === '/api/settings') value = {}
  if (url.startsWith('/api/seasons')) value = [{ number: 1, name: '第 1 季', count: 24 }]
  if (url === '/api/batches/preview') {
    const { duplicatePolicy, mode } = JSON.parse(options?.body as string)
    if (scenario.has('fail') && duplicatePolicy === 'overwrite' && !failedPreview) {
      failedPreview = true
      return new Response(JSON.stringify({ message: '示例：预览更新失败' }), { status: 500 })
    }
    const rows = files.map((file, index) => ({ source: file.path, target: `/media/library/Show/Season 01/Show.S01E0${index + 1}.mp4`, season: 1, episodes: file.episodes, action: index && !scenario.has('all') ? 'create' : duplicatePolicy === 'overwrite' ? 'overwrite' : 'skip', artifacts: [], issues: [] }))
    const group = { id: 'group', title: '示例剧集', mode, duplicatePolicy, rows, required: rows.filter(row => row.action !== 'skip').length * 1024, issues: [], type: 'tv', tmdbId: 1 }
    value = { ...group, id: `preview-${duplicatePolicy}`, groups: [group] }
  }
  if (url === '/api/batches/execute') return new Response(JSON.stringify({ message: '隔离预览不执行文件操作' }), { status: 400 })
  return new Response(JSON.stringify(value), { status: 200 })
}
const app = createApp({ setup() { provideRuntime(); return () => h('main', { style: 'padding:24px' }, [h(AutomaticRecognitionPage), h(ToastHost)]) } })
app.use(VueQueryPlugin, { queryClient: createQueryClient() })
app.use(createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }] }))
app.mount('#app')
