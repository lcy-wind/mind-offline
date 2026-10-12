import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {transformWithEsbuild} from 'vite';
const src=await readFile('src/lib/healing-text.ts','utf8');
const compiled=await transformWithEsbuild(src,'healing-text.ts',{loader:'ts',target:'es2022'});
const {formatHealingReply}=await import('data:text/javascript;base64,'+Buffer.from(compiled.code).toString('base64'));
test('saved and streaming replies remove empty lines but retain content and necessary line breaks',()=>{
 assert.equal(formatHealingReply('\r\n早呀。\r\n \t\r\n今天吃面。\n\n  1. 留着步骤\n  2. 不删内容\n'), '早呀。\n今天吃面。\n  1. 留着步骤\n  2. 不删内容');
 assert.equal(formatHealingReply('吃了，番茄炒蛋。你呢？'),'吃了，番茄炒蛋。你呢？');
});
