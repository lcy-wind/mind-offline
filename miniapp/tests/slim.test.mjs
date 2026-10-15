import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {transformWithEsbuild} from 'vite';
const compiled=await transformWithEsbuild(await readFile('src/lib/slim.ts','utf8'),'slim.ts',{loader:'ts',target:'es2022'});
const {bmi,bmiLabel,foodCalories,exerciseCalories,weightDays}=await import('data:text/javascript;base64,'+Buffer.from(compiled.code).toString('base64'));
test('BMI uses raw thresholds, invalid measurements do not produce numbers',()=>{assert.equal(bmi(200,96),24);assert.equal(bmiLabel(24),'超重范围');assert.equal(bmiLabel(23.99),'正常范围');assert.equal(bmiLabel(28),'肥胖范围');assert.equal(bmiLabel(18.5),'正常范围');assert.equal(bmi(0,60),null);assert.equal(bmi(170,NaN),null);});
test('food and exercise retain units and daily weight uses latest measurement',()=>{assert.equal(foodCalories(200,130),260);assert.equal(exerciseCalories(30,60,3.5),110.25);assert.deepEqual(weightDays([{id:'new',kind:'weight',day:'2026-10-15',weight:60},{id:'old',kind:'weight',day:'2026-10-15',weight:61},{id:'start',kind:'weight',day:'2026-10-14',weight:62}]).map(x=>x.id),['start','new']);});
import {resolve} from 'node:path';
import {parse,compileScript} from '@vue/compiler-sfc';
import {build} from 'esbuild';
const {descriptor}=parse((await readFile('src/components/SlimLab.vue','utf8')).replace("import WorkDatePicker from './WorkDatePicker.vue';",'const WorkDatePicker={};'));
const script=compileScript(descriptor,{id:'slim-test'});
const bundled=await build({stdin:{contents:script.content+'\nexport {createRenderer,nextTick} from "vue";',loader:'ts',resolveDir:resolve('src/components')},bundle:true,write:false,platform:'node',format:'esm'});
const {default:SlimLab,createRenderer,nextTick}=await import('data:text/javascript;base64,'+Buffer.from(bundled.outputFiles[0].text).toString('base64'));SlimLab.render=()=>null;
const renderer=createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}});
const settle=async()=>{await new Promise(r=>setImmediate(r));await nextTick()};
function mount(){const pending=[];globalThis.uni={getStorageSync:()=> 'owner-session',request:o=>{pending.push(o);return {abort(){}}}};const app=renderer.createApp(SlimLab,{foreground:true});app.mount({});return {app,pending,state:app._instance.setupState};}
function finish(r,status,data){r.success({statusCode:status,data});}
test('food submission retains retry ID and only adds server-confirmed records',async()=>{const {app,pending,state}=mount();try{finish(pending[0],200,{profile:{},entries:[]});await settle();state.name='米饭';state.rate='130';state.grams='200';let save=state.save();const id=pending[1].data.id;finish(pending[1],500,{error:'暂时失败'});await save;assert.equal(state.entries.length,0);save=state.save();assert.equal(pending[2].data.id,id);finish(pending[2],201,{...pending[2].data,calories:260});await save;assert.equal(state.intake,260);}finally{app.unmount()}});
test('late private data is discarded when the account component unmounts',async()=>{const {app,pending,state}=mount();app.unmount();finish(pending[0],200,{profile:{height:170},entries:[{id:'private'}]});await settle();assert.equal(state.entries.length,0);assert.equal(state.height,'');});
