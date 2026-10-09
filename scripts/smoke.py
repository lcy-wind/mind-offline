#!/usr/bin/env python3
"""Exercise real API transactions. Requires ADMIN_PASSWORD; writes scoped cleanup SQL."""
import concurrent.futures, hashlib, json, os, secrets, urllib.request, urllib.error
from pathlib import Path
base=os.environ.get('BASE_URL','http://127.0.0.1:18082').rstrip('/')
if urllib.parse.urlparse(base).hostname in ('127.0.0.1', 'localhost'):
    urllib.request.install_opener(urllib.request.build_opener(urllib.request.ProxyHandler({})))
password=os.environ['ADMIN_PASSWORD']
tokens=[];dish_id=None;admin_token=None

def call(path,method='GET',data=None,token='',status=200):
    req=urllib.request.Request(base+'/api'+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+token},method=method)
    try:
        with urllib.request.urlopen(req,timeout=20) as r: actual=r.status; payload=json.load(r)
    except urllib.error.HTTPError as e:
        actual=e.code;payload=json.load(e)
    assert actual in (status if isinstance(status, tuple) else (status,)),(path,actual,payload)
    return payload

def order_payload(did,qty=2,key=None):
    return {'request_key':key or secrets.token_hex(16),'mood':'灵魂离线','note':'automated-smoke-test','items':[{'dish_id':did,'quantity':qty,'mood':'灵魂离线'}]}

try:
    call('/health')
    call('/admin/orders',status=401)
    call('/orders',status=401)
    menu=call('/menu');assert len(menu)>=9
    admin_token=call('/admin/login','POST',{'password':password})['token']
    dish={'name':'自动验证专用菜','description':'测试后清理','category':'摸鱼小食','emoji':'🧪','price':28,'available':True}
    created=call('/admin/dishes','POST',dish,admin_token,201);dish_id=created['id']
    for _ in range(2):tokens.append(call('/guest','POST',{},status=201)['token'])
    t,other=tokens
    assert call('/me',token=t)['balance']==300
    with concurrent.futures.ThreadPoolExecutor(3) as pool:
        list(pool.map(lambda _:call('/claim','POST',{},t),range(3)))
    assert call('/me',token=t)['balance']==400
    data=order_payload(dish_id)
    with concurrent.futures.ThreadPoolExecutor(4) as pool:
        results=list(pool.map(lambda _:call('/orders','POST',data,t,status=(200,201)),range(4)))
    first=results[0]
    assert all(o['id']==first['id'] for o in results)
    assert call('/me',token=t)['balance']==344
    assert len(call('/orders',token=t))==1
    assert call('/orders',token=other)==[]
    bad=order_payload(dish_id,-1);call('/orders','POST',bad,t,400)
    forged=order_payload(dish_id);forged['items'][0]['price']=1;call('/orders','POST',forged,t,400)
    high=order_payload(dish_id,10);high['items']*=2;call('/orders','POST',high,t,409)
    call('/admin/orders/'+first['id'],'PATCH',{'status':'completed'},admin_token,409)
    call('/admin/orders/'+first['id'],'PATCH',{'status':'cooking'},admin_token)
    call('/admin/orders/'+first['id'],'PATCH',{'status':'cancelled'},admin_token)
    assert call('/me',token=t)['balance']==400
    call('/admin/orders/'+first['id'],'PATCH',{'status':'cancelled'},admin_token,409)
    assert call('/me',token=t)['balance']==400
    with concurrent.futures.ThreadPoolExecutor(3) as pool:
        spending=list(pool.map(lambda _:call('/orders','POST',order_payload(dish_id,10),t,status=(201,409)),range(3)))
    successful=[o for o in spending if 'id' in o]
    assert len(successful)==1
    assert call('/me',token=t)['balance']==120
    call('/admin/orders/'+successful[0]['id'],'PATCH',{'status':'cancelled'},admin_token)
    assert call('/me',token=t)['balance']==400
    second=call('/orders','POST',order_payload(dish_id,1),t,201)
    for status in ['cooking','ready','completed']:
        call('/admin/orders/'+second['id'],'PATCH',{'status':status},admin_token)
    dish['available']=False
    call('/admin/dishes/'+str(dish_id),'PUT',dish,admin_token)
    call('/orders','POST',order_payload(dish_id),t,409)
    assert call('/me',token=t)['balance']==372
    dish['price']=39;call('/admin/dishes/'+str(dish_id),'PUT',dish,admin_token)
    assert next(o for o in call('/orders',token=t) if o['id']==second['id'])['items'][0]['price']==28
    call('/admin/summary',token=admin_token)
    call('/admin/logout','POST',{},admin_token)
    call('/admin/orders',token=admin_token,status=401)
    print('PASS: authentication, guest isolation, daily grant, duplicate-order protection, server-side prices, balance, refund-once, lifecycle, sold-out, historical price snapshot, logout')
finally:
    sql=[]
    for t in tokens:
        h=hashlib.sha256(t.encode()).hexdigest()
        sql += [f"DELETE FROM orders WHERE guest_id IN (SELECT id FROM guests WHERE token_hash='{h}');",f"DELETE FROM guests WHERE token_hash='{h}';"]
    if dish_id is not None:sql.append(f'DELETE FROM dishes WHERE id={int(dish_id)};')
    if admin_token:sql.append("DELETE FROM admin_sessions WHERE token_hash='%s';"%hashlib.sha256(admin_token.encode()).hexdigest())
    path=Path(__file__).resolve().parents[1]/'.deploy/smoke-cleanup.sql';path.parent.mkdir(exist_ok=True)
    path.write_text('BEGIN;\n'+'\n'.join(sql)+'\nCOMMIT;\n')
