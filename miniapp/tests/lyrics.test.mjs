import test from 'node:test';
import assert from 'node:assert/strict';
import {transformWithEsbuild} from 'vite';
import {readFile} from 'node:fs/promises';
const compiled=await transformWithEsbuild(await readFile('src/lib/lyrics.ts','utf8'),'lyrics.ts',{loader:'ts',target:'es2022'});
const {parseLyrics,activeLyricIndex}=await import('data:text/javascript;base64,'+Buffer.from(compiled.code).toString('base64'));
test('LRC supports offsets, repeated timestamps, fractions and out-of-order lines',()=>{
 const x=parseLyrics('[ar:Artist]\n[offset:500]\n[00:12.250]third\n[00:01.2][00:03.20]first\n[00:03.200]second');
 assert.deepEqual(x.lines,[{time:.7,text:'first'},{time:2.7,text:'first / second'},{time:11.75,text:'third'}]);
 assert.deepEqual(x.plain,[]);
});
test('timestamp lookup follows seeks both forward and backward including intro',()=>{
 const x=parseLyrics('[00:05]one\n[00:15]two\n[00:30]three').lines;
 assert.equal(activeLyricIndex(x,0),-1);
 assert.equal(activeLyricIndex(x,15),1);
 assert.equal(activeLyricIndex(x,29),1);
 assert.equal(activeLyricIndex(x,6),0);
 assert.equal(activeLyricIndex([],8),-1);
});
test('translations match their times, not their array positions',()=>{
 const x=parseLyrics('[00:01]one\n[00:02]two\n[00:03]three','[00:01.1]一\n[00:03]三');
 assert.equal(x.lines[0].translation,'一');assert.equal(x.lines[1].translation,undefined);assert.equal(x.lines[2].translation,'三');
});
test('plain lyrics, instrumental notes and empty lyrics remain readable',()=>{
 assert.deepEqual(parseLyrics('纯音乐，请欣赏').plain,['纯音乐，请欣赏']);
 assert.deepEqual(parseLyrics('[ti:Name]\n[by:Author]'),{lines:[],plain:[]});
 assert.deepEqual(parseLyrics('','[00:01]translation only').lines,[{time:1,text:'translation only'}]);
 assert.equal(parseLyrics('[00:01]<script>text</script>').lines[0].text,'<script>text</script>');
});
