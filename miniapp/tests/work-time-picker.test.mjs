import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {parse,compileScript} from '@vue/compiler-sfc';
import {build} from 'esbuild';
const {descriptor}=parse(await readFile('src/components/WorkTimePicker.vue','utf8'));
const compiled=compileScript(descriptor,{id:'work-time-picker-test'});
const bundle=await build({stdin:{contents:compiled.content+'\nexport {createRenderer,nextTick} from "vue";',loader:'ts',resolveDir:resolve('src/components')},bundle:true,write:false,platform:'node',format:'esm'});
const {default:Picker,createRenderer,nextTick}=await import('data:text/javascript;base64,'+Buffer.from(bundle.outputFiles[0].text).toString('base64'));Picker.render=()=>null;
const renderer=createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),insert(){},remove(){},setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null,patchProp(){}});
function mount(value){const values=[];globalThis.uni={getSystemInfoSync:()=>({windowHeight:750})};globalThis.document={activeElement:{focus(){}},getElementById:()=>null};const app=renderer.createApp(Picker,{modelValue:value,'onUpdate:modelValue':v=>values.push(v)});app.mount({});return {app,state:app._instance.setupState,values};}
test('time selection preserves minute precision and only commits on confirmation',async()=>{
 const x=mount('09:05');try{x.state.open();await nextTick();assert.equal(x.state.selected,'09:05');x.state.hour=23;x.state.minute=59;assert.deepEqual(x.values,[]);x.state.confirm();assert.deepEqual(x.values,['23:59']);assert.equal(x.state.opened,false)}finally{x.app.unmount()}
});
test('cancel keeps the original value; presets and midnight remain valid',async()=>{
 const x=mount('00:00');try{x.state.open();await nextTick();assert.equal(x.state.selected,'00:00');x.state.choose('14:00');x.state.close();assert.deepEqual(x.values,[]);x.state.open();await nextTick();assert.equal(x.state.selected,'00:00');x.state.choose('18:00');x.state.confirm();assert.deepEqual(x.values,['18:00'])}finally{x.app.unmount()}
});
test('keyboard minute/hour adjustment respects 24-hour boundaries',async()=>{
 const x=mount('23:59');try{x.state.open();await nextTick();x.state.adjust({key:'ArrowDown',preventDefault(){}},'hour');x.state.adjust({key:'ArrowDown',preventDefault(){}},'minute');assert.equal(x.state.selected,'23:59');x.state.adjust({key:'Home',preventDefault(){}},'hour');x.state.adjust({key:'Home',preventDefault(){}},'minute');assert.equal(x.state.selected,'00:00')}finally{x.app.unmount()}
});
