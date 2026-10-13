<script setup lang="ts">
import {ref} from 'vue';
import AdminWorkTasks from '../../components/AdminWorkTasks.vue';
const section=ref<'healing'|'work'>('healing');
import {onShow,onHide,onUnload} from '@dcloudio/uni-app';
import {request,type Guest} from '../../lib/api';
import {formatHealingReply} from '../../lib/healing-text';
interface Conversation{id:string;username:string;title:string;mbti:string;name:string;style:string;turn_count:number;failed_count:number;updated_at:string}
interface Turn{id:string;user:string;assistant:string;status:string;created_at:string}
interface Page<T>{items:T[];total:number;page:number;page_size:number}
const allowed=ref(false),busy=ref(false),error=ref(''),q=ref(''),role=ref(''),page=ref(1);
const list=ref<Page<Conversation>>({items:[],total:0,page:1,page_size:20});
const overview=ref<{customers:number;today_orders:number}|null>(null);
const detail=ref<(Page<Turn>&{conversation:Conversation})|null>(null),selected=ref(''),detailPage=ref(1);
const roles=['全部人格','INTJ','INTP','ENTJ','ENTP','INFJ','INFP','ENFJ','ENFP','ISTJ','ISFJ','ESTJ','ESFJ','ISTP','ISFP','ESTP','ESFP'];
const state:Record<string,string>={complete:'已完成',pending:'生成中',failed:'失败',interrupted:'已中断'};
let generation=0;
function clear(){generation++;allowed.value=false;overview.value=null;list.value={items:[],total:0,page:1,page_size:20};detail.value=null;selected.value='';busy.value=false;}
function failure(e:unknown){error.value=(e as Error).message;}
function date(s:string){return s.replace('T',' ').slice(0,16);}
async function load(reset=false){
 if(!allowed.value)return;if(reset)page.value=1;
 const n=++generation;busy.value=true;error.value='';
 try{const data=await request<Page<Conversation>>('/admin/healing/conversations?page='+page.value+'&q='+encodeURIComponent(q.value)+'&mbti='+encodeURIComponent(role.value));if(n===generation)list.value=data;}
 catch(e){if(n===generation)failure(e);}finally{if(n===generation)busy.value=false;}
}
async function open(id:string,p=1){
 const n=++generation;busy.value=true;selected.value=id;detailPage.value=p;detail.value=null;error.value='';
 try{const data=await request<Page<Turn>&{conversation:Conversation}>('/admin/healing/conversations/'+encodeURIComponent(id)+'?page='+p);if(n===generation)detail.value=data;}
 catch(e){if(n===generation)failure(e);}finally{if(n===generation)busy.value=false;}
}
function back(){if(selected.value){generation++;selected.value='';detail.value=null;busy.value=false;error.value='';}else uni.navigateBack();}
function switchSection(value:'healing'|'work'){generation++;selected.value='';detail.value=null;busy.value=false;error.value='';section.value=value;if(value==='healing')load(true);}
function pick(e:any){role.value=roles[Number(e.detail.value)]==='全部人格'?'':roles[Number(e.detail.value)];load(true);}
onShow(async()=>{clear();const n=generation;error.value='';busy.value=true;try{
 const me=await request<Guest>('/me');if(n!==generation)return;
 if(!me.is_admin){error.value='当前账号没有管理员权限';return;}
 const stats=await request<{customers:number;today_orders:number}>('/admin/overview');if(n!==generation)return;
 overview.value=stats;allowed.value=true;if(section.value==='healing')await load();
}catch(e){if(n===generation)failure(e);}finally{if(n===generation)busy.value=false;}});
onHide(clear);onUnload(clear);
</script>
<template>
<view class="admin-page"><view class="header"><button class="back" @click="back">← {{selected?'会话列表':'返回食堂'}}</button><text class="heading">cole 的管理后台</text></view>
<text v-if="error" class="error" selectable>{{error}}</text><text v-if="busy" class="muted">正在读取…</text>
<template v-if="allowed"><view class="admin-sections"><button :class="{selected:section==='healing'}" @click="switchSection('healing')">AI 会话</button><button :class="{selected:section==='work'}" @click="switchSection('work')">顾客工作记录</button></view><AdminWorkTasks v-if="section==='work'" @auth-expired="clear"/><view v-else>
 <template v-if="!selected"><view class="stats"><text>顾客 {{overview?.customers||0}}</text><text>今日订单 {{overview?.today_orders||0}}</text><text>会话 {{list.total}}</text></view><text class="title">用户 AI 会话</text>
 <view class="search"><input v-model="q" placeholder="搜索顾客、会话或角色" :maxlength="80" confirm-type="search" @confirm="load(true)"/><button :disabled="busy" @click="load(true)">搜索</button></view>
 <picker :range="roles" @change="pick"><view class="filter">{{role||'全部人格'}} ▾</view></picker>
 <button v-for="c in list.items" :key="c.id" class="card" @click="open(c.id)"><text class="title">{{c.title}}</text><text>@{{c.username}} · {{c.name}} / {{c.mbti}}</text><text class="muted">{{c.turn_count}} 轮 · {{date(c.updated_at)}}<text v-if="c.failed_count"> · {{c.failed_count}} 轮未完成</text></text><text class="link">查看完整聊天 →</text></button>
 <text v-if="!busy&&!list.items.length" class="muted">暂无符合条件的会话</text>
 <view class="pager"><button :disabled="busy||page<=1" @click="page--;load()">上一页</button><text>第 {{page}} 页</text><button :disabled="busy||page*20>=list.total" @click="page++;load()">下一页</button></view></template>
 <template v-else-if="detail"><text class="title">{{detail.conversation.title}}</text><text class="muted">@{{detail.conversation.username}} · {{detail.conversation.name}} / {{detail.conversation.mbti}}</text><text v-if="detail.conversation.style" class="muted">角色补充：{{detail.conversation.style}}</text>
 <view v-for="t in detail.items" :key="t.id" class="turn"><text class="muted">{{date(t.created_at)}} · {{state[t.status]||t.status}}</text><view class="message user"><text class="label">@{{detail.conversation.username}}</text><text selectable>{{t.user}}</text></view><view class="message"><text class="label">{{detail.conversation.name}}</text><text selectable>{{formatHealingReply(t.assistant)||'尚无回复内容'}}</text></view></view>
 <text v-if="!detail.items.length" class="muted">这段会话还没有消息。</text>
 <view class="pager"><button :disabled="busy||detailPage<=1" @click="open(selected,detailPage-1)">上一页</button><text>{{detailPage}} / {{Math.max(1,Math.ceil(detail.total/20))}} 页</text><button :disabled="busy||detailPage*20>=detail.total" @click="open(selected,detailPage+1)">下一页</button></view></template>
 <button v-if="error" @click="selected?open(selected,detailPage):load()">重试</button>
