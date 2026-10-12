import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {parse,compileScript} from '@vue/compiler-sfc';
import {build} from 'esbuild';
let source=(await readFile('src/components/MusicCanteen.vue','utf8')).replace(/\/\/ #ifdef MP-WEIXIN[\s\S]*?\/\/ #endif/g,'');
source=source.replace('import { request, ApiError } from "../lib/api";','const request=(...args:any[])=>globalThis.canteenRequest(...args);class ApiError extends Error {constructor(message:string,public status:number){super(message)}}');
source=source.replace('import { player as platformPlayer } from "../lib/player";','const platformPlayer={reset(){}};');
const {descriptor}=parse(source);const compiled=compileScript(descriptor,{id:'canteen-playback-test'});
const bundle=await build({stdin:{contents:compiled.content+'\nexport {createRenderer,nextTick} from "vue";',loader:'ts',resolveDir:resolve('src/components')},bundle:true,write:false,platform:'node',format:'esm'});
const {default:Canteen,createRenderer,nextTick}=await import('data:text/javascript;base64,'+Buffer.from(bundle.outputFiles[0].text).toString('base64'));
Canteen.render=()=>null;
const renderer=createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}});
const settle=async()=>{await new Promise(r=>setImmediate(r));await nextTick()};
const skipped=async()=>{await new Promise(r=>setTimeout(r,850));await settle()};
const tracks=['1','2','3'].map(id=>({id,source:'netease',name:'歌曲'+id,artist:'歌手',album:'',version:'',genre:'',duration:120}));
function setup(resolveURL=async()=>({url:'https://music.example/song.mp3'})){
 const calls=[];globalThis.canteenRequest=async(path,method,data)=>{
  if(path==='/canteen/favorites')return {tracks:[]};
  if(path.startsWith('/canteen/lyrics'))return {lyric:'',translation:''};
  if(path==='/canteen/playback'){calls.push(data.id);return resolveURL(data.id)}
  throw Error('unexpected request '+path);
 };
 const app=renderer.createApp(Canteen);app.mount({});const state=app._instance.setupState;
 const audio={paused:true,error:null,currentTime:0,duration:120,volume:.65,load(){},removeAttribute(){},pause(){this.paused=true},async play(){this.paused=false;state.onPlaying({currentTarget:audio})}};
 state.audio=audio;state.tracks=tracks;
 return {app,state,audio,calls,end(){audio.paused=true;state.onEnded({currentTarget:audio})}};
}
test('search play-all snapshots the queue and advances on ended through the entire list',async()=>{
 const x=setup();try{
 x.state.playAll();await settle();assert.deepEqual(x.calls,['1']);
 x.state.tracks=[{...tracks[0],id:'unrelated-search'}];
 x.end();await settle();assert.deepEqual(x.calls,['1','2']);
 x.end();await settle();assert.deepEqual(x.calls,['1','2','3']);
 x.end();await settle();assert.deepEqual(x.calls,['1','2','3']);
 }finally{x.app.unmount()}
});
test('unavailable next playback URL is skipped and the queue continues',async()=>{
 const x=setup(async id=>{if(id==='2')throw Error('暂无可播放地址');return {url:'https://music.example/'+id}});
 try{x.state.playAll();await settle();x.end();await settle();assert.deepEqual(x.calls,['1','2']);await skipped();assert.deepEqual(x.calls,['1','2','3']);assert.equal(x.state.isPlaying,true)}finally{x.app.unmount()}
});
test('media loading error skips a broken track; stop cancels pending automatic work',async()=>{
 const x=setup();try{x.state.playAll();await settle();x.state.onAudioError({currentTarget:x.audio});await skipped();assert.deepEqual(x.calls,['1','2']);x.state.onAudioError({currentTarget:x.audio});x.state.stop();await skipped();assert.deepEqual(x.calls,['1','2'])}finally{x.app.unmount()}
});
test('an entirely unavailable looping queue stops after each track has been tried once',async()=>{
 const x=setup(async()=>{throw Error('unavailable')});try{x.state.setPlayMode('loop');x.state.playAll();await settle();await skipped();await skipped();await skipped();assert.deepEqual(x.calls,['1','2','3']);assert.match(x.state.playback,/都无法播放/)}finally{x.app.unmount()}
});
test('browser autoplay denial keeps the selected next song for manual continuation',async()=>{
 const x=setup();try{x.state.playAll();await settle();x.audio.play=async()=>{const e=Error('gesture required');e.name='NotAllowedError';throw e};x.end();await settle();await skipped();assert.deepEqual(x.calls,['1','2']);assert.match(x.state.playback,/浏览器/)}finally{x.app.unmount()}
});
