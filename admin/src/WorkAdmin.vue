<script setup lang="ts">
import {ref,onMounted,onUnmounted} from 'vue';
import {workDateParts,workStatuses,workUrgencies,workReminderLabels,type WorkTask} from '../../miniapp/src/lib/work-tasks';
type Customer={id:string;username:string;total:number;todo:number;doing:number;done:number;updated_at:string|null};
type Page<T>={items:T[];total:number;page:number;page_size:number};
const props=defineProps<{api:(path:string)=>Promise<any>;download:(path:string,filename:string)=>Promise<void>;accountId:string}>();
const selected=ref(props.accountId),customerSearch=ref(''),taskSearch=ref(''),status=ref('all'),page=ref(1),busy=ref(false),exporting=ref(false),error=ref('');
const customers=ref<Page<Customer>>({items:[],total:0,page:1,page_size:20});
const report=ref<(Page<WorkTask>&{customer:{id:string;username:string}})|null>(null);
let active=true,seq=0;
const date=(s?:string|null)=>{if(!s)return '未安排';const d=workDateParts(s);return d.day+' '+d.time;};
function query(){return new URLSearchParams({status:status.value,q:taskSearch.value.trim()});}
async function load(reset=false){if(reset)page.value=1;const n=++seq;busy.value=true;error.value='';try{
 if(selected.value){const data=await props.api('/admin/customers/'+encodeURIComponent(selected.value)+'/work-tasks?'+query()+'&page='+page.value);if(active&&n===seq)report.value=data;}
 else{const data=await props.api('/admin/work-customers?'+new URLSearchParams({q:customerSearch.value.trim(),page:String(page.value)}));if(active&&n===seq)customers.value=data;}
}catch(e){if(active&&n===seq)error.value=(e as Error).message;}finally{if(active&&n===seq)busy.value=false;}}
function open(id:string){selected.value=id;report.value=null;taskSearch.value='';status.value='all';load(true);}
function back(){selected.value='';report.value=null;load(true);}
async function exportExcel(){if(!report.value||exporting.value)return;exporting.value=true;error.value='';try{await props.download('/admin/customers/'+encodeURIComponent(selected.value)+'/work-tasks/export?'+query(),report.value.customer.username+'_工作记录.xlsx');}catch(e){if(active)error.value=(e as Error).message;}finally{if(active)exporting.value=false;}}
onMounted(()=>load());onUnmounted(()=>{active=false;seq++;});
</script>
<template>
<section class="work-admin">
 <div class="heading"><div><button v-if="selected" class="text-button" @click="back">← 顾客列表</button><h2>{{selected?(report?'@'+report.customer.username+' 的工作记录':'正在读取顾客工作…'):'顾客工作列表'}}</h2><p class="muted">{{selected?'只读查看 · 时间均为北京时间':'选择一位顾客，查看其牛马痛苦区记录'}}</p></div><button class="outline" :disabled="busy" @click="load()">刷新</button></div>
 <p v-if="error" class="error" role="alert">{{error}}</p>
 <template v-if="!selected"><form class="filters" @submit.prevent="load(true)"><input v-model="customerSearch" placeholder="搜索顾客用户名" maxlength="80" aria-label="搜索工作记录所属顾客"/><button class="primary" :disabled="busy">搜索</button></form>
 <div class="table-wrap"><table><thead><tr><th>顾客</th><th>工作总数</th><th>待开始</th><th>进行中</th><th>已完成</th><th>最近更新</th><th></th></tr></thead><tbody><tr v-for="c in customers.items" :key="c.id"><td>@{{c.username}}</td><td>{{c.total}}</td><td>{{c.todo}}</td><td>{{c.doing}}</td><td>{{c.done}}</td><td>{{c.updated_at?date(c.updated_at):'暂无记录'}}</td><td><button class="outline" @click="open(c.id)">查看工作</button></td></tr><tr v-if="!customers.items.length"><td colspan="7">{{busy?'正在读取…':'暂无符合条件的顾客'}}</td></tr></tbody></table></div>
 <div class="pager"><button class="outline" :disabled="busy||page<=1" @click="page--;load()">上一页</button><span>第 {{page}} 页 · {{customers.total}} 位顾客</span><button class="outline" :disabled="busy||page*20>=customers.total" @click="page++;load()">下一页</button></div></template>
 <template v-else><form class="filters" @submit.prevent="load(true)"><input v-model="taskSearch" placeholder="搜索工作内容" maxlength="80" aria-label="搜索顾客工作"/><select v-model="status" aria-label="筛选工作进度"><option value="all">全部进度</option><option value="active">未完成</option><option v-for="s in workStatuses" :key="s.id" :value="s.id">{{s.label}}</option></select><button class="primary" :disabled="busy">筛选</button><button type="button" class="outline" :disabled="busy||exporting||!report?.total" @click="exportExcel">{{exporting?'导出中…':'↓ 导出 Excel'}}</button></form>
 <p class="muted">当前筛选 {{report?.total||0}} 项 · 导出包含符合条件的全部记录</p>
 <div class="table-wrap"><table><thead><tr><th>工作内容</th><th>计划时间</th><th>紧急程度</th><th>进度</th><th>邮件提醒</th><th>更新时间</th></tr></thead><tbody><tr v-for="t in report?.items||[]" :key="t.id"><td class="task-title">{{t.title}}</td><td>{{date(t.planned_at)}}</td><td><span :class="{urgent:t.urgency===3}">{{workUrgencies[t.urgency-1].label}}</span></td><td>{{workStatuses.find(s=>s.id===t.status)?.label}}</td><td>{{workReminderLabels[t.reminder_state||'off']}}<small v-if="t.reminder_at">{{date(t.reminder_at)}}</small><small v-if="t.reminder_email">{{t.reminder_email}}</small><small v-if="t.reminder_error" class="error-note">{{t.reminder_error}}</small></td><td>{{date(t.updated_at)}}</td></tr><tr v-if="!report?.items.length"><td colspan="6">{{busy?'正在读取…':'该顾客暂无符合条件的工作'}}</td></tr></tbody></table></div>
 <div class="pager"><button class="outline" :disabled="busy||page<=1" @click="page--;load()">上一页</button><span>第 {{page}} 页</span><button class="outline" :disabled="busy||page*20>=(report?.total||0)" @click="page++;load()">下一页</button></div></template>
</section>
</template>
<style scoped>
.work-admin{background:#fffdf8;border:1px solid #e5dfd4;border-radius:20px;padding:24px}.heading{display:flex;align-items:center;justify-content:space-between;gap:14px}.filters{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin:20px 0}.filters input{flex:1;min-width:170px}.filters input,.filters select{padding:12px;border:1px solid #ddd4c7;border-radius:8px;background:white}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse}th,td{padding:14px 10px;text-align:left;border-bottom:1px solid #ece6dc;font-size:13px;vertical-align:top}td{line-height:1.7}small{display:block;color:#948572;overflow-wrap:anywhere}.task-title{min-width:200px;max-width:320px;white-space:pre-wrap;overflow-wrap:anywhere}.urgent,.error-note{color:#a86a4d}.pager{display:flex;align-items:center;justify-content:center;gap:12px;margin-top:22px}@media(max-width:650px){.work-admin{padding:14px}.filters input{flex-basis:100%}}
</style>
