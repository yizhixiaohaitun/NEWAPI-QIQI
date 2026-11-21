"""Smoke the released binary inside a private Docker network with no actual model."""
import http.cookiejar
import json
from pathlib import Path
import secrets
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib import error, request

HERE = Path(__file__).resolve().parent
proof = json.loads((HERE / 'isolated-image.json').read_text('utf-8'))
sample = json.loads((HERE / 'responses-test.json').read_text('utf-8'))
assert sample['stream'] is True and sample['store'] is False
model = sample['model']
base = 'http://gateway:3000'
lock = threading.Lock()
case = ''
calls = []

class Fixture(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        global calls
        lane, path = self.path[1:].split('/', 1)
        assert lane in ('a', 'b') and path == 'v1/responses', self.path
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        assert all(body.get(k) == v for k, v in sample.items()), list(body)
        with lock:
            scenario = case
            calls.append(lane)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Connection', 'close')
        self.end_headers()
        def frame(obj, name=None):
            head = f'event: {name}\n' if name else ''
            self.wfile.write((head + 'data: ' + json.dumps(obj, separators=(',', ':')) + '\n\n').encode())
            self.wfile.flush()
        try:
            frame({'type':'response.created','response':{'id':'smoke-'+lane,'status':'in_progress'}})
            if lane == 'a':
                if scenario == 'before':
                    frame({'code':429}, 'error')
                else:
                    frame({'type':'response.reasoning_summary_text.delta','delta':'delivered reasoning'})
                    frame({'type':'response.function_call_arguments.done','item_id':'smoke-call','arguments':'{}'})
                    if scenario == 'after':
                        frame({'type':'response.output_text.delta','delta':'billable fixture output'})
                    frame({'code':9007199254740993}, 'response.error')
                # Intentionally leave the upstream connection open beyond the client's timeout.
                threading.Event().wait(8)
            else:
                frame({'type':'response.output_text.delta','delta':'OK-from-fixture-B'})
                frame({'type':'response.completed','response':{'id':'smoke-b','status':'completed','usage':{'input_tokens':5,'output_tokens':1,'total_tokens':6}}})
        except (BrokenPipeError, ConnectionResetError):
            pass

server = ThreadingHTTPServer(('0.0.0.0', 18080), Fixture)
threading.Thread(target=server.serve_forever, daemon=True).start()
admin = request.build_opener(request.ProxyHandler({}), request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
relay = request.build_opener(request.ProxyHandler({}))

def api(verb, path, data=None):
    encoded = None if data is None else json.dumps(data).encode()
    req = request.Request(base + path, method=verb, data=encoded, headers={'Content-Type':'application/json'})
    with admin.open(req, timeout=6) as res:
        val = json.load(res)
    assert val.get('success'), (path, val.get('message'))
    return val.get('data')

for _ in range(50):
    try:
        status = api('GET','/api/status')
        break
    except (error.URLError, ValueError):
        time.sleep(1)
else:
    raise RuntimeError('isolated gateway did not become healthy')
assert status['version'] == proof['tag']
if api('GET','/api/setup')['status']:
    username, password = 'root', '123456'  # ephemeral database's built-in default
else:
    username, password = 'rsp-smoke', secrets.token_hex(12)
    api('POST','/api/setup', {'username':username,'password':password,'confirmPassword':password,'SelfUseModeEnabled':False,'DemoSiteEnabled':False})
account=api('POST','/api/user/login',{'username':username,'password':password})
assert account['role'] == 100
admin.addheaders.append(('New-Api-User',str(account['id'])))

def option(key, val):
    api('PUT','/api/option/',{'key':key,'value':val})

option('RetryTimes','2')
option('qiqi_setting.responses_stream_error_retry_enabled','true')
option('qiqi_setting.responses_stream_error_retry_times','2')
option('ChannelRetryPolicy','[]')
option('AutomaticRetryKeywords','')
option('ModelPrice',json.dumps({model:0.001}))
option('QuotaPerUnit','500000')
for lane, priority in [('a',10),('b',0)]:
    api('POST','/api/channel/',{'mode':'single','channel':{'type':1,'key':'isolated-'+lane,'name':'responses-fixture-'+lane,'base_url':f'http://fixture:18080/{lane}','models':model,'group':'default','priority':priority,'weight':1,'status':1,'auto_ban':0}})
api('POST','/api/token/',{'name':'isolated-smoke','group':'default','remain_quota':100000,'expired_time':-1,'first_byte_timeout':20,'upstream_timeout':30})
token = next(t for t in api('GET','/api/token/')['items'] if t['name']=='isolated-smoke')
key = api('POST',f"/api/token/{token['id']}/key")['key']

def once(scenario):
    global case
    with lock:
        case = scenario
        calls.clear()
    before = api('GET','/api/user/self')['quota']
    req = request.Request(base+'/v1/responses', data=json.dumps(sample, ensure_ascii=False).encode('utf-8'), headers={'Authorization':'Bearer '+key,'Content-Type':'application/json'})
    start = time.monotonic()
    with relay.open(req,timeout=5) as res:
        assert res.status == 200 and 'text/event-stream' in res.headers['Content-Type'], res.status
        raw = res.read(65536).decode()
    elapsed = time.monotonic() - start
    assert elapsed < 6, round(elapsed,2)
    with lock:
        actual = list(calls)
    items = [json.loads(l[6:]) for l in raw.splitlines() if l.startswith('data: {')]
    if scenario == 'before':
        assert actual == ['a','b'], (actual, raw[:800])
        assert 'smoke-a' not in raw and '"code":429' not in raw and raw.count('OK-from-fixture-B') == 1, raw[:800]
        assert sum(item.get('type') == 'response.completed' for item in items) == 1
    else:
        assert actual == ['a'] and 'smoke-b' not in raw, (actual, raw[:800])
        assert 'delivered reasoning' in raw and any(i.get('type')=='response.function_call_arguments.done' for i in items)
        assert raw.count('event: response.error') == 1, raw[:800]
        assert sum(i.get('code')==9007199254740993 for i in items) == 1, raw[:800]
        assert ('billable fixture output' in raw) == (scenario == 'after')
    charge = before - api('GET','/api/user/self')['quota']
    assert charge == (0 if scenario == 'after_no_usage' else 500), (scenario,charge)
    return {'scenario':scenario,'attempts':actual,'ended_before_upstream_closed':True,'charge_quota':charge}

try:
    observations = [once('before'), once('after_no_usage'), once('after')]
    logs = api('GET','/api/log/?type=2&model_name='+model)['items']
    assert sorted(l['quota'] for l in logs)==[0,500,500], [(l['quota'],l.get('channel_id')) for l in logs]
    assert api('GET','/api/status')['start_time']==status['start_time']
    result=dict(proof, early_rolled_to_second_channel=True, committed_error_once_without_replay=True, numeric_error_code_preserved=True, one_charge_per_billable_request=True, reasoning_tool_only_no_usage_kept_zero=True, real_upstream_calls=0, sample=sample, observations=observations)
    (HERE / 'isolated-responses.json').write_text(json.dumps(result,indent=2,ensure_ascii=False)+'\n', encoding='utf-8')
    print(json.dumps(result,ensure_ascii=False))
finally:
    server.shutdown()
