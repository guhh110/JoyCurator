import { createServer } from '../../../frontend/node_modules/vite/dist/node/index.js'
import vue from '../../../frontend/node_modules/@vitejs/plugin-vue/dist/index.mjs'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('./', import.meta.url))
const project = fileURLToPath(new URL('../../../', import.meta.url))
const server = await createServer({
  configFile: false,
  root,
  cacheDir: `${project}/local/cache/offset-preview`,
  resolve: { dedupe: ['vue'], alias: { vue: `${project}/frontend/node_modules/vue/dist/vue.esm-bundler.js` } },
  plugins: [vue(), {
    name: 'offset-fixture-api',
    configureServer(server) {
      server.middlewares.use('/api/episodes', (request, response) => {
        const season = Number(new URL(request.url, 'http://localhost').searchParams.get('season') || 1)
        response.setHeader('Content-Type', 'application/json')
        response.end(JSON.stringify(Array.from({ length: season === 1 ? 24 : 12 }, (_, i) => ({ number: i + 1, season, name: `第 ${i + 1} 集`, airDate: '' }))))
      })
    },
  }],
  server: { host: '127.0.0.1', port: 5176, strictPort: true, fs: { allow: [project] } },
})
await server.listen()
console.log('Offset UI fixture: http://127.0.0.1:5176/episode-offset.html')
