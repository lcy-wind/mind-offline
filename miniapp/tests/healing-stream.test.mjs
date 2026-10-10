import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {transformWithEsbuild} from 'vite';
let src=await readFile('src/lib/healing-stream.ts','utf8');
src=src.replace('import { currentToken, ApiError } from "./api";', 'const currentToken=()=>globalThis.healingTestToken;class ApiError extends Error{constructor(message,status){super(message);this.status=status}}');
const compiled=await transformWithEsbuild(src,'stream.ts',{loader:'ts',target:'es2022'});
const {createChatEventParser,streamHealing}=await import('data:text/javascript;base64,'+Buffer.from(compiled.code).toString('base64'));
test('SSE parser handles CRLF and event boundaries split across chunks',()=>{
 const events=[];const parse=createChatEventParser(e=>events.push(e));
 for(const c of ['event: del','ta\r\ndata: {"text":"你','好"}\r','\n\r\nevent: done\ndata: {"status":"complete"}\n','\n'])parse(c);
 assert.deepEqual(events,[{event:'delta',data:{text:'你好'}},{event:'done',data:{status:'complete'}}]);
});
test('stream decodes split UTF-8 and requires a completion event',async t=>{
 const old=globalThis.fetch;t.after(()=>{globalThis.fetch=old});globalThis.healingTestToken='account-a';
 let incomplete=false;
 globalThis.fetch=async(url,options)=>{
  assert.match(url,/^\/api\/healing\/conversations\/abc\/messages$/);assert.equal(options.headers.Authorization,'Bearer account-a');
  const bytes=new TextEncoder().encode('event: delta\ndata: {"text":"你好"}\n\n'+(incomplete?'':'event: done\ndata: {"status":"complete"}\n\n'));
  return new Response(new ReadableStream({start(c){for(const b of bytes)c.enqueue(Uint8Array.of(b));c.close()}}));
 };
 const events=[];await streamHealing('abc','request-1','hello',new AbortController().signal,e=>events.push(e));assert.equal(events[0].data.text,'你好');
 incomplete=true;await assert.rejects(()=>streamHealing('abc','request-2','hello',new AbortController().signal,()=>{}),/未完成/);
});
test('account changes cannot publish stale stream events',async t=>{
 const old=globalThis.fetch;t.after(()=>{globalThis.fetch=old});globalThis.healingTestToken='account-a';
 globalThis.fetch=async()=>{globalThis.healingTestToken='account-b';return new Response('event: done\ndata: {}\n\n')};
 let emitted=false;await assert.rejects(()=>streamHealing('abc','request-3','hello',new AbortController().signal,()=>{emitted=true}),/账号已切换/);assert.equal(emitted,false);
});
