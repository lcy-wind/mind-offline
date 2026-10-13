import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {transformWithEsbuild} from 'vite';
import {parse,compileScript} from '@vue/compiler-sfc';
import {build} from 'esbuild';
const lib=await transformWithEsbuild(await readFile('src/lib/work-tasks.ts','utf8'),'work.ts',{loader:'ts',target:'es2022'});
const {workDateParts,workPlannedAt,sortWorkTasks}=await import('data:text/javascript;base64,'+Buffer.from(lib.code).toString('base64'));
test('planned time remains Beijing time across UTC date boundaries and an unscheduled task stays empty',()=>{
 assert.deepEqual(workDateParts('2026-10-19T16:30:00Z'),{day:'2026-10-20',time:'00:30'});
 assert.equal(workPlannedAt('2026-10-20','09:30'),'2026-10-20T09:30:00+08:00');
 assert.equal(workPlannedAt('','09:00'),null);
});
test('unfinished urgent work comes first and completed work stays below without mutating data',()=>{
 const base={title:'工作',created_at:'2026-10-12T00:00:00Z',updated_at:'2026-10-12T00:00:00Z',planned_at:null};
 const items=[{...base,id:'done',urgency:3,status:'done'},{...base,id:'normal',urgency:2,status:'todo'},{...base,id:'urgent',urgency:3,status:'doing'}];
 assert.deepEqual(sortWorkTasks(items).map(x=>x.id),['urgent','normal','done']);assert.equal(items[0].id,'done');
});
const {descriptor}=parse((await readFile('src/components/WorkTasks.vue','utf8')).replace("import WorkDatePicker from './WorkDatePicker.vue';",'const WorkDatePicker={};').replace("import WorkTimePicker from './WorkTimePicker.vue';",'const WorkTimePicker={};'));const script=compileScript(descriptor,{id:'work-test'});
const bundled=await build({stdin:{contents:script.content+'\nexport {createRenderer,nextTick} from "vue";',loader:'ts',resolveDir:resolve('src/components')},bundle:true,write:false,platform:'node',format:'esm'});
const {default:WorkTasks,createRenderer,nextTick}=await import('data:text/javascript;base64,'+Buffer.from(bundled.outputFiles[0].text).toString('base64'));WorkTasks.render=()=>null;
const renderer=createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}});
const settle=async()=>{await new Promise(r=>setImmediate(r));await nextTick()};
function mount(){const pending=[];globalThis.uni={getStorageSync:()=> 'owner-session',request:o=>{pending.push(o);return {abort(){}}}};const app=renderer.createApp(WorkTasks,{foreground:true});app.mount({});return {app,pending,state:app._instance.setupState};}
function finish(r,status,data){r.success({statusCode:status,data});}
test('save uses the selected schedule, keeps the draft on error, and publishes only confirmed server data',async()=>{
 const {app,pending,state}=mount();try{
 finish(pending[0],200,{tasks:[]});await settle();state.edit();state.title='写方案';state.day='2026-10-20';state.clock='09:30';state.urgency=3;
 const first=state.save();assert.equal(pending[1].data.planned_at,'2026-10-20T09:30:00+08:00');finish(pending[1],500,{error:'暂时失败'});await first;
 assert.equal(state.formOpen,true);assert.equal(state.title,'写方案');assert.equal(state.tasks.length,0);
 const retry=state.save();finish(pending[2],201,{id:'task1',...pending[2].data,created_at:'2026-10-12T00:00:00Z',updated_at:'2026-10-12T00:00:00Z'});await retry;
 assert.equal(state.tasks[0].title,'写方案');assert.equal(state.formOpen,false);
 }finally{app.unmount()}
});
test('late account data cannot repopulate an unmounted work area',async()=>{
 const {app,pending,state}=mount();app.unmount();finish(pending[0],200,{tasks:[{id:'private-old-task'}]});await settle();assert.equal(state.tasks.length,0);
});
test('email recipient is user supplied and reminder time is independent of planned work time',async()=>{
 const {app,pending,state}=mount();try{
 finish(pending[0],200,{tasks:[],email_available:true});await settle();state.edit();
 assert.equal(state.recipient,'');assert.equal(state.mailEnabled,false);
 state.title='开会';state.day='2027-01-20';state.clock='10:00';state.mailEnabled=true;state.recipient='recipient@example.com';state.remindDay='2027-01-20';state.remindClock='09:30';
 const save=state.save();assert.deepEqual(pending[1].data.reminder,{enabled:true,email:'recipient@example.com',at:'2027-01-20T09:30:00+08:00'});assert.equal(pending[1].data.planned_at,'2027-01-20T10:00:00+08:00');
 finish(pending[1],201,{id:'mail-task',...pending[1].data,reminder_email:'recipient@example.com',reminder_at:'2027-01-20T01:30:00Z',reminder_state:'pending',created_at:'2026-10-12T00:00:00Z',updated_at:'2026-10-12T00:00:00Z'});await save;
 state.edit(state.tasks[0]);assert.equal(state.recipient,'recipient@example.com');assert.equal(state.remindClock,'09:30');assert.equal(state.mailEnabled,true);
 }finally{app.unmount()}
});
