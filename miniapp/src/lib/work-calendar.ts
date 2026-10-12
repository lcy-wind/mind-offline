// Calendar arithmetic uses UTC date-only values, independent of browser timezone.
export function dateOnly(value:Date):string{return value.toISOString().slice(0,10);}
export function shiftDay(day:string,amount:number):string{const d=new Date(day+'T12:00:00Z');d.setUTCDate(d.getUTCDate()+amount);return dateOnly(d);}
export function shiftMonth(month:string,amount:number):string{const d=new Date(month+'-01T12:00:00Z');d.setUTCMonth(d.getUTCMonth()+amount);return dateOnly(d).slice(0,7);}
export function nextMonday(day:string):string{const d=new Date(day+'T12:00:00Z');return shiftDay(day,(8-d.getUTCDay())%7||7);}
export function monthDays(month:string):{date:string;day:number;outside:boolean}[]{
 const first=month+'-01';const weekday=(new Date(first+'T12:00:00Z').getUTCDay()+6)%7;
 return Array.from({length:42},(_,i)=>{const date=shiftDay(first,i-weekday);return {date,day:Number(date.slice(8)),outside:date.slice(0,7)!==month};});
}
export function dateCaption(day:string):string{
 if(!day)return '';
 const d=new Date(day+'T12:00:00Z');if(!Number.isFinite(d.getTime()))return day;
 return Number(day.slice(5,7))+'月'+Number(day.slice(8))+'日 · 周'+['日','一','二','三','四','五','六'][d.getUTCDay()];
}
