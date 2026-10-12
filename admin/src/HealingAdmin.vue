<script setup lang="ts">
import {ref,onMounted,onUnmounted} from 'vue';
type Conversation={id:string;account_id:string;username:string;title:string;mbti:string;name:string;style:string;turn_count:number;failed_count:number;updated_at:string};
type Turn={id:string;user:string;assistant:string;status:string;created_at:string};
type Page<T>={items:T[];total:number;page:number;page_size:number};
const props=defineProps<{api:(path:string)=>Promise<any>;accountId:string}>();
const roles=['INTJ','INTP','ENTJ','ENTP','INFJ','INFP','ENFJ','ENFP','ISTJ','ISFJ','ESTJ','ESFJ','ISTP','ISFP','ESTP','ESFP'];
const q=ref(''),role=ref(''),page=ref(1),loading=ref(false),error=ref('');
const result=ref<Page<Conversation>>({items:[],total:0,page:1,page_size:20});
const selected=ref(''),detail=ref<(Page<Turn>&{conversation:Conversation})|null>(null),detailPage=ref(1),detailLoading=ref(false),detailError=ref('');
let active=true,seq=0,detailSeq=0;
const date=(s:string)=>new Date(s).toLocaleString('zh-CN',{hour12:false});
const status:Record<string,string>={complete:'已完成',pending:'生成中',failed:'失败',interrupted:'已中断'};
const text=(s:string)=>s.replace(/\r\n?/g,'\n').replace(/[ \t]*\n(?:[ \t]*\n)+/g,'\n').trim();
async function load(reset=false){
 if(reset)page.value=1;
 const n=++seq;loading.value=true;error.value='';
 try{const data=await props.api('/admin/healing/conversations?'+new URLSearchParams({page:String(page.value),q:q.value,mbti:role.value,account_id:props.accountId}));if(active&&n===seq)result.value=data;}
 catch(e){if(active&&n===seq)error.value=(e as Error).message;}
 finally{if(active&&n===seq)loading.value=false;}
}
async function open(id:string,newPage=1){
 selected.value=id;detailPage.value=newPage;detail.value=null;detailError.value='';detailLoading.value=true;
 const n=++detailSeq;
 try{const data=await props.api('/admin/healing/conversations/'+encodeURIComponent(id)+'?page='+newPage);if(active&&n===detailSeq)detail.value=data;}
 catch(e){if(active&&n===detailSeq)detailError.value=(e as Error).message;}
 finally{if(active&&n===detailSeq)detailLoading.value=false;}
}
function close(){detailSeq++;selected.value='';detail.value=null;detailLoading.value=false;}
onMounted(()=>load());onUnmounted(()=>{active=false;seq++;detailSeq++;});
</script>
<template>
<section class="healing-admin">
 <div class="intro"><div><h2>用户 AI 会话</h2><p class="muted">查看角色、会话记录及生成状态 · 共 {{ result.total }} 段会话</p></div><button class="outline" :disabled="loading" @click="load()">刷新列表</button></div>
 <p v-if="accountId" class="muted">当前仅显示选中顾客的会话，重新点击左侧 AI 会话可查看全部。</p>
 <form class="filters" @submit.prevent="load(true)"><input v-model="q" placeholder="搜索顾客、会话标题或角色名称" maxlength="80" aria-label="搜索AI会话"/><select v-model="role" aria-label="筛选MBTI"><option value="">全部人格</option><option v-for="r in roles" :key="r">{{r}}</option></select><button class="primary" :disabled="loading">搜索</button></form>
 <p v-if="error" class="error" role="alert">{{error}}</p>
 <div class="table-wrap"><table><thead><tr><th>顾客</th><th>会话</th><th>角色</th><th>对话轮数</th><th>更新于</th><th>操作</th></tr></thead><tbody>
 <tr v-for="c in result.items" :key="c.id"><td>@{{c.username}}</td><td class="title-cell">{{c.title}}</td><td>{{c.name}}<small>{{c.mbti}}</small></td><td>{{c.turn_count}}<small v-if="c.failed_count">{{c.failed_count}} 轮未完成</small></td><td>{{date(c.updated_at)}}</td><td><button class="outline" @click="open(c.id)">查看聊天</button></td></tr>
 <tr v-if="!result.items.length"><td colspan="6">{{loading?'正在读取会话…':'暂无符合条件的会话'}}</td></tr></tbody></table></div>
 <div class="pager"><button class="outline" :disabled="page<=1||loading" @click="page--;load()">上一页</button><span>第 {{page}} 页</span><button class="outline" :disabled="page*20>=result.total||loading" @click="page++;load()">下一页</button></div>
 <div v-if="selected" class="drawer-backdrop" @click.self="close"><aside class="chat-drawer" role="dialog" aria-modal="true" aria-label="完整聊天记录"><div class="intro"><h2>聊天记录</h2><button class="outline" @click="close" aria-label="关闭聊天记录">关闭</button></div>
 <p v-if="detailLoading">正在读取…</p><p v-if="detailError" class="error">{{detailError}} <button @click="open(selected,detailPage)">重试</button></p>
 <template v-if="detail"><h3>{{detail.conversation.title}}</h3><p class="muted">@{{detail.conversation.username}} · {{detail.conversation.name}} / {{detail.conversation.mbti}}</p><p v-if="detail.conversation.style" class="muted">角色补充：{{detail.conversation.style}}</p>
 <div v-for="t in detail.items" :key="t.id" class="chat-turn"><small>{{date(t.created_at)}} · {{status[t.status]||t.status}}</small><div class="chat-message user"><b>@{{detail.conversation.username}}</b><p>{{t.user}}</p></div><div class="chat-message"><b>{{detail.conversation.name}}</b><p>{{text(t.assistant)||'尚无回复内容'}}</p></div></div>
 <p v-if="!detail.items.length" class="muted">这段会话还没有消息。</p>
 <div class="pager"><button class="outline" :disabled="detailPage<=1" @click="open(selected,detailPage-1)">上一页</button><span>{{detailPage}} / {{Math.max(1,Math.ceil(detail.total/20))}} 页 · 共 {{detail.total}} 轮</span><button class="outline" :disabled="detailPage*20>=detail.total" @click="open(selected,detailPage+1)">下一页</button></div></template>
 </aside></div>
</section>
</template>
<style scoped>
.healing-admin{background:#fffdf8;border:1px solid #e5dfd4;border-radius:20px;padding:24px}.intro,.filters,.pager{display:flex;align-items:center;gap:12px;justify-content:space-between}.filters{margin:20px 0;justify-content:flex-start}.filters input{flex:1;min-width:0}.filters input,.filters select{padding:12px;border:1px solid #ddd4c7;border-radius:8px;background:#fff}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse}td,th{padding:14px 10px;text-align:left;border-bottom:1px solid #ece6dc}small{display:block;color:#8b7e71;margin-top:5px}.title-cell{max-width:260px;overflow-wrap:anywhere}.pager{justify-content:center;margin-top:22px}.chat-drawer{margin-left:auto;width:min(760px,100%);height:100%;overflow:auto;background:#fbf9f2;padding:28px;box-sizing:border-box}.chat-turn{margin:25px 0}.chat-message{background:#eee8f4;border-radius:12px;padding:14px;margin-top:12px}.chat-message.user{background:#e9eed6}.chat-message p{white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.7;margin:8px 0 0}.chat-message b{font-size:13px;color:#786582}@media(max-width:650px){.healing-admin{padding:14px}.filters{flex-wrap:wrap}.filters input{flex-basis:100%}.chat-drawer{padding:16px}}
</style>
