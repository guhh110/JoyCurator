import { createRenderer, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useConfirmation } from '../../../frontend/src/features/workspace/useConfirmation'
import { digitParts, digitPattern, takeDigits, episodeCandidates } from '../../../frontend/src/features/workspace/confirmation'
import type { CandidateView } from '../../../frontend/src/features/workspace/confirmation'
import { recognitionGroups } from '../../../frontend/src/recognition'
import type { Recognition, RecognitionItem, Entry } from '../../../frontend/src/model'
import { clearToasts } from '../../../frontend/src/app/toast'
const renderer = createRenderer<object, object>({ createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert:()=>{},remove:()=>{},setText:()=>{},setElementText:()=>{},patchProp:()=>{},parentNode:()=>null,nextSibling:()=>null })
const cleanup: (()=>void)[]=[]
afterEach(()=>{cleanup.splice(0).forEach(fn=>fn());clearToasts();vi.unstubAllGlobals()})
const metadata={title:'1',tmdbId:null,mediaType:null,year:null,season:null,episodes:[]}
const work:Entry={type:'tv',tmdbId:2,title:'作品',originalTitle:'',year:2024,overview:'',posterPath:''}
function view(name:string,values:number[]=[]):CandidateView {return {path:`/target/1-159/${name}.mp4`,traceId:'snapshot',document:{fileId:name,path:`/target/1-159/${name}.mp4`,directory:'/target/1-159',sources:[{id:'name',kind:'filename',depth:0,text:name},{id:'dir-0',kind:'directories',depth:1,text:'1-159'}],tokens:[],candidates:values.length?[{id:'episodes:0',ruleId:'episodes',role:'episodes',value:values,spans:[{sourceId:'name',start:0,end:name.length}],strength:'heuristic',status:'valid',reason:'数字'}]:[]},suggestion:{fileId:name,path:`/target/1-159/${name}.mp4`,metadata:{...metadata,episodes:values},decisions:{}}}}
function file(v:CandidateView):RecognitionItem{return {path:v.path,group:v.path,state:'review',reason:'待确认',size:10,mtime:'1',season:-1,episodes:[],metadata:{...metadata},entry:null,selected:false,edited:false,removed:false,traceId:v.traceId}}
async function settle(){for(let i=0;i<15;i++)await Promise.resolve();await nextTick()}
function fixture(views=[view('1'),view('2')]){
 const task=ref<Recognition|null>({purpose:'scrape',id:'w',revision:3,state:'completed',paths:['/target'],items:views.map(file),total:views.length,completed:views.length,error:'',created:''})
 const busy=ref(false),invalidate=vi.fn();let state!:ReturnType<typeof useConfirmation>
 const app=renderer.createApp({setup(){state=useConfirmation({task,busy,act:async fn=>{busy.value=true;try{await fn()}finally{busy.value=false}},accept:value=>{task.value=value},invalidate});return()=>h('div')}});app.mount({});cleanup.push(()=>app.unmount())
 return{state,task,invalidate,views}
}
function backend(views:CandidateView[],posted:(body:any)=>unknown){return vi.fn(async(path:string,options:RequestInit)=>{
 let result:unknown=null
 if(path.endsWith('/candidates'))result=views
 else if(path.startsWith('/api/seasons'))result=[{number:1,name:'第一季',count:4}]
 else if(path.startsWith('/api/episodes'))result=[{number:1},{number:2},{number:3},{number:4}]
 else if(path.endsWith('/confirm'))result=posted(JSON.parse(options.body as string))
 return new Response(JSON.stringify(result))
})}
describe('explicit workspace confirmation',()=>{
 it('keeps directory selection separate from inferred titles',()=>{
  const files=[file(view('1')),file(view('2'))]
  expect(recognitionGroups(files,'works')).toHaveLength(2)
  expect(recognitionGroups(files,'directories')).toHaveLength(1)
  expect(files.every(f=>!f.selected)).toBe(true)
 })
 it('restores an unconfirmed manual work and empty season draft after reidentification',async()=>{
  const {state,task,views}=fixture([view('1',[1])])
  const item=task.value!.items[0]!
  item.entry=work;item.edited=true;item.season=-1;item.episodes=[]
  vi.stubGlobal('fetch',backend(views,()=>task.value))
  await state.start(task.value!.items);await settle()
  const row=state.rows.value[0]!
  expect(row.entry).toEqual(work);expect(row.season).toBe(-1);expect(row.text).toBe('')
  expect(row.basis.kind).toBe('manual');expect(state.group('tv:2').canConfirm.value).toBe(false)
 })
 it('maps bare filenames by clicked digits and submits one explicit confirmation',async()=>{
  const {state,task,invalidate,views}=fixture();let input:any
  const fetch=backend(views,body=>{input=body;return {...task.value!,revision:4,items:task.value!.items.map((item,index)=>({...item,entry:work,season:body.items[index].season,episodes:body.items[index].episodes,confirmation:{basis:body.items[index].basis}}))}});vi.stubGlobal('fetch',fetch)
  await state.start(task.value!.items);const g=state.group('tv:2')
  expect(g.entry.value).toBeNull();expect(g.canConfirm.value).toBe(false)
  await state.group('unassigned').pick(work);await settle()
  g.mode.value='digits';g.applyMapping('0');await settle()
  expect(state.rows.value.map(r=>r.text)).toEqual(['1','2'])
  expect(g.canConfirm.value).toBe(true)
  await g.confirm()
  expect(input).toMatchObject({revision:3,items:[{episodes:[1],season:1,basis:{kind:'digits',index:0,traceId:'snapshot'}},{episodes:[2],season:1}]})
  expect(input.items[0]).not.toHaveProperty('selected')
  expect(invalidate).toHaveBeenCalledOnce();expect(state.open.value).toBe(true);expect(state.dirty.value).toBe(false)
 })
 it('requires mismatching rows to be corrected or excluded and preserves missing episodes',async()=>{
  const {state,task,views}=fixture([view('1'),view('3'),view('extra4')]);vi.stubGlobal('fetch',backend(views,()=>task.value))
  await state.start(task.value!.items);const g=state.group('tv:2');await state.group('unassigned').pick(work);await settle();g.mode.value='digits';g.applyMapping('0');await settle()
  expect(state.rows.value.map(r=>r.text)).toEqual(['1','3',''])
  expect(state.rows.value[2]!.error).toContain('结构');expect(g.canConfirm.value).toBe(false)
  state.rows.value[2]!.included=false;await settle();expect(g.canConfirm.value).toBe(true)
  state.rows.value[2]!.included=true;state.rows.value[2]!.text='4';state.manual(state.rows.value[2]!);await settle();expect(g.canConfirm.value).toBe(true)
 })
 it('keeps pending TMDB checks out of row errors without allowing confirmation',async()=>{
  const {state,task,views}=fixture()
  let finishSeasons!:(value:Response)=>void,finishEpisodes!:(value:Response)=>void
  const base=backend(views,()=>task.value)
  vi.stubGlobal('fetch',vi.fn((path:string,options:RequestInit)=>path.startsWith('/api/seasons')?new Promise<Response>(done=>{finishSeasons=done}):path.startsWith('/api/episodes')?new Promise<Response>(done=>{finishEpisodes=done}):base(path,options)))
  await state.start(task.value!.items);const g=state.group('tv:2')
  state.rows.value.forEach((row,index)=>{row.text=String(index+1);state.manual(row)})
  const picking=state.group('unassigned').pick(work)
  expect(g.validating.value).toBe(true);expect(g.validation.value).toEqual(['',''])
  expect(g.canConfirm.value).toBe(false);expect(await g.confirm()).toBe(false)
  finishSeasons(new Response(JSON.stringify([{number:1,name:'第一季',count:2}])))
  await picking;await settle()
  expect(g.validating.value).toBe(true);expect(g.validation.value).toEqual(['',''])
  expect(g.canConfirm.value).toBe(false);expect(await g.confirm()).toBe(false)
  finishEpisodes(new Response(JSON.stringify([{number:1},{number:2}])))
  await settle()
  expect(g.validating.value).toBe(false);expect(g.canConfirm.value).toBe(true)
  state.rows.value[1]!.text='3'
  expect(g.problem(state.rows.value[1]!)).toContain('不存在');expect(g.canConfirm.value).toBe(false)
  expect(base.mock.calls.some(([path])=>path.endsWith('/confirm'))).toBe(false)
 })
 it('reports failed TMDB checks and keeps retry pending separate from errors',async()=>{
  const {state,task,views}=fixture()
  let finishEpisodes!:(value:Response)=>void
  const base=backend(views,()=>task.value)
  vi.stubGlobal('fetch',vi.fn((path:string,options:RequestInit)=>path.startsWith('/api/episodes')?new Promise<Response>(done=>{finishEpisodes=done}):base(path,options)))
  await state.start(task.value!.items);const g=state.group('tv:2');await state.group('unassigned').pick(work);await settle()
  state.rows.value.forEach((row,index)=>{row.text=String(index+1);state.manual(row)})
  finishEpisodes(new Response(JSON.stringify({message:'网络错误'}),{status:503}));await settle()
  expect(g.validating.value).toBe(false);expect(g.problem(state.rows.value[0]!)).toContain('失败');expect(g.canConfirm.value).toBe(false)
  g.retry();await settle()
  expect(g.validating.value).toBe(true);expect(g.validation.value).toEqual(['','']);expect(g.canConfirm.value).toBe(false)
  finishEpisodes(new Response(JSON.stringify([{number:1},{number:2}])));await settle()
  expect(g.validating.value).toBe(false);expect(g.canConfirm.value).toBe(true)
 })
 it('preserves merged candidate values and offsets, rejecting stale revisions',async()=>{
  const {state,task,views}=fixture([view('第1集',[1,2]),view('第3集',[3,4])]);vi.stubGlobal('fetch',backend(views,()=>task.value))
  await state.start(task.value!.items);const g=state.group('tv:2');await state.group('unassigned').pick(work);await settle();g.mode.value='candidate';g.offset.value='-1';g.applyMapping('episodes:0')
  expect(state.rows.value.map(r=>r.text)).toEqual(['0,1','2,3'])
  expect(state.rows.value[0]!.basis).toMatchObject({kind:'candidate',candidateId:'episodes:0',offset:-1})
  task.value!.revision++;await settle();expect(g.canConfirm.value).toBe(false);expect(state.stale.value).toBe(true)
 })
 it('ignores candidate responses after closing the editor',async()=>{
  const {state,task}=fixture();let resolve!:(r:Response)=>void;vi.stubGlobal('fetch',vi.fn(()=>new Promise<Response>(done=>{resolve=done})))
  const pending=state.start(task.value!.items);state.close();resolve(new Response(JSON.stringify([view('1')])));await pending
  expect(state.open.value).toBe(false);expect(state.rows.value).toEqual([])
 })
 it('takes UTF-16 digit positions and never treats literal placeholders as equivalent',()=>{
  expect(digitParts('🏠S01E03').map(p=>p.start)).toEqual([3,6])
  expect(digitPattern('A1#')).not.toBe(digitPattern('A#1'))
  const v=view('S01E03')
  expect(takeDigits(v,'name',1,digitPattern('S01E01'),-1).episodes).toEqual([2])
  expect(()=>takeDigits(v,'name',2,digitPattern('S01E01'),0)).toThrow()
  expect(episodeCandidates(view('第一集',[1]),'episodes','name')[0]?.value).toEqual([1])
 })
})
