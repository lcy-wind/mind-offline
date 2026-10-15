<script setup lang="ts">
import {ref,computed,nextTick,onBeforeUnmount,getCurrentInstance} from 'vue';
import {workDateParts} from '../lib/work-tasks';
import {dateCaption,shiftDay,shiftMonth,nextMonday,monthDays} from '../lib/work-calendar';
const props=withDefaults(defineProps<{modelValue:string;title?:string;min?:string;max?:string;clearable?:boolean}>(),{title:'安排一个日子',min:'',max:'',clearable:true});
const emit=defineEmits<{(e:'update:modelValue',value:string):void}>();
const panelHeight=ref(600);
const opened=ref(false),draft=ref(''),month=ref(''),today=ref('');
const id='work-calendar-'+getCurrentInstance()?.uid;
const weekdays=['一','二','三','四','五','六','日'];
const days=computed(()=>month.value?monthDays(month.value):[]);
const monthTitle=computed(()=>month.value.slice(0,4)+'年 '+Number(month.value.slice(5))+'月');
const shortcuts=computed(()=>today.value?[{name:'今天',date:today.value},{name:'明天',date:shiftDay(today.value,1)},{name:'下周一',date:nextMonday(today.value)}]:[]);
const nextDisabled=computed(()=>!!props.max&&shiftMonth(month.value||'2026-01',1)>props.max.slice(0,7));
const previousDisabled=computed(()=>!!props.min&&shiftMonth(month.value||'2026-01',-1)<props.min.slice(0,7));
let returnFocus:HTMLElement|null=null;
function open(){panelHeight.value=Math.min(600,Math.max(300,uni.getSystemInfoSync().windowHeight-28));today.value=workDateParts(new Date().toISOString()).day;draft.value=props.modelValue||today.value;if(props.min&&draft.value<props.min)draft.value=props.min;if(props.max&&draft.value>props.max)draft.value=props.max;month.value=draft.value.slice(0,7);opened.value=true;
 // #ifdef H5
 returnFocus=document.activeElement as HTMLElement;nextTick(()=>document.getElementById(id)?.querySelector<HTMLButtonElement>('.calendar-day.selected')?.focus());
 // #endif
}
function close(){opened.value=false;
 // #ifdef H5
 returnFocus?.focus();
 // #endif
}
function choose(day:string){if((props.min&&day<props.min)||(props.max&&day>props.max))return;draft.value=day;month.value=day.slice(0,7);}
function confirm(){if(draft.value&&(!props.min||draft.value>=props.min)&&(!props.max||draft.value<=props.max)){emit('update:modelValue',draft.value);close();}}
function clear(){emit('update:modelValue','');close();}
function keys(e:KeyboardEvent){
 // #ifdef H5
 if(e.key==='Escape'){e.preventDefault();close();return;}
 if(e.key==='Tab'){
  const buttons=Array.from(document.getElementById(id)?.querySelectorAll<HTMLButtonElement>('button:not([disabled])')||[]);
  const first=buttons[0],last=buttons[buttons.length-1];
  if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus();}
 }
 // #endif
}
onBeforeUnmount(()=>{opened.value=false;});
</script>
<template>
<view class="work-date-picker">
 <button class="date-trigger" @click="open" :aria-label="title+'：'+(modelValue||'尚未选择')" :aria-expanded="opened"><view class="calendar-icon"><text>▦</text></view><view class="date-copy"><text class="date-caption">{{modelValue?dateCaption(modelValue):'选择日期'}}</text><text class="date-subtitle">{{modelValue?modelValue.slice(0,4)+' 年':clearable?'还没定？可以先留空':'选择要记录的日子'}}</text></view><text class="chevron">⌄</text></button>
 <view v-if="opened" class="calendar-backdrop" @click.self="close" @touchmove.stop.prevent>
  <view :id="id" class="calendar-panel" role="dialog" aria-modal="true" :aria-label="title" @keydown="keys" @touchmove.stop>
   <scroll-view scroll-y :style="{height:panelHeight+'px'}"><view class="calendar-inner">
   <view class="calendar-heading"><view><text class="calendar-eyebrow">给日子留个记号</text><text class="calendar-title">{{title}}</text></view><button class="calendar-close" @click="close" aria-label="关闭日期选择">×</button></view>
   <view class="quick-dates"><button v-for="s in shortcuts" :key="s.name" :class="{picked:draft===s.date}" :disabled="(!!min&&s.date<min)||(!!max&&s.date>max)" @click="choose(s.date)">{{s.name}}</button></view>
   <view class="month-heading"><button class="month-arrow" :disabled="previousDisabled" @click="month=shiftMonth(month,-1)" aria-label="上个月">‹</button><text>{{monthTitle}}</text><button class="month-arrow" :disabled="nextDisabled" @click="month=shiftMonth(month,1)" aria-label="下个月">›</button></view>
   <view class="weekdays"><text v-for="w in weekdays" :key="w">{{w}}</text></view>
   <view class="calendar-grid"><button v-for="d in days" :key="d.date" :class="['calendar-day',{outside:d.outside,selected:draft===d.date,today:d.date===today}]" :disabled="(!!min&&d.date<min)||(!!max&&d.date>max)" :aria-label="d.date" :aria-pressed="draft===d.date" @click="choose(d.date)">{{d.day}}<view v-if="d.date===today" class="today-dot"/></button></view>
   <view class="selection-summary"><text>已选 {{dateCaption(draft)}}</text><text>北京时间</text></view>
   <view class="calendar-actions"><button v-if="clearable" class="clear-date" @click="clear">暂不安排</button><button v-else class="clear-date" @click="close">取消</button><button class="confirm-date" @click="confirm">就这天 ✓</button></view>
   </view></scroll-view>
  </view>
 </view>
