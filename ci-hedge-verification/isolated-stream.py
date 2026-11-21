"""Exact-image relay smoke using a local, scripted Claude SSE upstream only."""
import json, time, threading, uuid, requests, subprocess, secrets
from pathlib import Path
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
R=Path(__file__).parent
read=lambda name: json.loads((R/name).read_text(encoding='utf8'))
B=read('build-image-evidence.json'); V=read('verified-local-image.json')
assert (V['revision'],V['digest'])==(B['sha'],B['digest'])
def docker(*args): return subprocess.check_output(['docker',*args],text=True,timeout=45).strip()
assert docker('image','inspect',V['image'],'--format','{{.Id}}')==V['id']
import sys
chat='chat' in sys.argv
name='parallel-sse-'+uuid.uuid4().hex[:9]; model='isolated-model-alias'; arrivals={}; lock=threading.Lock(); same_started=threading.Event(); trial=0
class Upstream(BaseHTTPRequestHandler):
    def log_message(self,*args): pass
    def do_POST(self):
        global trial
        payload=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        assert payload['model']==model and payload['stream']
        with lock:
            endpoint=self.path.split('/')[1]; seq=len(arrivals)
            lane='alternate' if endpoint=='b' else ('same' if 'original' in arrivals else 'original')
            assert lane not in arrivals
            arrivals[lane]=time.monotonic()
            if lane=='same': same_started.set()
            scenario=trial
        self.send_response(200); self.send_header('Content-Type','text/event-stream'); self.end_headers()
        def frame(data):
            self.wfile.write(('data: '+(data if isinstance(data,str) else json.dumps(data))+'\n\n').encode()); self.wfile.flush()
        def chunk(choices, usage=None):
            frame(dict(id=lane+'-chunk', model=model, object='chat.completion.chunk', choices=choices, **({'usage':usage} if usage else {})))
        try:
            if chat:
                chunk([{'index':0,'delta':{'role':'assistant','content':''}}])
                chunk([], {'prompt_tokens':10,'completion_tokens':0,'total_tokens':10})
            else:
                frame({'type':'message_start','message':{'id':lane+'-message','model':model,'usage':{'input_tokens':10,'output_tokens':1}}})
                frame({'type':'content_block_start','index':0,'content_block':{'type':'text','text':''}})
                frame({'type':'ping'})
            if lane==('alternate' if scenario else 'same'):
                assert same_started.wait(8)
                time.sleep(.08)
                if chat:
                    chunk([{'index':0,'delta':{'content':lane+'-visible'}}])
                    chunk([{'index':0,'delta':{},'finish_reason':'stop'}], {'prompt_tokens':10,'completion_tokens':3,'total_tokens':13})
                    frame('[DONE]')
                else:
                    frame({'type':'content_block_delta','index':0,'delta':{'type':'text_delta','text':lane+'-visible'}})
                    frame({'type':'content_block_stop','index':0})
                    frame({'type':'message_delta','delta':{'stop_reason':'end_turn'},'usage':{'output_tokens':3}})
                    frame({'type':'message_stop'})
            else:
                end=time.monotonic()+23
                while time.monotonic()<end:
                    time.sleep(.2)
                    if chat: chunk([{'index':0,'delta':{'content':''}}])
                    else: frame({'type':'ping'})
        except (BrokenPipeError,ConnectionResetError): pass
