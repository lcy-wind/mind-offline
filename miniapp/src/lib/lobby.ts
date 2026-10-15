import {currentToken,ApiError} from './api';
import {createChatEventParser,type ChatEvent} from './healing-stream';
export interface LobbyMessage {read_count?:number;nickname?:string;mbti?:string;avatar_url?:string;is_admin?:boolean;id:number;account_id:string;username:string;request_id:string;content:string;status:'active'|'retracted'|'removed';created_at:string;updated_at:string;reactions:{emoji:string;count:number;mine:boolean}[]}
export function mergeLobby(old:LobbyMessage[],incoming:LobbyMessage[]){const map=new Map(old.map(m=>[m.id,m]));for(const m of incoming){const prev=map.get(m.id);if(prev&&prev.status!=='active'&&m.status==='active')continue;const version=(s:string)=>[Date.parse(s),Number((s.match(/\.(\d+)/)?.[1]||'').padEnd(9,'0'))];const next=version(m.updated_at),last=prev?version(prev.updated_at):[0,0];if(!prev||next[0]>last[0]||(next[0]===last[0]&&next[1]>=last[1]))map.set(m.id,m);}return [...map.values()].sort((a,b)=>a.id-b.id);}
export function retractable(m:LobbyMessage,owner:string,now:number){return m.account_id===owner&&m.status==='active'&&now-Date.parse(m.created_at)<120000&&now>=Date.parse(m.created_at);}
export async function streamLobby(active:boolean,signal:AbortSignal,accept:(event:ChatEvent)=>void){
 const token=currentToken(),controller=new AbortController();let timeout:ReturnType<typeof setTimeout>|undefined;
 const stop=()=>controller.abort();signal.addEventListener('abort',stop,{once:true});if(signal.aborted)stop();
 const reset=()=>{clearTimeout(timeout);timeout=setTimeout(stop,45000)};reset();
 try{const response=await fetch('/api/lobby/events?active='+(active?'1':'0'),{headers:{Authorization:'Bearer '+token},signal:controller.signal});
 if(token!==currentToken())throw new ApiError('账号已切换',0);
 if(!response.ok){const data=await response.json().catch(()=>({}));throw new ApiError(data.error||'鱼塘暂时没连上',response.status)}
 if(!response.body)throw new Error('当前浏览器不支持实时连接');
 const parse=createChatEventParser(e=>{if(signal.aborted||token!==currentToken())throw new ApiError('账号已切换',0);if(e.event==='expired')throw new ApiError('登录已过期',401);accept(e)});
 const reader=response.body.getReader(),decoder=new TextDecoder();try{while(true){const {value,done}=await reader.read();if(done)break;reset();parse(decoder.decode(value,{stream:true}));}}finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
 }finally{clearTimeout(timeout);signal.removeEventListener('abort',stop);controller.abort();}
}

export const emojiCategories=[
 {name:'摸鱼表情',items:['🐟','🫠','🙃','🥲','😵‍💫','🫥','😮‍💨','🤡','💀','👀','🤫','😴','🥱','🛌','🧠','💻','⌨️','📱','📄','🗑️','🧯','🪑','🏃','🚪']},
 {name:'情绪营业',items:['😀','😆','😂','🤣','😊','🥰','😍','😎','🤩','🥳','😏','😑','😒','🙄','😬','😅','😳','😨','😱','😭','🥺','😤','😡','🤯','🤔','🧐','🤨','😇','🤗','🤭','🫢','🫡']},
 {name:'手势回应',items:['👍','👎','👌','✌️','🤞','🤟','🤘','🤙','👋','👏','🙌','👐','🤝','🙏','💪','✊','👊','🤏','🫶','🫰','☝️','👈','👉','👇']},
 {name:'续命补给',items:['☕','🍵','🧋','🥛','🍺','🥤','🍚','🍜','🍲','🍣','🍙','🍔','🍟','🍕','🥪','🥟','🍗','🥚','🍞','🍰','🍩','🍦','🍫','🍎']},
 {name:'可爱杂货',items:['🐱','🐶','🐼','🐸','🐷','🐔','🦆','🦊','🐧','🐢','🦥','🐮','🐎','🌱','🌿','🍄','🌸','🌈','⭐','✨','🔥','💯','🎉','🎊','🎈','🎁','🏆','❤️','💚','💜','💔','💤']}
];

// Only count a substantial part of a bubble visible inside the current viewport.
export function visibleLobbyMessage(b:{top:number;bottom:number;height:number},root:{top:number;bottom:number;height:number}){const overlap=Math.min(b.bottom,root.bottom)-Math.max(b.top,root.top);return b.height>0&&root.height>0&&overlap>=Math.min(b.height,root.height)*.6;}
