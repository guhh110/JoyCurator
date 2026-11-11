import { createServer } from '../../../frontend/node_modules/vite/dist/node/index.js'
import vue from '../../../frontend/node_modules/@vitejs/plugin-vue/dist/index.mjs'
import { fileURLToPath } from 'node:url'
const root = fileURLToPath(new URL('./', import.meta.url))
const project = fileURLToPath(new URL('../../../', import.meta.url))
const server = await createServer({configFile:false,root,cacheDir:`${project}/local/cache/workflow-preview`,resolve:{dedupe:['vue'],alias:{'vue-router':`${project}/frontend/node_modules/vue-router/dist/vue-router.js`,'@tanstack/vue-query':`${project}/frontend/node_modules/@tanstack/vue-query/build/modern/index.js`,vue:`${project}/frontend/node_modules/vue/dist/vue.esm-bundler.js`}},plugins:[vue()],server:{host:'127.0.0.1',port:15175,strictPort:true,fs:{allow:[project]}}})
await server.listen()
console.log('Workflow fixture: http://127.0.0.1:15175/workflow.html')