</view></template></view>
</template>
<style scoped>
.admin-page{padding:20px 16px 40px;max-width:900px;margin:auto;background:#f8f6ee;min-height:100vh;box-sizing:border-box;color:#4e4845}.header{display:flex;align-items:center;gap:12px;margin-bottom:24px}.heading{font-size:18px;font-weight:700}.back{margin:0;font-size:12px}.stats{display:flex;gap:15px;background:#e9eed6;padding:16px;border-radius:12px;margin-bottom:22px;font-size:13px}.title{display:block;font-size:17px;font-weight:650;margin:10px 0}.search{display:flex;gap:10px;align-items:center;margin:16px 0}.search input{flex:1;background:white;border:1px solid #ddd;padding:10px;border-radius:8px;font-size:14px}.search button,.pager button{font-size:13px}.filter{padding:10px;border:1px solid #ddd;border-radius:8px;margin-bottom:15px}.card{display:block;width:100%;text-align:left;background:#fffdf8;padding:16px;margin:12px 0;border:1px solid #e4ddd2;border-radius:12px;line-height:1.6}.card text{display:block;font-size:13px}.card .title{font-size:16px}.muted{display:block;color:#918777;font-size:12px;line-height:1.7}.link{color:#7e6990;margin-top:10px}.pager{display:flex;align-items:center;justify-content:center;gap:12px;margin-top:25px;font-size:13px}.turn{margin:24px 0}.message{background:#eee7f3;border-radius:12px;padding:14px;margin-top:10px;white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.65;font-size:14px}.message.user{background:#e9eed6}.message .label{display:block;color:#887695;margin-bottom:8px;font-size:12px}.error{display:block;color:#ac463c;margin:15px 0}
.admin-sections{display:flex;gap:10px;margin:15px 0 25px}.admin-sections button{flex:1;margin:0;font-size:13px;background:#eee7d9;color:#9a876b}.admin-sections button.selected{background:#dedfc9;color:#768052}
</style>
