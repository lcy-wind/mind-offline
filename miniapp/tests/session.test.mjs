import test from 'node:test';
import assert from 'node:assert/strict';
import {transformWithEsbuild} from 'vite';
import {readFile} from 'node:fs/promises';
const compiled=await transformWithEsbuild(await readFile('src/lib/api.ts','utf8'),'api.ts',{loader:'ts',target:'es2022'});
let stored='',pending;
globalThis.uni={getStorageSync:()=>stored,request:options=>{pending=options}};
const {request,ApiError}=await import('data:text/javascript;base64,'+Buffer.from(compiled.code).toString('base64'));
test('request uses the current account and drops late responses after account switch',async()=>{
 stored='account-a';const result=request('/orders');assert.equal(pending.header.Authorization,'Bearer account-a');
 stored='account-b';pending.success({statusCode:200,data:[{id:'private-a-order'}]});
 await assert.rejects(result,e=>e instanceof ApiError&&e.status===0);
 const next=request('/orders');assert.equal(pending.header.Authorization,'Bearer account-b');pending.success({statusCode:200,data:[]});assert.deepEqual(await next,[]);
});
test('a request pending at logout cannot restore private state',async()=>{
 stored='account-a';const result=request('/me');stored='';pending.success({statusCode:200,data:{balance:300}});await assert.rejects(result,e=>e.status===0);
});
test('expired session is returned as an authentication error',async()=>{
 stored='expired-session';const result=request('/me');pending.success({statusCode:401,data:{error:'请登录后继续'}});await assert.rejects(result,e=>e instanceof ApiError&&e.status===401);
});

test('network failures distinguish WeChat domain rejection, TLS, DNS and timeout without credentials',async()=>{
 for(const [errMsg,code] of [['request:fail url not in domain list','DOMAIN'],['request:fail ssl hand shake error','TLS'],['request:fail ERR_NAME_NOT_RESOLVED','DNS'],['request:fail timeout','TIMEOUT'],['request:fail interrupted','NETWORK']]){
  const run=request('/auth/login','POST',{password:'not-to-be-displayed'});pending.fail({errMsg});await assert.rejects(run,e=>e.message.includes('NET-'+code)&&!e.message.includes('not-to-be-displayed'));
 }
 const run=request('/auth/login');pending.fail({errMsg:'request:fail https://example.com/path?token=secret Bearer session-secret'});
 await assert.rejects(run,e=>!e.message.includes('secret')&&!e.message.includes('?token'));
});
