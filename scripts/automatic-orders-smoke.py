#!/usr/bin/env python3
"""Verify real automatic fulfillment without an admin session or browser polling."""
import json,secrets,time,urllib.request,urllib.error
from pathlib import Path
base='https://mind-offline.duckdns.org'
account=None
cleanup=Path(__file__).resolve().parents[1]/'.deploy/automatic-orders-smoke-cleanup.sql'
def call(path,data=None,auth='',expected=200):
 req=urllib.request.Request(base+'/api'+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+auth})
 try:
  with urllib.request.urlopen(req,timeout=20) as r:status=r.status;out=json.load(r)
 except urllib.error.HTTPError as e:status=e.code;out=json.load(e)
 assert status==expected,(path,status,out.get('error'));return out
try:
 account=call('/auth/register',{'username':'autoqa_'+secrets.token_hex(5),'password':secrets.token_hex(16)},expected=201)
 aid=account['id'];assert len(aid)==64 and all(c in '0123456789abcdef' for c in aid)
 cleanup.write_text("BEGIN;\nDELETE FROM orders WHERE guest_id='"+aid+"';\nDELETE FROM accounts WHERE id='"+aid+"' AND username LIKE 'autoqa_%';\nDELETE FROM guests WHERE id='"+aid+"' AND NOT EXISTS(SELECT 1 FROM accounts WHERE id='"+aid+"');\nCOMMIT;\n")
 auth=account['token'];menu=call('/menu');dish=next(d for d in menu if d['available'] and d['price']<=100)
 payload={'request_key':secrets.token_hex(16),'mood':'灵魂离线','note':'automatic-order-smoke','items':[{'dish_id':dish['id'],'quantity':1,'mood':'灵魂离线'}]}
 order=call('/orders',payload,auth,201);start=time.monotonic()
 assert order['status']=='pending' and order['auto_started_at'] and order['step_seconds']==30
 duplicate=call('/orders',payload,auth);assert duplicate['id']==order['id'] and duplicate['auto_started_at']==order['auto_started_at']
 print('0s: pending; schedule persisted; duplicate submission preserved same schedule',flush=True)
 for wait,want in [(31,'cooking'),(61,'ready'),(91,'completed')]:
  while time.monotonic()-start<wait:time.sleep(min(1,max(0,wait-(time.monotonic()-start))))
  current=next(o for o in call('/orders',auth=auth) if o['id']==order['id'])
  assert current['status']==want,(want,current['status'])
  print(str(round(time.monotonic()-start,1))+'s: '+want+' (no polling during preceding wait)',flush=True)
 assert call('/me',auth=auth)['balance']==300-dish['price']
 print('PASS: automatic 30/60/90-second fulfillment, persistence, idempotency and unchanged balance',flush=True)
finally:
 if account:
  try:call('/auth/logout',{},account['token'])
  except Exception:pass
