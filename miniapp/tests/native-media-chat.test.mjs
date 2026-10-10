import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
async function load(entry) {
 const r=await build({entryPoints:[entry],bundle:true,write:false,platform:'node',format:'esm'});
 return import('data:text/javascript;base64,'+Buffer.from(r.outputFiles[0].text).toString('base64'));
}
const {nativeHealing,createUTF8Decoder}=await load('src/lib/healing-native.ts');
const {createCanteenAudio}=await load('src/lib/canteen-native-audio.ts');
const encoder=new TextEncoder();
const sse='event: delta\ndata: {"text":"你好🎵"}\n\nevent: done\ndata: {"status":"complete"}\n\n';
function setup() {
 let token='a', options, chunk, headers, aborted=0;
 globalThis.uni={getStorageSync:()=>token,request:o=>{options=o;return {abort(){aborted++;o.fail({errMsg:'abort'})},onChunkReceived(fn){chunk=fn},onHeadersReceived(fn){headers=fn}}}};
 return {switch(){token='b'},get options(){return options},get aborted(){return aborted},send(text){for(const b of encoder.encode(text))chunk({data:Uint8Array.of(b).buffer})},headers(code){headers({statusCode:code})}};
}
test('native UTF-8 decoder keeps Chinese and emoji across every byte boundary',()=>{
 const bytes=encoder.encode('中文🎵𠮷尾');
 for(let i=0;i<=bytes.length;i++) {const decode=createUTF8Decoder();assert.equal(decode(bytes.slice(0,i).buffer)+decode(bytes.slice(i).buffer)+decode(undefined,true),'中文🎵𠮷尾');}
});
test('WeChat chunked chat streams once, preserves auth and requires done',async()=>{
 const mock=setup(), events=[];const run=nativeHealing('abc','id','hello',e=>events.push(e));
 assert.equal(mock.options.header.Authorization,'Bearer a');assert.equal(mock.options.enableChunked,true);
 mock.headers(200);mock.send(sse);mock.options.success({statusCode:200,data:sse});await run.promise;
 assert.deepEqual(events.map(e=>e.event),['delta','done']);assert.equal(events[0].data.text,'你好🎵');
 const bad=nativeHealing('abc','id2','hello',()=>{});mock.send('event: delta\ndata: {"text":"partial"}\n\n');mock.options.success({statusCode:200,data:''});await assert.rejects(bad.promise,/未完成/);
});
test('WeChat buffered clients, HTTP auth errors and cancellation settle correctly',async()=>{
 const mock=setup(),events=[];const buffered=nativeHealing('a','1','hi',e=>events.push(e));mock.options.success({statusCode:200,data:sse});await buffered.promise;assert.equal(events.length,2);
 const denied=nativeHealing('a','2','hi',()=>{});mock.headers(401);mock.send('{"error":"登录过期"}');mock.options.success({statusCode:401,data:'{"error":"登录过期"}'});await assert.rejects(denied.promise,e=>e.status===401);
 const cancelEvents=[];const stopped=nativeHealing('a','3','hi',e=>cancelEvents.push(e));stopped.abort();mock.send(sse);await assert.rejects(stopped.promise,e=>e.name==='AbortError');assert.equal(cancelEvents.length,0);
});
test('WeChat account switch rejects response and ignores subsequent chunks',async()=>{
 const mock=setup(),events=[];const run=nativeHealing('a','1','hi',e=>events.push(e));mock.switch();mock.send(sse);await assert.rejects(run.promise,/账号已切换/);assert.equal(events.length,0);assert.equal(mock.aborted,1);
});
test('native audio retires old callbacks, seeks, changes volume and destroys on cleanup',async()=>{
 const contexts=[];globalThis.uni={createInnerAudioContext(){const callbacks={};const a={currentTime:0,duration:120,destroyed:false,play(){callbacks.Play()},pause(){callbacks.Pause()},seek(s){this.currentTime=s},destroy(){this.destroyed=true},callbacks};for(const n of ['Canplay','TimeUpdate','Play','Pause','Ended','Waiting','Error'])a['on'+n]=fn=>callbacks[n]=fn;contexts.push(a);return a;}};
 let url='https://example.com/one.mp3';const events=[];const audio=createCanteenAudio(()=>url,e=>events.push(e));
 audio.load();await audio.play();assert.equal(audio.paused,false);audio.currentTime=30;assert.equal(audio.currentTime,30);audio.volume=.3;assert.equal(contexts[0].volume,.3);
 const old=contexts[0];url='https://example.com/two.mp3';audio.load();assert.equal(old.destroyed,true);events.length=0;old.callbacks.Ended();old.callbacks.Error();assert.deepEqual(events,[]);assert.equal(audio.error,false);
 contexts[1].callbacks.Error();assert.equal(audio.error,true);assert.equal(audio.paused,true);assert.deepEqual(events,['error']);audio.destroy();assert.equal(contexts[1].destroyed,true);events.length=0;contexts[1].callbacks.Play();assert.deepEqual(events,[]);
});
