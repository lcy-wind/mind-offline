<script setup lang="ts">
import {ref,onMounted,onUnmounted} from 'vue';
import BeadThumbnail from '../../miniapp/src/components/BeadThumbnail.vue';
import BeadPreview from '../../miniapp/src/components/BeadPreview.vue';
import {beadHeight,beadProgress,paletteLabel,type BeadWork} from '../../miniapp/src/lib/beads';
type Work=BeadWork&{account_id:string;username:string;nickname:string};
const props=defineProps<{api:(path:string)=>Promise<any>}>();
const items=ref<Work[]>([]),total=ref(0),page=ref(1),search=ref(''),status=ref('all'),owner=ref<{id:string;name:string}|null>(null),busy=ref(false),error=ref(''),preview=ref<Work|null>(null);
let active=true,sequence=0;
const date=(s?:string)=>s?new Date(s).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false}):'暂无记录';
async function load(reset=false){if(reset)page.value=1;const n=++sequence;busy.value=true;error.value='';preview.value=null;try{const result=await props.api('/admin/beads?'+new URLSearchParams({q:search.value.trim(),status:status.value,account_id:owner.value?.id||'',page:String(page.value)}));if(active&&n===sequence){items.value=result.items;total.value=result.total;}}catch(e){if(active&&n===sequence)error.value=(e as Error).message;}finally{if(active&&n===sequence)busy.value=false;}}
function byAuthor(w:Work){owner.value={id:w.account_id,name:w.nickname||w.username};search.value='';void load(true);}
onMounted(()=>load());onUnmounted(()=>{active=false;sequence++;});
</script>
<template>
<section class="beads-admin">
 <div class="gallery-heading"><div><h2>拼豆作品集</h2><p class="muted">全员摸鱼艺术展 · 按最近保存排列 · 包含未分享到大厅的作品</p></div><button class="outline" :disabled="busy" @click="load()">刷新</button></div>
 <form class="gallery-filters" @submit.prevent="load(true)"><input v-model="search" maxlength="80" placeholder="搜索作品名、昵称或账号" aria-label="搜索拼豆作品"/><select v-model="status" aria-label="作品状态"><option value="all">全部状态</option><option value="ongoing">创作中</option><option value="finished">已熨好</option></select><button class="primary" :disabled="busy">搜索</button></form>
 <p v-if="owner" class="author-filter">正在看 {{owner.name}} 的作品 <button class="text-button" @click="owner=null;load(true)">查看所有人 ×</button></p>
 <p v-if="error" class="error" role="alert">{{error}}</p>
 <p class="muted">共 {{total}} 件作品 · 点击图案预览，放大后可拖动查看</p>
 <div class="gallery-grid" :aria-busy="busy"><article v-for="w in items" :key="w.account_id+':'+w.id" class="gallery-card"><button class="art-preview" @click="preview=w" :aria-label="'预览 '+w.data.name"><BeadThumbnail :pixels="w.data.pixels" :size="w.data.size" :height="beadHeight(w.data)" :palette="w.data.palette" :round="false"/><span>放大欣赏 ↗</span></button><h3>{{w.data.name}}</h3><button class="author" @click="byAuthor(w)">{{w.nickname||w.username}} <small>@{{w.username}}</small></button><p>{{w.data.size}} × {{beadHeight(w.data)}} · 已拼 {{beadProgress(w.data).filled}} 颗</p><p>{{paletteLabel(w.data.palette)}} · {{w.data.finished?'已熨好':'创作中'}}</p><time :datetime="w.updated_at">更新于 {{date(w.updated_at)}}</time></article></div>
 <p v-if="!items.length" class="gallery-empty">{{busy?'正在收集大家的摸鱼成果…':'暂时没有符合条件的作品'}}</p>
 <div class="gallery-pager"><button class="outline" :disabled="busy||page<=1" @click="page--;load()">上一页</button><span>第 {{page}} / {{Math.max(1,Math.ceil(total/12))}} 页</span><button class="outline" :disabled="busy||page*12>=total" @click="page++;load()">下一页</button></div>
 <BeadPreview v-if="preview" :data="preview.data" @close="preview=null"/>
</section>
</template>
<style scoped>
.beads-admin{padding:24px;background:#fffdf8;border:1px solid #e5dfd4;border-radius:20px}.gallery-heading{display:flex;justify-content:space-between;align-items:center;gap:15px}.gallery-heading h2{margin:0}.gallery-filters{display:flex;flex-wrap:wrap;gap:10px;margin:22px 0}.gallery-filters input{flex:1;min-width:160px;width:auto}.gallery-filters select{width:auto}.gallery-filters input,.gallery-filters select{padding:12px;border:1px solid #ddd4c7;background:#fff;border-radius:8px}.gallery-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.gallery-card{min-width:0;padding:14px;border:1px solid #e5e6d9;border-radius:15px;background:#fafbf5}.art-preview{display:block;width:100%;padding:9px;background:#fff;border:1px solid #e5e7dc;border-radius:10px;cursor:zoom-in}.art-preview :deep(.bead-thumbnail){height:180px;max-height:180px;object-fit:contain;border-radius:4px}.art-preview span{display:block;margin-top:8px;font-size:10px;color:#9aa989}.gallery-card h3{font-size:14px;overflow-wrap:anywhere;margin:14px 0 9px}.author{background:transparent;border:0;padding:0;text-align:left;color:#7b8d64;font-size:12px;cursor:pointer;overflow-wrap:anywhere}.author small{color:#a3a895;font-size:10px}.gallery-card p,.gallery-card time{display:block;font-size:11px;line-height:1.6;color:#999e8b;margin:6px 0;overflow-wrap:anywhere}.gallery-pager{display:flex;align-items:center;justify-content:center;gap:12px;margin-top:24px;font-size:12px}.gallery-empty{padding:35px;text-align:center;color:#a1aa94}.author-filter{font-size:12px}.art-preview:focus-visible,.author:focus-visible{outline:2px solid #91a779;outline-offset:3px}@media(max-width:1000px){.gallery-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:600px){.beads-admin{padding:14px}.gallery-grid{gap:10px}.gallery-card{padding:10px}.art-preview :deep(.bead-thumbnail){height:130px;max-height:130px}.gallery-heading{align-items:flex-start}.gallery-filters input{flex-basis:100%}.gallery-card h3{font-size:12px}}
</style>
