<script setup lang="ts">
import {ref,computed,nextTick,getCurrentInstance} from 'vue';
const props=withDefaults(defineProps<{modelValue:string;title?:string}>(),{title:'几点开始？'});
const emit=defineEmits<{(e:'update:modelValue',value:string):void}>();
const opened=ref(false),hour=ref(9),minute=ref(0),panelHeight=ref(540);
const id='work-time-'+getCurrentInstance()?.uid;
const hours=Array.from({length:24},(_,i)=>i),minutes=Array.from({length:60},(_,i)=>i);
const presets=['09:00','12:00','14:00','18:00'];
const pad=(n:number)=>String(n).padStart(2,'0');
const selected=computed(()=>pad(hour.value)+':'+pad(minute.value));
const hourScroll=computed(()=>Math.max(0,hour.value*40-60));
const minuteScroll=computed(()=>Math.max(0,minute.value*40-60));
let returnFocus:HTMLElement|null=null;
function choose(value:string){const valid=/^(?:[01]\d|2[0-3]):[0-5]\d$/.test(value)?value:'09:00';const parts=valid.split(':');hour.value=Number(parts[0]);minute.value=Number(parts[1]);}
function open(){choose(props.modelValue);panelHeight.value=Math.min(540,Math.max(300,uni.getSystemInfoSync().windowHeight-28));opened.value=true;
 // #ifdef H5
 returnFocus=document.activeElement as HTMLElement;nextTick(()=>document.getElementById(id)?.querySelector<HTMLElement>('.time-value.selected')?.focus());
 // #endif
}
function close(){opened.value=false;
 // #ifdef H5
 returnFocus?.focus();
 // #endif
}
function confirm(){emit('update:modelValue',selected.value);close();}
function adjust(e:KeyboardEvent,field:'hour'|'minute'){
 // #ifdef H5
 const value=field==='hour'?hour:minute,max=field==='hour'?23:59;
 const change=e.key==='ArrowUp'?-1:e.key==='ArrowDown'?1:0;
 if(change||e.key==='Home'||e.key==='End'){
  e.preventDefault();value.value=e.key==='Home'?0:e.key==='End'?max:Math.max(0,Math.min(max,value.value+change));
  nextTick(()=>document.getElementById(id)?.querySelector<HTMLElement>('.'+field+'-column .time-value.selected')?.focus());
 }
 // #endif
}
function keys(e:KeyboardEvent){
 // #ifdef H5
 if(e.key==='Escape'){e.preventDefault();close();return;}
 if(e.key==='Tab'){
  const controls=Array.from(document.getElementById(id)?.querySelectorAll<HTMLElement>('button:not([disabled]),uni-button:not([disabled])')||[]);
  const first=controls[0],last=controls[controls.length-1];
  if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus();}
 }
 // #endif
}
</script>
<template>
<view class="work-time-picker">
 <button class="time-trigger" tabindex="0" @click="open" :aria-label="title+'：'+modelValue" :aria-expanded="opened"><text class="trigger-label">时间</text><view><text>{{modelValue||'09:00'}}</text><text class="chevron">⌄</text></view></button>
 <view v-if="opened" class="time-backdrop" @click.self="close" @touchmove.stop.prevent>
  <view :id="id" class="time-panel" role="dialog" aria-modal="true" :aria-label="title" @keydown="keys" @touchmove.stop>
   <scroll-view scroll-y :style="{height:panelHeight+'px'}"><view class="time-inner">
    <view class="time-heading"><view><text class="eyebrow">给今天留个时间</text><text class="time-title">{{title}}</text></view><button class="close-time" tabindex="0" @click="close" aria-label="关闭时间选择">×</button></view>
    <view class="time-preview"><text>{{pad(hour)}}</text><text class="colon">:</text><text>{{pad(minute)}}</text><text class="timezone">北京时间 · 24 小时制</text></view>
    <view class="time-presets"><button v-for="p in presets" :key="p" :class="{picked:selected===p}" tabindex="0" @click="choose(p)">{{p}}</button></view>
    <view class="time-columns"><view class="time-column hour-column"><text class="column-label">小时</text><scroll-view scroll-y :scroll-top="hourScroll" class="number-list"><button v-for="h in hours" :key="h" :class="['time-value',{selected:hour===h}]" :tabindex="hour===h?0:-1" :aria-label="h+'时'" :aria-pressed="hour===h" @click="hour=h" @keydown="adjust($event,'hour')">{{pad(h)}}</button></scroll-view></view><view class="time-column minute-column"><text class="column-label">分钟</text><scroll-view scroll-y :scroll-top="minuteScroll" class="number-list"><button v-for="m in minutes" :key="m" :class="['time-value',{selected:minute===m}]" :tabindex="minute===m?0:-1" :aria-label="m+'分'" :aria-pressed="minute===m" @click="minute=m" @keydown="adjust($event,'minute')">{{pad(m)}}</button></scroll-view></view></view>
    <text class="time-hint">上下滑动，点选小时和分钟</text>
    <view class="time-actions"><button class="cancel-time" tabindex="0" @click="close">取消</button><button class="confirm-time" tabindex="0" @click="confirm">就这个点 ✓</button></view>
   </view></scroll-view>
  </view>
 </view>
