<script setup lang="ts">
import {ref,onMounted,onBeforeUnmount} from 'vue';
import {request,ApiError} from '../lib/api';
import {downloadWorkExcel} from '../lib/work-export';
import {workDateParts,workStatuses,workUrgencies,workReminderLabels,type WorkTask} from '../lib/work-tasks';
type Customer={id:string;username:string;total:number;todo:number;doing:number;done:number};
type Page<T>={items:T[];total:number;page:number;page_size:number};
const emit=defineEmits<{(e:'auth-expired'):void}>();
const selected=ref(''),q=ref(''),status=ref('all'),page=ref(1),busy=ref(false),exporting=ref(false),error=ref('');
const customers=ref<Page<Customer>>({items:[],total:0,page:1,page_size:20});
const report=ref<(Page<WorkTask>&{customer:{id:string;username:string}})|null>(null);
let disposed=false,seq=0;
function date(s?:string|null){if(!s)return '未安排';const d=workDateParts(s);return d.day+' '+d.time;}
function fail(e:unknown){if(disposed)return;if(e instanceof ApiError&&(e.status===401||e.status===403))emit('auth-expired');error.value=(e as Error).message;}
function filters(){return 'status='+encodeURIComponent(status.value)+'&q='+encodeURIComponent(q.value.trim());}
async function load(reset=false){if(reset)page.value=1;const n=++seq;busy.value=true;error.value='';try{
 if(selected.value){const d=await request<Page<WorkTask>&{customer:{id:string;username:string}}>('/admin/customers/'+selected.value+'/work-tasks?'+filters()+'&page='+page.value);if(!disposed&&n===seq)report.value=d;}
 else{const d=await request<Page<Customer>>('/admin/work-customers?q='+encodeURIComponent(q.value.trim())+'&page='+page.value);if(!disposed&&n===seq)customers.value=d;}
 }catch(e){if(n===seq)fail(e);}finally{if(!disposed&&n===seq)busy.value=false;}}
function open(id:string){selected.value=id;q.value='';status.value='all';report.value=null;load(true);}
function back(){selected.value='';q.value='';report.value=null;load(true);}
async function exportExcel(){if(exporting.value||!report.value)return;exporting.value=true;error.value='';try{await downloadWorkExcel('/admin/customers/'+selected.value+'/work-tasks/export?'+filters(),report.value.customer.username+'_工作记录.xlsx');}catch(e){fail(e);}finally{if(!disposed)exporting.value=false;}}
onMounted(()=>load());onBeforeUnmount(()=>{disposed=true;seq++;});
</script>
<template>
<view class="admin-work"><button v-if="selected" class="text-link" @click="back">← 返回顾客列表</button><text class="heading">{{selected?(report?'@'+report.customer.username+' 的工作记录':'正在读取…'):'顾客工作列表'}}</text><text class="muted">{{selected?'时间均为北京时间 · 只读查看':'选择顾客，查看其牛马痛苦区记录'}}</text>
 <view class="search"><input v-model="q" :placeholder="selected?'搜索工作内容':'搜索顾客用户名'" :maxlength="80" @confirm="load(true)"/><button :disabled="busy" @click="load(true)">搜索</button></view>
 <view v-if="selected" class="filters"><button :class="{picked:status==='all'}" @click="status='all';load(true)">全部</button><button v-for="s in workStatuses" :key="s.id" :class="{picked:status===s.id}" @click="status=s.id;load(true)">{{s.label}}</button></view>
 <text v-if="error" class="error" selectable>{{error}}</text><text v-if="busy" class="muted">正在读取…</text>
 <template v-if="!selected"><button v-for="c in customers.items" :key="c.id" class="customer" @click="open(c.id)"><text class="title">@{{c.username}}</text><text>工作 {{c.total}} 项 · 进行中 {{c.doing}} · 已完成 {{c.done}}</text><text class="link">查看工作记录 →</text></button><text v-if="!busy&&!customers.items.length" class="muted">暂无符合条件的顾客</text></template>
 <template v-else><view class="export-row"><text class="muted">共 {{report?.total||0}} 项</text><button :disabled="busy||exporting||!report?.total" @click="exportExcel">{{exporting?'导出中…':'↓ 导出 Excel'}}</button></view><view v-for="t in report?.items||[]" :key="t.id" class="task"><text class="title">{{t.title}}</text><text>{{workUrgencies[t.urgency-1].label}} · {{workStatuses.find(s=>s.id===t.status)?.label}}</text><text>计划：{{date(t.planned_at)}}</text><text>邮件：{{workReminderLabels[t.reminder_state||'off']}}</text><text v-if="t.reminder_at">提醒：{{date(t.reminder_at)}}</text><text v-if="t.reminder_email">收件：{{t.reminder_email}}</text><text v-if="t.reminder_error" class="error">{{t.reminder_error}}</text><text class="muted">更新于 {{date(t.updated_at)}}</text></view><text v-if="!busy&&!report?.items.length" class="muted">暂无符合条件的工作</text></template>
 <view class="pager"><button :disabled="busy||page<=1" @click="page--;load()">上一页</button><text>第 {{page}} 页</text><button :disabled="busy||page*20>=(selected?(report?.total||0):customers.total)" @click="page++;load()">下一页</button></view>
</view>
</template>
<style scoped>
.admin-work{color:#665a49}.heading,.title{display:block;font-weight:650;line-height:1.7}.heading{font-size:20px;margin:14px 0}.title{font-size:16px;margin-bottom:8px}.muted{display:block;font-size:12px;color:#a0927b;line-height:1.7}.search{display:flex;gap:10px;margin:20px 0}.search input{flex:1;min-width:0;padding:10px;border:1px solid #ddd3c1;border-radius:8px;background:#fffdf8;font-size:14px}.search button,.export-row button,.pager button{font-size:12px;margin:0;background:#e9e2d1;color:#817054}.filters{display:flex;flex-wrap:wrap;gap:6px}.filters button{font-size:12px;padding:7px 12px;margin:0;background:#f0eade;color:#9d8a69;line-height:1.5}.filters button.picked{background:#dfdcc7;color:#6c7349}.customer,.task{display:block;background:#fffdf7;border:1px solid #e2d9c8;border-radius:12px;padding:16px;margin:12px 0;text-align:left;line-height:1.8;overflow-wrap:anywhere}.customer text,.task text{display:block;font-size:13px}.customer .title,.task .title{font-size:16px;white-space:pre-wrap}.link{color:#a18b65;margin-top:8px}.text-link{background:none;text-align:left;font-size:12px;color:#99876a;padding:0;margin:12px 0}.export-row{display:flex;align-items:center;justify-content:space-between;margin:16px 0}.error{display:block;color:#ac6650;font-size:12px;margin:10px 0}.pager{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:22px;font-size:12px}
</style>
