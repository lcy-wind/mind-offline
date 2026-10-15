export interface SlimEntry {id:string;day:string;kind:'food'|'exercise'|'weight';name:string;quantity:number;rate:number;weight:number;calories:number}
export const activities=[{name:'平地走路（约4.5–5公里/小时）',met:3.5},{name:'快走（约5.6–6.3公里/小时）',met:4.8},{name:'慢跑（自选配速）',met:7.5},{name:'休闲骑车（低于16公里/小时）',met:4},{name:'轻度拉伸',met:2.3}];
// Rounded examples per 100 g edible portion; recipes and brands vary. Users can replace with label values.
export const foods=[{name:'熟白米饭',kcal:130},{name:'水煮鸡蛋（去壳）',kcal:155},{name:'苹果（可食部分）',kcal:52},{name:'香蕉（去皮）',kcal:89},{name:'全脂纯牛奶',kcal:61},{name:'原味干燕麦',kcal:379},{name:'熟鸡胸肉（去皮）',kcal:165},{name:'水煮土豆',kcal:87},{name:'清蒸西兰花',kcal:35},{name:'黄瓜',kcal:15},{name:'豆腐',kcal:80},{name:'无糖黑咖啡',kcal:2}];
export function bmi(height:number,weight:number){return Number.isFinite(height)&&Number.isFinite(weight)&&height>=100&&height<=250&&weight>=20&&weight<=400?weight/((height/100)**2):null;}
export function bmiLabel(value:number){return value<18.5?'体重偏低':value<24?'正常范围':value<28?'超重范围':'肥胖范围';}
export function foodCalories(grams:number,rate:number){return grams*rate/100;}
export function exerciseCalories(minutes:number,weight:number,met:number){return met*3.5*weight/200*minutes;}
export function weightDays(entries:SlimEntry[]){const days=new Map<string,SlimEntry>();for(const e of entries)if(e.kind==='weight'&&!days.has(e.day))days.set(e.day,e);return [...days.values()].sort((a,b)=>a.day.localeCompare(b.day));}

const foodRemarks=['胃部已签收，老板的饼请走废品回收。','今日续航已补货，灵魂暂不接受加班。','这口算你的，工作群不许分走。','饭已入账，烦恼暂时挂起。'];
const exerciseRemarks=['工位失去一位人质，身体喜提一次放风。','腿：终于想起来我不是椅子的配件了。','今日移动完成，老板的需求没有跟上来。','身体刚刚上线，工作消息继续免打扰。'];
const weightRemarks=['只是记个数，秤没有人事任免权。','身体观察已归档，不开绩效会。','今天的数值收到了，长期趋势慢慢看。'];
export function entryRemark(e:SlimEntry){const pool=e.kind==='food'?foodRemarks:e.kind==='exercise'?exerciseRemarks:weightRemarks;let n=0;for(const c of e.id)n=(n+c.charCodeAt(0))%997;return pool[n%pool.length];}
export function dayReport(entries:SlimEntry[]){
 const food=entries.filter(e=>e.kind==='food'),exercise=entries.filter(e=>e.kind==='exercise');
 const stamps=Number(food.length>0)+Number(exercise.length>0);
 return {food:food.length,exercise:exercise.length,stamps,
 title:stamps===2?'吃动搭子，今日合体':food.length?'胃部已上线':'腿部已上线',
 detail:stamps===2?'饭有记录，活动也有回音。今天的身体小剧场，素材齐了。':food.length?'投喂记录已收好。身体不是永动机，吃饭这事很正经。':exercise.length?'身体已经出门营业，活动记录收到了。没记饭不代表没吃饭。':'研究所刚开门，今天的故事由你投喂。',
 };
}