</view>
</template>
<style scoped>
.work-time-picker{flex:0 0 102px}.time-trigger{display:flex;flex-direction:column;justify-content:center;gap:5px;width:100%;min-height:66px;padding:12px 14px;border:1px solid #ded3c0;border-radius:12px;background:#faf7ef;color:#746348;text-align:left;margin:0;line-height:1.4;box-sizing:border-box}.time-trigger::after,.time-panel button::after{border:0}.trigger-label{font-size:10px;color:#a99a81}.time-trigger>view{display:flex;gap:12px;align-items:center;justify-content:space-between;font-size:15px;font-weight:600}.chevron{color:#a49780}.time-backdrop{position:fixed;inset:0;z-index:1200;display:flex;align-items:center;justify-content:center;padding:14px;box-sizing:border-box;background:rgba(53,43,30,.32);backdrop-filter:blur(4px)}.time-panel{width:360px;max-width:100%;overflow:hidden;background:#fffdf7;border:1px solid #e4dbc9;border-radius:24px;box-shadow:0 20px 80px rgba(68,51,24,.18);color:#665740}.time-inner{padding:20px;box-sizing:border-box}.time-heading{display:flex;align-items:center;justify-content:space-between}.time-heading text{display:block}.eyebrow{font-size:10px;color:#ad9b7b;letter-spacing:1px;margin-bottom:7px}.time-title{font-size:21px;font-weight:700}.close-time{width:32px;height:32px;line-height:30px;padding:0;margin:0;border-radius:50%;font-size:24px;background:#f4efe4;color:#aa9b82}.time-preview{position:relative;display:flex;align-items:center;justify-content:center;gap:10px;margin:14px 0 12px;padding-bottom:18px;line-height:1.2;font-size:34px;font-weight:650;letter-spacing:2px;font-variant-numeric:tabular-nums}.colon{color:#b0a184;font-weight:400}.timezone{position:absolute;bottom:0;font-size:10px;font-weight:400;letter-spacing:.5px;color:#ab9b7f}.time-presets{display:flex;gap:7px;margin-bottom:14px}.time-presets button{flex:1;padding:8px 0;margin:0;line-height:1.5;font-size:12px;border:1px solid transparent;border-radius:8px;background:#f4f0e6;color:#9d8b6c}.time-presets button.picked{background:#e9eddd;border-color:#b4bc98;color:#798558}.time-columns{display:flex;gap:12px}.time-column{flex:1;min-width:0}.column-label{display:block;text-align:center;font-size:11px;color:#a99a81;margin-bottom:10px}.number-list{height:160px;scrollbar-width:thin;scrollbar-color:#cfc2aa transparent;background:#f6f2e9;border:1px solid #eee5d6;border-radius:12px;box-sizing:border-box}.time-value{display:block;height:40px;line-height:40px;padding:0;margin:0;width:100%;border-radius:9px;background:transparent;color:#9a8a6e;font-size:16px;font-variant-numeric:tabular-nums}.time-value.selected{background:#82704f;color:#fffdf7;font-weight:600}.time-hint{display:block;text-align:center;font-size:10px;color:#b1a389;margin:12px 0 16px}.time-actions{display:flex;gap:12px;border-top:1px solid #eee6d6;padding-top:14px}.cancel-time{flex:1;padding:11px;margin:0;line-height:1.5;font-size:12px;background:none;color:#a18e71}.confirm-time{flex:2;padding:12px;margin:0;line-height:1.5;font-size:13px;font-weight:600;background:#e5ead5;color:#737f4d;border-radius:11px}.time-panel button:focus-visible,.time-trigger:focus-visible{outline:2px solid #9aab76;outline-offset:2px}@media(max-width:500px){.work-time-picker{flex-basis:94px}.time-trigger{padding:10px;min-height:59px}.time-panel{border-radius:20px}.time-inner{padding:18px}}
.time-panel :deep(.uni-scroll-view){scrollbar-width:thin;scrollbar-color:#cfc2aa transparent}
</style>
