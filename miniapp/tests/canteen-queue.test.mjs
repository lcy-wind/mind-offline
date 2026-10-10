import test from 'node:test';
import assert from 'node:assert/strict';
import {transformWithEsbuild} from 'vite';
import {readFile} from 'node:fs/promises';
const code=await transformWithEsbuild(await readFile('src/lib/canteen-queue.ts','utf8'),'queue.ts',{loader:'ts',target:'es2022'});
const {createQueueOrder,moveQueue}=await import('data:text/javascript;base64,'+Buffer.from(code.code).toString('base64'));
const index=s=>s.order[s.position];
test('sequential playback finishes at end and previous retraces queue',()=>{
 let s=createQueueOrder(3,0,'sequence');
 assert.equal(moveQueue(s,-1),null);
 s=moveQueue(s,1,true);assert.equal(index(s),1);
 s=moveQueue(s,1,true);assert.equal(index(s),2);
 assert.equal(moveQueue(s,1,true),null);
 assert.equal(index(moveQueue(s,-1)),1);
});
test('shuffle visits every track before starting a new round and avoids immediate repeat',()=>{
 let s=createQueueOrder(8,3,'shuffle',()=>.4);const played=[index(s)];
 for(let i=1;i<8;i++){s=moveQueue(s,1,true,()=>.4);played.push(index(s));}
 assert.equal(new Set(played).size,8);
 const last=index(s);s=moveQueue(s,1,true,()=>.2);assert.notEqual(index(s),last);
 assert.equal(index(moveQueue(s,-1)),last);
});
test('single repeat keeps same track automatically but permits manual skips',()=>{
 const s=createQueueOrder(3,1,'single');assert.equal(index(moveQueue(s,1,true)),1);
 assert.equal(index(moveQueue(s,1)),2);assert.equal(index(moveQueue(s,-1)),0);
});
test('list loop wraps and small/empty queues are safe',()=>{
 assert.equal(index(moveQueue(createQueueOrder(2,1,'loop'),1,true)),0);
 assert.equal(index(moveQueue(createQueueOrder(2,0,'loop'),-1)),1);
 for(const mode of ['single','loop','shuffle']) assert.equal(index(moveQueue(createQueueOrder(1,0,mode),1,true)),0);
 assert.equal(moveQueue(createQueueOrder(0,-1,'shuffle'),1),null);
});
test('mode changes preserve the selected track and never mutate previous order',()=>{
 const original=createQueueOrder(5,2,'sequence');const changed=createQueueOrder(5,index(original),'shuffle',()=>.5);
 assert.equal(index(changed),2);assert.deepEqual(original.order,[0,1,2,3,4]);
 const moved=moveQueue(changed,1);assert.equal(changed.position,0);assert.equal(index(moveQueue(moved,-1)),2);
});

test('each later shuffle round also contains every track exactly once',()=>{
 let s=createQueueOrder(6,0,'shuffle',()=>.3);
 for(let round=0;round<4;round++){
  const played=[index(s)];for(let i=1;i<6;i++){s=moveQueue(s,1,true,()=>.6);played.push(index(s));}
  assert.equal(new Set(played).size,6);
  const last=index(s);s=moveQueue(s,1,true,()=>.6);assert.notEqual(index(s),last);
  const previous=moveQueue(s,-1);assert.equal(index(previous),last);
  assert.deepEqual(moveQueue(previous,1).order,s.order);
 }
});
