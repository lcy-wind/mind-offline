import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {parse,compileScript} from '@vue/compiler-sfc';
import {build} from 'esbuild';
const source=(await readFile('src/components/HealingChat.vue','utf8')).replace(/\/\/ #ifdef MP-WEIXIN[\s\S]*?\/\/ #endif/g,'');
const {descriptor}=parse(source);
const compiled=compileScript(descriptor,{id:'healing-load-test'});
const bundle=await build({stdin:{contents:compiled.content+'\nexport {createRenderer,nextTick} from "vue";',loader:'ts',resolveDir:resolve('src/components')},bundle:true,write:false,platform:'node',format:'esm'});
const {default:HealingChat,createRenderer,nextTick}=await import('data:text/javascript;base64,'+Buffer.from(bundle.outputFiles[0].text).toString('base64'));
HealingChat.render=()=>null;
const renderer=createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}});
const settle=async()=>{await new Promise(resolve=>setImmediate(resolve));await nextTick()};
function mount(){
 const pending=[];globalThis.uni={getStorageSync:()=> 'test-session',request:options=>{pending.push(options);return {abort(){}}}};
 const app=renderer.createApp(HealingChat,{foreground:true});app.mount({});return {app,pending};
}
function finish(request,data){request.success({statusCode:200,data})}
test('opening healing starts metadata and conversation list together, then reads chosen history',async()=>{
 const {app,pending}=mount();
 try{
  assert.equal(pending.length,2);assert.ok(pending[0].url.endsWith('/healing/config'));assert.ok(pending[1].url.endsWith('/healing/conversations'));
  finish(pending[0],{enabled:true,roles:[]});await settle();assert.equal(pending.length,2);
  const c={id:'test-conversation',name:'测试',mbti:'INFP'};finish(pending[1],{conversations:[c]});await settle();
  assert.equal(pending.length,3);assert.ok(pending[2].url.endsWith('/healing/conversations/test-conversation'));
  finish(pending[2],{conversation:c,turns:[]});await settle();assert.equal(pending.length,3);
 }finally{app.unmount()}
});
test('leaving the account during initialization cannot launch an old account history request',async()=>{
 const {app,pending}=mount();app.unmount();
 finish(pending[0],{enabled:true,roles:[]});finish(pending[1],{conversations:[{id:'old-account-conversation'}]});await settle();
 assert.equal(pending.length,2);
});