</view>
</template>
<style scoped>
.work-date-picker{min-width:0;flex:1;max-width:310px}.date-trigger{display:flex;align-items:center;width:100%;gap:12px;padding:12px 14px;border:1px solid #ded3c0;border-radius:12px;background:#faf7ef;text-align:left;line-height:1.4;margin:0;color:#746348;box-sizing:border-box}.date-trigger::after,.calendar-panel button::after{border:0}.calendar-icon{width:35px;height:35px;display:flex;align-items:center;justify-content:center;background:#e9e2d0;border-radius:9px;font-size:24px;color:#8c7958;flex-shrink:0}.date-copy{flex:1}.date-copy text{display:block}.date-caption{font-size:14px;font-weight:600}.date-subtitle{font-size:10px;color:#a99a81;margin-top:4px}.chevron{color:#a49780;font-size:18px}.calendar-backdrop{position:fixed;inset:0;z-index:1200;display:flex;align-items:center;justify-content:center;padding:20px;box-sizing:border-box;background:rgba(53,43,30,.32);backdrop-filter:blur(4px)}.calendar-panel{width:360px;max-width:100%;overflow:hidden;background:#fffdf7;border:1px solid #e4dbc9;border-radius:24px;padding:0;box-sizing:border-box;box-shadow:0 20px 80px rgba(68,51,24,.18);color:#665740}.calendar-inner{padding:20px;box-sizing:border-box}.calendar-heading{display:flex;justify-content:space-between;align-items:center}.calendar-heading text{display:block}.calendar-eyebrow{color:#ad9b7b;font-size:10px;letter-spacing:1px;margin-bottom:7px}.calendar-title{font-size:21px;font-weight:700}.calendar-close{font-size:24px;width:32px;height:32px;line-height:30px;margin:0;padding:0;color:#aa9b82;background:#f4efe4;border-radius:50%}.quick-dates{display:flex;gap:8px;margin:14px 0 10px}.quick-dates button{flex:1;font-size:12px;line-height:1.5;background:#f4f0e6;color:#9d8b6c;border-radius:9px;margin:0;padding:8px 0;border:1px solid transparent}.quick-dates button.picked{background:#e9eddd;border-color:#b4bc98;color:#798558}.month-heading{display:flex;align-items:center;justify-content:space-between;margin:8px 0;font-size:15px;font-weight:600}.month-arrow{margin:0;width:32px;height:32px;line-height:27px;padding:0;font-size:25px;background:none;color:#8e7d5f;border-radius:8px}.weekdays,.calendar-grid{display:grid;grid-template-columns:repeat(7,minmax(0,1fr));gap:4px}.weekdays{text-align:center;font-size:11px;color:#ac9f86;padding:6px 0 8px}.calendar-day{position:relative;display:flex;align-items:center;justify-content:center;width:100%;height:34px;font-size:13px;font-weight:500;background:transparent;color:#776850;border-radius:10px;margin:0;padding:0;line-height:1}.calendar-day.outside{color:#c9bfac}.calendar-day.selected{background:#82704f;color:#fffdf7;box-shadow:0 3px 8px #82704f26}.calendar-day.today:not(.selected){background:#f0efdf;color:#788052}.today-dot{position:absolute;bottom:4px;width:3px;height:3px;border-radius:50%;background:currentColor}.calendar-panel button[disabled]{opacity:.3}.selection-summary{display:flex;justify-content:space-between;margin:14px 0 12px;color:#8c7a5c;font-size:11px}.selection-summary text:last-child{color:#b3a78e}.calendar-actions{display:flex;align-items:center;gap:12px;border-top:1px solid #eee6d6;padding-top:12px}.clear-date{flex:1;background:none;color:#a18e71;font-size:12px;padding:10px;margin:0;line-height:1.5}.confirm-date{flex:2;background:#e5ead5;color:#737f4d;font-weight:600;border-radius:11px;font-size:13px;padding:12px;margin:0;line-height:1.5}.calendar-panel button:focus-visible,.date-trigger:focus-visible{outline:2px solid #9aab76;outline-offset:2px}@media(max-width:500px){.calendar-backdrop{padding:14px}.calendar-panel{border-radius:20px}.calendar-inner{padding:18px}.date-trigger{gap:8px;padding:10px}.calendar-icon{width:30px;height:30px;font-size:21px}.date-caption{font-size:12px}.calendar-day{height:34px}}
</style>
