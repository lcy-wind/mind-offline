import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {transformWithEsbuild} from 'vite';
const compiled=await transformWithEsbuild(await readFile('src/lib/work-calendar.ts','utf8'),'calendar.ts',{loader:'ts',target:'es2022'});
const {monthDays,shiftMonth,shiftDay,nextMonday,dateCaption}=await import('data:text/javascript;base64,'+Buffer.from(compiled.code).toString('base64'));
test('month grid starts Monday and includes leap day without timezone shifts',()=>{
 const days=monthDays('2028-02');assert.equal(days.length,42);assert.equal(days[0].date,'2028-01-31');assert.equal(days.find(x=>x.date==='2028-02-29').outside,false);assert.equal(days.filter(x=>!x.outside).length,29);
 assert.equal(monthDays('2026-02').filter(x=>!x.outside).length,28);
});
test('calendar shortcuts and month navigation cross year/month boundaries correctly',()=>{
 assert.equal(shiftMonth('2026-12',1),'2027-01');assert.equal(shiftMonth('2027-01',-1),'2026-12');assert.equal(shiftDay('2028-02-28',1),'2028-02-29');assert.equal(nextMonday('2026-10-12'),'2026-10-19');assert.equal(nextMonday('2026-10-11'),'2026-10-12');assert.equal(dateCaption('2026-10-12'),'10月12日 · 周一');
});
