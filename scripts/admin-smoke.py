#!/usr/bin/env python3
"""Exercise staff customer management against real API; creates only scoped test data."""
import concurrent.futures,hashlib,json,os,secrets,urllib.request,urllib.error
from pathlib import Path
base=os.environ.get('BASE_URL','http://127.0.0.1:18082').rstrip('/')
if urllib.parse.urlparse(base).hostname in ('127.0.0.1','localhost'):
 urllib.request.install_opener(urllib.request.build_opener(urllib.request.ProxyHandler({})))
ids=[];dish_id=None;admin='';password=secrets.token_hex(16)
def call(path,method='GET',data=None,t='',expected=200):
 req=urllib.request.Request(base+'/api'+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+t},method=method)
 try:
  with urllib.request.urlopen(req,timeout=20) as r:status=r.status;body=json.load(r)
 except urllib.error.HTTPError as e:status=e.code;body=json.load(e)
 assert status==expected,(path,status,body)
 return body
def no_credentials(value):
 if isinstance(value,dict):
  assert not set(value).intersection({'password','password_hash','token','token_hash','legacy_token'})
  for v in value.values():no_credentials(v)
 elif isinstance(value,list):
  for v in value:no_credentials(v)
try:
 admin=call('/admin/login','POST',{'password':os.environ['ADMIN_PASSWORD']})['token']
 accounts=[]
 for _ in range(2):
  u='adminqa_'+secrets.token_hex(5)
  a=call('/auth/register','POST',{'username':u,'password':password},expected=201);accounts.append(a);ids.append(a['id'])
 a,b=accounts
 for path in ['/admin/customers','/admin/customers/'+a['id'],'/admin/order-list','/admin/overview']:
  call(path,expected=401);call(path,t=a['token'],expected=401)
 for path in ['/admin/customers?page=-1','/admin/customers?status=unknown','/admin/order-list?page=abc','/admin/order-list?status=unknown']:
  call(path,t=admin,expected=400)
 call('/admin/customers/'+('f'*64),t=admin,expected=404)
 d=call('/admin/dishes','POST',{'name':'后台验证专用菜','description':'验证完成后清理','category':'摸鱼小食','emoji':'🧪','price':1,'available':True},admin,201);dish_id=d['id']
 def order(t):return call('/orders','POST',{'request_key':secrets.token_hex(16),'mood':'灵魂离线','note':'admin-management-smoke','items':[{'dish_id':dish_id,'quantity':1,'mood':'灵魂离线'}]},t,201)
 with concurrent.futures.ThreadPoolExecutor(4) as pool:orders=list(pool.map(lambda _:order(a['token']),range(21)))
 order(b['token'])
 for status in ['cooking','ready','completed']:call('/admin/orders/'+orders[0]['id'],'PATCH',{'status':status},admin)
 call('/admin/orders/'+orders[1]['id'],'PATCH',{'status':'cancelled'},admin)
 query='/admin/order-list?customer_id='+a['id']
 page1=call(query,t=admin);page2=call(query+'&page=2',t=admin)
 assert page1['total']==21 and len(page1['items'])==20 and len(page2['items'])==1
 assert all(o['customer_id']==a['id'] and o['username']==a['username'] for o in page1['items']+page2['items'])
 assert not set(o['id'] for o in page1['items']).intersection(o['id'] for o in page2['items'])
 assert call(query+'&status=completed',t=admin)['total']==1
 assert call(query+'&status=cancelled',t=admin)['total']==1
 assert call(query+'&status=active',t=admin)['total']==19
 assert call('/admin/order-list?q='+a['username'],t=admin)['total']==21
 found=call('/admin/order-list?q=MO-'+str(orders[0]['number']).zfill(4),t=admin)
 assert found['total']==1 and found['items'][0]['id']==orders[0]['id']
 customers=call('/admin/customers?q='+a['username'].upper(),t=admin);assert customers['total']==1
 detail=call('/admin/customers/'+a['id'],t=admin);c=detail['customer']
 assert c['balance']==280 and c['order_count']==21 and c['completed_count']==1 and c['cancelled_count']==1 and c['active_count']==19 and c['total_points']==20
 assert c['last_login_at'] and c['last_order_at'] and detail['favorite_mood']=='灵魂离线'
 assert detail['favorites'][0]['quantity']==20
 for result in [detail,customers,page1]:no_credentials(result)
 note='仅店长可见：喜欢摸鱼 <script>不是HTML</script>'
 call('/admin/customers/'+a['id'],'PATCH',{'admin_note':note},admin)
 assert call('/admin/customers/'+a['id'],t=admin)['customer']['admin_note']==note
 assert 'admin_note' not in call('/me',t=a['token'])
 call('/admin/customers/'+a['id'],'PATCH',{'admin_note':'字'*501},admin,400)
 call('/admin/customers/'+a['id'],'PATCH',{'balance':9999},admin,400)
 call('/admin/customers/'+a['id'],'PATCH',{'disabled':True},b['token'],401)
 second=call('/auth/login','POST',{'username':a['username'],'password':password})
 call('/admin/customers/'+a['id'],'PATCH',{'disabled':True},admin)
 for t in [a['token'],second['token']]:call('/me',t=t,expected=401)
 call('/auth/login','POST',{'username':a['username'],'password':password},expected=403)
 assert call('/admin/customers?q='+a['username']+'&status=disabled',t=admin)['total']==1
 assert call('/admin/customers?q='+a['username']+'&status=enabled',t=admin)['total']==0
 call('/admin/customers/'+a['id'],'PATCH',{'disabled':False},admin)
 for t in [a['token'],second['token']]:call('/orders',t=t,expected=401)
 restored=call('/auth/login','POST',{'username':a['username'],'password':password})
 assert call('/me',t=restored['token'])['balance']==280
 assert call('/me',t=b['token'])['balance']==299
 assert len(call('/orders',t=b['token']))==1
 overview=call('/admin/overview',t=admin);no_credentials(overview)
 assert len(overview['trend'])==7 and overview['today_orders']>=22 and overview['total_orders']>=22
 assert overview['pending']>=20 and overview['completed']>=1 and overview['cancelled']>=1
 call('/admin/logout','POST',{},admin);call('/admin/customers',t=admin,expected=401)
 print('PASS: customer list/detail/search; 21-order pagination; ownership and role checks; exact customer totals; favorites; private notes; status filters; disable revokes all sessions; restore requires fresh login; dashboard; no credential disclosure')
finally:
 sql=[]
 for aid in ids:
  assert len(aid)==64 and all(c in '0123456789abcdef' for c in aid)
  sql.extend([f"DELETE FROM orders WHERE guest_id='{aid}';",f"DELETE FROM accounts WHERE id='{aid}';",f"DELETE FROM guests WHERE id='{aid}';"])
 if dish_id:sql.append(f'DELETE FROM dishes WHERE id={int(dish_id)};')
 if admin:sql.append("DELETE FROM admin_sessions WHERE token_hash='%s';"%hashlib.sha256(admin.encode()).hexdigest())
 path=Path(__file__).resolve().parents[1]/'.deploy/admin-smoke-cleanup.sql';path.parent.mkdir(exist_ok=True)
 path.write_text('BEGIN;\n'+'\n'.join(sql)+'\nCOMMIT;\n')
