export interface SlimEntry {id:string;day:string;kind:'food'|'exercise'|'weight';name:string;quantity:number;rate:number;weight:number;calories:number}
export const activities=[{name:'平地走路（约4.5–5公里/小时）',met:3.5},{name:'快走（约5.6–6.3公里/小时）',met:4.8},{name:'慢跑（自选配速）',met:7.5},{name:'休闲骑车（低于16公里/小时）',met:4},{name:'轻度拉伸',met:2.3}];
// Rounded examples per 100 g edible portion; recipes and brands vary. Users can replace with label values.
export const foods=[{name:'熟白米饭',kcal:130},{name:'水煮鸡蛋（去壳）',kcal:155},{name:'苹果（可食部分）',kcal:52},{name:'香蕉（去皮）',kcal:89},{name:'全脂纯牛奶',kcal:61},{name:'原味干燕麦',kcal:379},{name:'熟鸡胸肉（去皮）',kcal:165},{name:'水煮土豆',kcal:87},{name:'清蒸西兰花',kcal:35},{name:'黄瓜',kcal:15},{name:'豆腐',kcal:80},{name:'无糖黑咖啡',kcal:2}];
export function bmi(height:number,weight:number){return Number.isFinite(height)&&Number.isFinite(weight)&&height>=100&&height<=250&&weight>=20&&weight<=400?weight/((height/100)**2):null;}
export function bmiLabel(value:number){return value<18.5?'体重偏低':value<24?'正常范围':value<28?'超重范围':'肥胖范围';}
export function foodCalories(grams:number,rate:number){return grams*rate/100;}
export function exerciseCalories(minutes:number,weight:number,met:number){return met*3.5*weight/200*minutes;}
export function weightDays(entries:SlimEntry[]){const days=new Map<string,SlimEntry>();for(const e of entries)if(e.kind==='weight'&&!days.has(e.day))days.set(e.day,e);return [...days.values()].sort((a,b)=>a.day.localeCompare(b.day));}