server=ThreadingHTTPServer(('0.0.0.0',0),Upstream); threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    docker('run','--pull=never','-d','--name',name,'--add-host=host.docker.internal:host-gateway','-e','STREAMING_TIMEOUT=60','-p','127.0.0.1::3000',V['image'])
    port=int(docker('port',name,'3000/tcp').rsplit(':',1)[1]); base=f'http://127.0.0.1:{port}'
    def request(method,path,**kwargs):
        r=session.request(method,base+path,timeout=8,**kwargs);r.raise_for_status();d=r.json()
        assert d.get('success'), (path,d.get('message','API failed')); return d.get('data')
    session=requests.Session()
    for _ in range(40):
        try: status=request('GET','/api/status'); break
        except (requests.RequestException,ValueError): time.sleep(1)
    else: raise RuntimeError('isolated app not ready')
    assert status['version']==B['tag']
    setup=request('GET','/api/setup')
    if setup['status']:
        username,password='root','123456'
    else:
        username,password='ssesmoke',secrets.token_hex(12)
        request('POST','/api/setup',json={'username':username,'password':password,'confirmPassword':password,'SelfUseModeEnabled':False,'DemoSiteEnabled':False})
    info=request('POST','/api/user/login',json={'username':username,'password':password})
    assert info['role']==100; session.headers['New-Api-User']=str(info['id'])
    def option(k,v): request('PUT','/api/option/',json={'key':k,'value':v})
    option('RetryTimes','2'); option('ModelPrice',json.dumps({model:0.001})); option('QuotaPerUnit','500000')
    # Exercise the native Chat adapter, not the default Chat-to-Responses bridge.
    if chat:
        option('global.auto_chat_to_responses_enabled','false')
    for ch,priority in [('a',10),('b',0)]:
        request('POST','/api/channel/',json={'mode':'single','channel':{'type':1 if chat else 14,'key':'local-fixture-'+ch,'name':'fixture-'+ch,'base_url':f'http://host.docker.internal:{server.server_port}/{ch}','models':model,'group':'default','priority':priority,'weight':1,'status':1,'auto_ban':0}})
    request('POST','/api/token/',json={'name':'fixture','group':'default','remain_quota':100000,'expired_time':-1,'first_byte_timeout':55,'upstream_timeout':60})
    tokens=request('GET','/api/token/')['items']; token=next(t for t in tokens if t['name']=='fixture'); key=request('POST',f"/api/token/{token['id']}/key")['key']
    modes=[]
    option('ChannelRetryPolicy',json.dumps([{'stream_hedge':{'delay_ms':250,'retry_same_channel_first':True}}]))
    for is_parallel in ([True] if chat else [False,True]):
        with lock: trial=int(is_parallel); arrivals.clear(); same_started.clear()
        if is_parallel:
            option('ChannelRetryPolicy',json.dumps([{'stream_hedge':{'delay_ms':15000,'retry_same_channel_first':True,'parallel_backup_scope':'all' if chat else 'claude'}}]))
        before=request('GET','/api/user/self')['quota']
        resp=requests.post(base+('/v1/chat/completions' if chat else '/v1/messages'),headers={'Authorization':'Bearer '+key},json={'model':model,'stream':True,'max_tokens':24,'messages':[{'role':'user','content':'isolated smoke'}]},timeout=(5,25))
        assert resp.status_code==200, (resp.status_code,resp.text[:350])
        expected='alternate' if is_parallel else 'same'; assert resp.text.count(expected+'-visible')==1, resp.text[:600]
        assert 'original-visible' not in resp.text and ('same-visible' if is_parallel else 'alternate-visible') not in resp.text
        with lock: times={k:round((v-arrivals['original'])*1000,2) for k,v in arrivals.items()}
        assert set(times)==({'original','same','alternate'} if is_parallel else {'original','same'}),times
        if is_parallel:
            assert all(14900<=times[k]<17000 for k in ('same','alternate')) and abs(times['same']-times['alternate'])<1000,times
        after=request('GET','/api/user/self')['quota']; assert before-after==500,(before,after)
        modes.append({'parallel':is_parallel,'arrivals_ms':times,'winner':expected,'platform_charge_quota':before-after})
    logs=request('GET','/api/log/?type=2&model_name='+model)['items']; assert len(logs)==len(modes) and all(row['quota']==500 for row in logs)
    assert request('GET','/api/status')['start_time']==status['start_time']
    out={'sha':B['sha'],'digest':B['digest'],'tag':B['tag'],'cases':modes,'one_charge_per_request':True,'real_upstream_calls':0}
    with (R/('isolated-chat.json' if chat else 'isolated-stream.json')).open('x',encoding='utf8') as f: json.dump(out,f,indent=2)
    print(json.dumps(out),flush=True)
finally:
    server.shutdown()
    subprocess.run(['docker','rm','-fv',name],capture_output=True,timeout=30)
