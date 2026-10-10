import test from 'node:test';import assert from 'node:assert/strict';import {readFile} from 'node:fs/promises';import{transformWithEsbuild}from'vite';
const compiled=await transformWithEsbuild(await readFile('src/lib/order-progress.ts','utf8'),'progress.ts',{loader:'ts',target:'es2022'});
const{orderProgress}=await import('data:text/javascript;base64,'+Buffer.from(compiled.code).toString('base64'));
const start=Date.parse('2026-10-10T00:00:00Z');const order={status:'pending',auto_started_at:new Date(start).toISOString(),step_seconds:30};
test('countdown reaches each deadline without claiming unconfirmed status',()=>{
 assert.equal(orderProgress(order,start).remaining,30);assert.equal(orderProgress(order,start+29000).remaining,1);
 const due=orderProgress(order,start+30000);assert.equal(due.due,true);assert.match(due.hint,/同步/);assert.equal(due.percent,33);
 assert.equal(orderProgress({...order,status:'cooking'},start+30000).remaining,30);
 assert.equal(orderProgress({...order,status:'ready'},start+60000).remaining,30);
 assert.equal(orderProgress(order,start+90000).percent,33);
});
test('cancelled, completed and legacy records stay terminal or untimed',()=>{
 assert.equal(orderProgress({...order,status:'cancelled'},start+100000).due,false);
 assert.equal(orderProgress({...order,status:'completed'},start+100000).percent,100);
 assert.equal(orderProgress({status:'pending'},start).remaining,null);
 assert.ok(orderProgress(order,start-5000).percent>=0);
});
