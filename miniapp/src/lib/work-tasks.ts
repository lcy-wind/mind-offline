export type WorkStatus = 'todo' | 'doing' | 'done';
export interface WorkTask {id:string;title:string;planned_at:string|null;urgency:number;status:WorkStatus;created_at:string;updated_at:string}
export const workStatuses: {id:WorkStatus;label:string}[] = [{id:'todo',label:'待开始'},{id:'doing',label:'进行中'},{id:'done',label:'已完成'}];
export const workUrgencies = [{id:1,label:'不急'},{id:2,label:'普通'},{id:3,label:'紧急'}];
export function workDateParts(iso: string) {
 const date = new Date(iso);
 if(!Number.isFinite(date.getTime())) return {day:'',time:''};
 const china = new Date(date.getTime()+8*3600000).toISOString();
 return {day:china.slice(0,10),time:china.slice(11,16)};
}
export function workPlannedAt(day:string,time:string):string|null { return day ? day+'T'+(time||'09:00')+':00+08:00' : null; }
export function sortWorkTasks(tasks: WorkTask[]): WorkTask[] {
 return [...tasks].sort((a,b)=>Number(a.status==='done')-Number(b.status==='done') || b.urgency-a.urgency || (a.planned_at?Date.parse(a.planned_at):Infinity)-(b.planned_at?Date.parse(b.planned_at):Infinity) || Date.parse(b.created_at)-Date.parse(a.created_at) || a.id.localeCompare(b.id));
}
