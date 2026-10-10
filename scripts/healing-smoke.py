#!/usr/bin/env python3
"""Real chat smoke test with disposable accounts and non-private sample messages."""
import json, secrets, time, urllib.request, urllib.error
from pathlib import Path
base='https://mind-offline.duckdns.org'
ids=[];sessions=[]
cleanup=Path(__file__).resolve().parents[1]/'.deploy/healing-smoke-cleanup.sql'
def journal():
 sql=[]
 for aid in ids:
  assert len(aid)==64 and all(c in '0123456789abcdef' for c in aid)
  sql += [f"DELETE FROM accounts WHERE id='{aid}' AND username LIKE 'healingqa_%';",f"DELETE FROM guests WHERE id='{aid}' AND NOT EXISTS(SELECT 1 FROM accounts WHERE id='{aid}');"]
 cleanup.write_text('BEGIN;\n'+'\n'.join(sql)+'\nCOMMIT;\n')
def call(path,data=None,auth='',status=200):
 req=urllib.request.Request(base+'/api'+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+auth})
 try:
  with urllib.request.urlopen(req,timeout=20) as response:actual=response.status;out=json.load(response)
 except urllib.error.HTTPError as e:actual=e.code;out=json.load(e)
 assert actual==status,(path,actual,out.get('error'));return out

def stream(path,auth,rid,text):
 req=urllib.request.Request(base+'/api'+path,data=json.dumps({'request_id':rid,'text':text}).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+auth})
 start=time.monotonic();first=None;answer=None;event='';parts=[]
 with urllib.request.urlopen(req,timeout=100) as response:
  assert response.status==200
  for raw in response:
   line=raw.decode().strip()
   if line.startswith('event:'):event=line[6:].strip()
   elif line.startswith('data:'):
    data=json.loads(line[5:])
    if event=='delta':
     if first is None:first=time.monotonic()-start
     parts.append(data['text'])
    elif event=='done':answer=data
    elif event=='error':raise AssertionError(data.get('message'))
 assert answer and answer['status']=='complete'
 print(json.dumps({'first_delta_seconds':round(first,2) if first else None,'total_seconds':round(time.monotonic()-start,2),'reply':answer['assistant']},ensure_ascii=False),flush=True)
 return answer
try:
 call('/healing/config',status=401)
 for _ in range(2):
  a=call('/auth/register',{'username':'healingqa_'+secrets.token_hex(5),'password':secrets.token_hex(16)},status=201)
  ids.append(a['id']);sessions.append(a['token']);journal()
 config=call('/healing/config',auth=sessions[0]);assert config['enabled'] and len(config['roles'])==16
 conv=call('/healing/conversations',{'mbti':'INFP','name':'小树','style':'回答简短自然。'},sessions[0],201)
 path='/healing/conversations/'+conv['id']
 call(path,auth=sessions[1],status=404)
 call(path+'/messages',{'request_id':'cross-account-test','text':'hello'},sessions[1],404)
 first=stream(path+'/messages',sessions[0],'smoke-first-'+secrets.token_hex(4),'我叫小林，今天想轻松聊一会儿，请简单打个招呼。')
 second=stream(path+'/messages',sessions[0],'smoke-second-'+secrets.token_hex(4),'我刚才说自己叫什么名字？只用一句话回答。')
 assert '小林' in second['assistant'],'context recall sample failed'
 detail=call(path,auth=sessions[0]);assert len(detail['turns'])==2 and all(t['status']=='complete' for t in detail['turns'])
 call(path+'/delete',{},sessions[0]);call(path,auth=sessions[0],status=404)
 print('PASS: real model streaming, role conversation, context recall, persistence, account isolation and deletion',flush=True)
finally:
 for session in sessions:
  try:call('/auth/logout',{},session)
  except Exception:pass
 journal()
