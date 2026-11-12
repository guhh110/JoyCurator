import { createApp, h } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { createRouter, createMemoryHistory, RouterView } from 'vue-router'
import AutomaticRecognitionPage from '../../../frontend/src/pages/AutomaticRecognitionPage.vue'
import ToastHost from '../../../frontend/src/components/ui/ToastHost.vue'
import ConfirmDialog from '../../../frontend/src/components/ui/ConfirmDialog.vue'
import { provideRuntime } from '../../../frontend/src/app/runtime'
import { createQueryClient } from '../../../frontend/src/app/query'
import type { Recognition, Entry } from '../../../frontend/src/model'
import '../../../frontend/src/style.css'

// Entirely in-memory transport. This page never reads or changes user media.
const params = new URLSearchParams(location.search)
const purpose = params.get('purpose') === 'scrape' ? 'scrape' : 'organize'
const movie = params.has('movie')
const narrow = params.has('narrow')
const paged = params.has('pages')
const mapping = params.has('mapping')
const sourceState = params.get('sources')
const sourcePaths = ['/隔离测试/单文件/作品.S01E03.mp4', '/隔离测试/单文件/作品.S01E04.mp4']
const work: Entry = { type: movie ? 'movie' : 'tv', tmdbId: 223911, title: movie ? '示例电影' : '仙逆', originalTitle: '', year: 2023, overview: '', posterPath: '' }
const works: Entry[] = paged ? [work,{...work,type:'movie',title:'示例电影'}] : params.has('works') ? [work, ...Array.from({length:5},(_,index)=>({...work,tmdbId:223912+index,title:`示例作品 ${index+2}`,year:2018+index}))] : [work]
const paths = mapping ? Array.from({length:42},(_,index)=>`/隔离测试/映射预览/Season 01/仙逆.S01E${String(index+1).padStart(2,'0')}.2023.2160p.WEB-DL.H265.AAC.mp4`) : paged ? Array.from({length:84},(_,index)=>`/隔离测试/分页/${index < 42 ? '剧集' : '电影'}${index%42+1}.mp4`) : ['/隔离测试/来自：分享/【X】《亻山》NI（4K）/1-159/1.mp4','/隔离测试/来自：分享/【X】《亻山》NI（4K）/1-159/2.mp4','/隔离测试/其他作品/Season 02/第03集.mp4']
let task: Recognition | null = params.has('empty') ? null : { id: 'fixture', purpose, revision: 1, state: 'completed', paths: ['/隔离测试'], items: paths.map((path,index) => {
  const entry = mapping ? work : paged ? works[index < 42 ? 0 : 1]! : null, season = entry?.type === 'tv' ? 1 : -1, episodes = entry?.type === 'tv' ? [index+1] : []
  return { path, size: 1024, mtime: '1', group: path, season, episodes, metadata: {title:'仙逆',season:1,episodes:mapping?episodes:[],mediaType:'tv',tmdbId:null,year:2023},entry,candidates:works,state:entry?'ready':'review',reason:entry?'':'待确认',selected:false,removed:false,traceId:'fixture-trace',confirmation:entry ? {source:'user',at:'',identity:'',size:1024,mTime:1,type:entry.type,tmdbId:entry.tmdbId,season,episodes,basis:{kind:'manual',index:0,offset:0}} : undefined}
}), completed:paths.length,total:paths.length,created:'',error:'' }
const sourceItems = task?.items.slice(0, 2).map((file, index) => ({ ...file, path: sourcePaths[index]!, group: sourcePaths[index]! })) || []
if (task && sourceState) {
  task.paths = sourceState === 'empty' ? [] : sourceState === 'completed' ? sourcePaths : ['/隔离测试/空目录']
  task.items = sourceState === 'completed' ? sourceItems.map(file => ({ ...file, removed: true, execution: { runId: 'finished', state: 'completed', locked: false, completed: true, manualReview: false } })) : []
  task.completed = task.total = task.items.length
}
const initial = task ? structuredClone(task) : null
window.fetch = async (input, options) => {
  const url = String(input), body = options?.body ? JSON.parse(options.body as string) : null
  let value: unknown = null
  if (url.startsWith('/api/recognitions')) {
    if (url.endsWith('/candidates')) value = task!.items.filter(file => body.paths.includes(file.path)).map(file => {
      const text = file.path.split('/').pop()!.replace(/\.[^.]+$/, '')
      const span = {sourceId:'name',start:text.indexOf('E')+1,end:text.indexOf('E')+3}
      const document = { path:file.path,fileId:file.path,directory:file.path.slice(0,file.path.lastIndexOf('/')),sources:[{id:'name',kind:'filename',depth:0,text},...(mapping?[{id:'dir-0',kind:'directories',depth:1,text:'Season 01'}]:[])],tokens:[],candidates:mapping?[{id:'episodes:0',ruleId:'episodes',role:'episodes',value:file.metadata.episodes,spans:[span],strength:'heuristic',status:'valid',reason:'季集标记'},{id:'episodes:1',ruleId:'merged',role:'episodes',value:[file.metadata.episodes[0],file.metadata.episodes[0]!+1],spans:[span],strength:'heuristic',status:'valid',reason:'连续集数'}]:[] }
      return {path:file.path,traceId:file.traceId,document,suggestion:{fileId:file.path,path:file.path,metadata:file.metadata,decisions:{}}}
    })
    else if (url.endsWith('/confirm')) {
      task!.revision++
      task!.items = task!.items.map(file => {
        const update = body.items.find((row: any) => row.path === file.path)
        const picked = works.find(item=>item.tmdbId===update?.tmdbId && item.type===update?.type) || work
        return update ? {...file,entry:picked,group:`${picked.type}:${picked.tmdbId}`,season:update.season,episodes:update.episodes,state:'ready',selected:true,confirmation:{source:'user',at:'',identity:'',size:file.size,mTime:1,type:picked.type,tmdbId:picked.tmdbId,season:update.season,episodes:update.episodes,basis:update.basis},layout:purpose === 'scrape' ? {workDir:'/隔离测试/仙逆 (2023)',seasonDir:'/隔离测试/仙逆 (2023)/Season 01',moviePrefix:false,manual:false}:undefined} : file
      })
      value = task
    } else if (options?.method === 'PUT') { task!.revision++; task!.items = task!.items.map(file => ({...file,...body.items.find((row: any) => row.path === file.path)})); value = task }
    else value = task
  }
  if (url.startsWith('/api/search')) value = works.filter(item=>item.title.includes(new URL(url,location.origin).searchParams.get('q') || ''))
  if (url.startsWith('/api/seasons')) value = [{number:1,name:'第一季',count:20},{number:2,name:'第二季',count:20}]
  if (url.startsWith('/api/episodes')) {
    if (params.has('slow-validation')) await new Promise(resolve=>setTimeout(resolve,5000))
    value = Array.from({length:paged || mapping ? 100 : 20},(_,index) => ({number:index+1,season:1,name:`第 ${index+1} 集`,airDate:''}))
  }
  if (url === '/api/queue') value = {active:false,recognizing:false,revision:1,organize:{current:null,queued:0,queuedBatches:0,waitReason:''},scrape:{current:null,queued:0,queuedBatches:0,waitReason:''}}
  if (url === '/api/settings') value = {}
  if (url === '/api/batches/preview') {
    const rows = task!.items.filter(file => file.selected).map(file => ({source:file.path,target:`/隔离测试/Target/仙逆 (2023)/Season 01/仙逆 - S01E${String(file.episodes[0]).padStart(2,'0')}.mp4`,season:file.season,episodes:file.episodes,artifacts:[],issues:params.has('conflict')?['目标已存在']:[],action:'create'}))
    const group = {id:'group',title:work.title,mode:body.mode,duplicatePolicy:body.duplicatePolicy,rows,required:2048,issues:[],type:work.type,tmdbId:work.tmdbId}
    value = {...group,id:'preview',groups:[group],issues:params.has('conflict')?['目标路径冲突']:[]}
  }
  if (url === '/api/scrape/batches/preview') value = {id:'scrape-preview',title:work.title,content:body.content,onlyMissing:body.onlyMissing,targets:task!.items.filter(file=>file.selected).map(file=>file.path),artifacts:['/隔离测试/仙逆 (2023)/tvshow.nfo','/隔离测试/仙逆 (2023)/poster.jpg','/隔离测试/仙逆 (2023)/Season 01/poster.jpg'],skipped:[],issues:[],groups:[],required:1024}
  if (url.endsWith('/execute')) { task!.items=task!.items.map(file=>file.selected?{...file,selected:false,execution:{runId:'isolated-run',state:'queued',locked:true,completed:false,manualReview:false}}:file);value={id:'isolated-batch',runs:[]} }
  if (url.startsWith('/api/browse')) value = sourceState && new URL(url, location.origin).searchParams.get('path') ? sourcePaths.map(path => ({path, dir:false})) : [{path:'/隔离测试',dir:true}]
  if (url === '/api/recognitions' && body) {
    if (sourceState && !body.id) {
      const items = sourceItems.filter(file => body.paths.includes(file.path))
      task = {id:'fixture-new',purpose,revision:1,state:'completed',paths:body.paths,items,completed:items.length,total:items.length,created:'',error:''}
    } else task = initial || {id:'fixture',purpose,revision:1,state:'completed',paths:['/隔离测试'],items:[],completed:0,total:0,created:'',error:''}
    value=task
  }
  return new Response(JSON.stringify(value),{status:200})
}
const app = createApp({setup(){provideRuntime();return()=>h('main',{style:`padding:${narrow ? '14px' : '32px'};max-width:${narrow ? '390px' : '1380px'};margin:auto`},[h(RouterView),h(ToastHost),h(ConfirmDialog)])}})
app.use(VueQueryPlugin,{queryClient:createQueryClient()})
const router = createRouter({history:createMemoryHistory(),routes:[{path:'/',component:AutomaticRecognitionPage,props:{purpose}},{path:'/history/:traceId',name:'extraction-history-detail',component:{render:()=>null}},{path:'/:any(.*)*',component:{render:()=>null}}]})
app.use(router)
await router.isReady()
app.mount('#app')
