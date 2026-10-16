import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {parse,compileScript} from '@vue/compiler-sfc';
import {build} from 'esbuild';
const source=(await readFile('../admin/src/App.vue','utf8')).replace('import HealingAdmin from "./HealingAdmin.vue";','const HealingAdmin={};').replace('import BeadsAdmin from "./BeadsAdmin.vue";','const BeadsAdmin={};').replace('import WorkAdmin from "./WorkAdmin.vue";','const WorkAdmin={};');
const {descriptor}=parse(source);
const script=compileScript(descriptor,{id:'admin-access-test'});
const bundled=await build({stdin:{contents:script.content+'\nexport {createRenderer,nextTick} from "vue";',loader:'ts',resolveDir:resolve('src/components')},bundle:true,write:false,platform:'node',format:'esm'});
const {default:Admin,createRenderer,nextTick}=await import('data:text/javascript;base64,'+Buffer.from(bundled.outputFiles[0].text).toString('base64'));
Admin.render=()=>null;
const renderer=createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}});
const settle=async()=>{await new Promise(resolve=>setImmediate(resolve));await nextTick()};
function mount(){
 const pending=[],redirects=[];
 globalThis.sessionStorage={removeItem(){}};
 globalThis.document={hidden:false,addEventListener(){},removeEventListener(){}};
 globalThis.window={location:{replace:path=>redirects.push(path)},addEventListener(){},removeEventListener(){}};
 globalThis.fetch=(path,options)=>new Promise(resolve=>pending.push({path,options,resolve}));
 const app=renderer.createApp(Admin);app.mount({});return {app,pending,redirects};
}
function finish(r,status,data){r.resolve({ok:status===200,status,json:async()=>data});}
test('admin loads no private overview before owner identity is verified',async()=>{
 const {app,pending,redirects}=mount();
 try{
  assert.deepEqual(pending.map(x=>x.path),['/api/admin/me']);
  finish(pending[0],403,{error:'仅管理员可以访问'});await settle();
  assert.equal(pending.length,1);assert.ok(redirects.includes('/'));
 }finally{app.unmount()}
});
test('verified owner can load data, and an expired session redirects to normal account login',async()=>{
 const {app,pending,redirects}=mount();
 try{
  finish(pending[0],200,{is_admin:true,username:'cole'});await settle();
  assert.ok(pending.some(x=>x.path==='/api/admin/overview'));
  const overview=pending.find(x=>x.path==='/api/admin/overview');
  assert.equal(overview.options.headers.Authorization,undefined);
  finish(overview,401,{error:'请登录后继续'});await settle();assert.ok(redirects.includes('/'));
 }finally{app.unmount()}
});
