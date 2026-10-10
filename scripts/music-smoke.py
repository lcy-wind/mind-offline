#!/usr/bin/env python3
"""Real QR smoke test without scanning or using any personal music account."""
import json,os,secrets,urllib.request,urllib.error
from pathlib import Path
base=os.environ.get('BASE_URL','https://mind-offline.duckdns.org').rstrip('/')
provider=os.environ.get('MUSIC_PROVIDER','netease')
assert provider in ('netease','kugou')
music='/music/'+provider
ids=[];password=secrets.token_hex(16)
def call(path,data=None,t='',status=200):
 req=urllib.request.Request(base+'/api'+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+t})
 try:
  with urllib.request.urlopen(req,timeout=20) as r:actual=r.status;out=json.load(r)
 except urllib.error.HTTPError as e:actual=e.code;out=json.load(e)
 assert actual==status,(path,actual,out.get('error'))
 return out
try:
 call(music,status=401)
 a=call('/auth/register',{'username':'musicqa_'+secrets.token_hex(5),'password':password},status=201);ids.append(a['id'])
 b=call('/auth/register',{'username':'musicqa_'+secrets.token_hex(5),'password':password},status=201);ids.append(b['id'])
 second=call('/auth/login',{'username':a['username'],'password':password})
 state=call(music,t=a['token']);assert state['enabled'] and not state['bound']
 qr=call(music+'/qr',{},a['token'],201);assert qr['image'].startswith('data:image/png;base64,') and len(qr['attempt_id'])==64
 assert not any(k in qr for k in ['cookie','key','secret','token'])
 call(music+'/qr/check',{'attempt_id':qr['attempt_id']},b['token'],410)
 call(music+'/qr/check',{'attempt_id':qr['attempt_id']},second['token'],410)
 result=call(music+'/qr/check',{'attempt_id':qr['attempt_id']},a['token']);assert result['status']=='waiting'
 call(music+'/qr/cancel',{'attempt_id':qr['attempt_id']},a['token'])
 call(music+'/qr/check',{'attempt_id':qr['attempt_id']},a['token'],410)
 call(music+'/playlists',t=a['token'],status=409)
 call(music+'/unbind',{},a['token']);assert not call(music,t=a['token'])['bound']
 call('/auth/logout',{},a['token']);call('/auth/logout',{},b['token']);call('/auth/logout',{},second['token'])
 print(provider+': PASS: real HTTPS QR generation and waiting status; account/session isolation; cancellation; no Cookie exposed. No personal music account was scanned.')
finally:
 sql=[]
 for aid in ids:
  assert len(aid)==64 and all(c in '0123456789abcdef' for c in aid)
  sql += [f"DELETE FROM accounts WHERE id='{aid}';",f"DELETE FROM guests WHERE id='{aid}';"]
 target=Path(__file__).resolve().parents[1]/'.deploy/music-smoke-cleanup.sql';target.parent.mkdir(exist_ok=True)
 target.write_text('BEGIN;\n'+'\n'.join(sql)+'\nCOMMIT;\n')
