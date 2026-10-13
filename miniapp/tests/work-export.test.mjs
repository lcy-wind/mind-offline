import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {transformWithEsbuild} from 'vite';
const source=(await readFile('src/lib/work-export.ts','utf8')).replace("import {apiBase,currentToken,ApiError} from './api';","const apiBase=()=>'';const currentToken=()=>globalThis.exportToken;class ApiError extends Error{constructor(message:string,public status:number){super(message)}}");
async function target(remove){const s=source.replace(new RegExp('// #ifdef '+remove+'[\\s\\S]*?// #endif','g'),'');const out=await transformWithEsbuild(s,'export.ts',{loader:'ts',target:'es2022'});return (await import('data:text/javascript;base64,'+Buffer.from(out.code).toString('base64'))).downloadWorkExcel;}
const browser=await target('MP-WEIXIN'),native=await target('H5');
test('browser exports authenticated XLSX and refuses a response after account switch',async t=>{
 const saved={fetch:globalThis.fetch,document:globalThis.document,timeout:globalThis.setTimeout,create:URL.createObjectURL,revoke:URL.revokeObjectURL};t.after(()=>{globalThis.fetch=saved.fetch;globalThis.document=saved.document;globalThis.setTimeout=saved.timeout;URL.createObjectURL=saved.create;URL.revokeObjectURL=saved.revoke});
 globalThis.exportToken='alice';let clicked=0,header='';
 const anchor={href:'',download:'',click(){clicked++},remove(){}};globalThis.document={createElement:()=>anchor,body:{appendChild(){}}};URL.createObjectURL=()=> 'blob:report';URL.revokeObjectURL=()=>{};globalThis.setTimeout=fn=>{fn();return 0};
 globalThis.fetch=async(url,options)=>{header=options.headers.Authorization;return {ok:true,blob:async()=>new Blob(['xlsx'])}};
 await browser('/work-tasks/export?status=active','工作.xlsx');assert.equal(clicked,1);assert.equal(header,'Bearer alice');assert.equal(anchor.download,'工作.xlsx');
 globalThis.fetch=async()=>({ok:true,blob:async()=>{globalThis.exportToken='bob';return new Blob(['private'])}});
 await assert.rejects(()=>browser('/work-tasks/export'),/账号已切换/);assert.equal(clicked,1);
});
test('mini-program opens the XLSX with save/share menu and deletes stale private downloads',async()=>{
 globalThis.exportToken='alice';let pending,opened=0,deleted=0;
 globalThis.uni={downloadFile:o=>{pending=o},openDocument:o=>{assert.equal(o.fileType,'xlsx');assert.equal(o.showMenu,true);opened++;o.success()},getFileSystemManager:()=>({unlink(){deleted++}})};
 let p=native('/work-tasks/export');assert.equal(pending.header.Authorization,'Bearer alice');pending.success({statusCode:200,tempFilePath:'/temp/report.xlsx'});await p;assert.equal(opened,1);
 p=native('/work-tasks/export');globalThis.exportToken='bob';pending.success({statusCode:200,tempFilePath:'/temp/private.xlsx'});await assert.rejects(()=>p,/账号已切换/);assert.equal(opened,1);assert.equal(deleted,1);
});
