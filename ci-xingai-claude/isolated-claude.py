"""Run the pinned release against two synthetic upstreams on a private Docker network."""
import http.cookiejar
import json
from pathlib import Path
import secrets
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib import error, request

HERE = Path(__file__).resolve().parent
proof = json.loads((HERE / 'isolated-image.json').read_text(encoding='utf-8'))
MODEL = 'claude-opus-5-5'
EMPTY = 'upstream stream ended before effective output'
CHARGE = 7 + 3 * 8
BASE = 'http://gateway:3000'
SAMPLE = {'model': MODEL, 'max_tokens': 64, 'stream': True,
          'messages': [{'role': 'user', 'content': 'Synthetic fixture prompt'}]}
admin = request.build_opener(request.ProxyHandler({}), request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
relay = request.build_opener(request.ProxyHandler({}))
lock = threading.Lock()
scenario = ''
calls = []
issues = []

def api(verb, path, data=None):
    encoded = None if data is None else json.dumps(data).encode()
    req = request.Request(BASE + path, method=verb, data=encoded, headers={'Content-Type':'application/json'})
    with admin.open(req, timeout=6) as res:
        val = json.load(res)
    assert val.get('success'), (path, val.get('message'))
    return val.get('data')


def balances():
    return (api('GET','/api/user/self')['quota'], api('GET',f'/api/token/{token_id}')['remain_quota'])


class Fixture(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        try:
            self.respond()
        except (BrokenPipeError, ConnectionResetError):
            pass
        except Exception as exc:
            with lock:
                issues.append(repr(exc))
            raise

    def respond(self):
        lane, path = self.path[1:].split('/', 1)
        assert lane in ('a','b') and path.split('?',1)[0] == 'v1/messages', self.path
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        with lock:
            label = scenario
        at_dispatch = balances()
        with lock:
            calls.append((lane, at_dispatch, body))
        assert body['model'] == MODEL and body['stream'] is True and body.get('container') is None
        primary = lane == 'a'
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Connection', 'close')
        self.end_headers()
        def frame(kind, **fields):
            msg = {'type':kind, **fields}
            self.wfile.write((f'event: {kind}\ndata: ' + json.dumps(msg, separators=(',', ':')) + '\n\n').encode())
            self.wfile.flush()
        if primary and label == 'mapped_json':
            # With hedge disabled, any earlier preamble is already delivered.
            frame('message_stop')
            threading.Event().wait(8)
            return
        def text(payload):
            frame('content_block_start', index=0, content_block={'type':'text','text':''})
            if payload:
                frame('content_block_delta', index=0, delta={'type':'text_delta','text':payload})
            frame('content_block_stop', index=0)
        msg = {'id':'stream-from-'+lane,'type':'message','role':'assistant','model':MODEL,
               'content':[], 'usage':{'input_tokens':11 if primary else 7,'output_tokens':1}}
        if primary and label == 'audit':
            msg['safeguard_results'] = {'provider':'fixture-audit'}
        frame('message_start', message=msg)
        frame('ping')
        if not primary:
            text('fixture-visible-b')
            frame('message_delta', delta={'stop_reason':'end_turn'}, usage={'output_tokens':3})
        elif label == 'provider_error':
            frame('error', error={'type':'bad_response_body','message':EMPTY})
            return
        elif label == 'committed':
            text('fixture-visible-a')
            frame('error', error={'type':'rate_limit_error','message':'fixture-after-text'})
            return
        elif label == 'tool':
            frame('content_block_start', index=0, content_block={'type':'tool_use','id':'fixture-tool','name':'lookup','input':{}})
            frame('content_block_stop', index=0)
            frame('message_delta', delta={'stop_reason':'tool_use'}, usage={'output_tokens':2})
        elif label == 'hosted_usage':
            frame('message_delta', delta={'stop_reason':'end_turn'}, usage={'output_tokens':1,'server_tool_use':{'web_search_requests':1}})
        elif label != 'audit':
            text('')
        frame('message_stop')
        if primary and label in ('mapped_status','mapped_json','rule_stop','unsafe_backup'):
            # A complete event without transport EOF: the gateway must use the explicit stop.
            threading.Event().wait(8)

server = ThreadingHTTPServer(('0.0.0.0', 18080), Fixture)
threading.Thread(target=server.serve_forever, daemon=True).start()


def option(key, val):
    api('PUT','/api/option/',{'key':key,'value':val})


def setup():
    for _ in range(50):
        try:
            status = api('GET','/api/status')
            break
        except (error.URLError, ValueError):
            time.sleep(1)
    else:
        raise RuntimeError('isolated gateway did not become healthy')
    assert status['version'] == proof['tag'], status['version']
    if api('GET','/api/setup')['status']:
        username, password = 'root', '123456'  # fresh local data only
    else:
        username, password = 'claude-smoke', secrets.token_hex(12)
        api('POST','/api/setup', {'username':username,'password':password,'confirmPassword':password,
                                 'SelfUseModeEnabled':False,'DemoSiteEnabled':False})
    account = api('POST','/api/user/login',{'username':username,'password':password})
    assert account['role'] == 100
    admin.addheaders.append(('New-Api-User',str(account['id'])))
    # Only this fresh test tenant: runner host load should not gate synthetic model traffic.
    option('performance_setting.monitor_enabled', 'false')
    option('RetryTimes', '2')
    option('ModelRatio', json.dumps({MODEL:1}))
    option('CompletionRatio', json.dumps({MODEL:8}))
    option('GroupRatio', json.dumps({'default':1}))
    option('LogConsumeEnabled','true')
    api('POST','/api/token/',{'name':'isolated-claude','group':'default','remain_quota':100000,
                             'expired_time':-1,'first_byte_timeout':20,'upstream_timeout':30})
    token = next(t for t in api('GET','/api/token/')['items'] if t['name'] == 'isolated-claude')
    key = api('POST',f"/api/token/{token['id']}/key")['key']
    for lane, priority in [('a',10),('b',0)]:
        row = {'type':14,'key':'isolated-'+lane,'name':'claude-fixture-'+lane,'base_url':f'http://fixture:18080/{lane}',
               'models':MODEL,'group':'default','priority':priority,'weight':1,'status':1,'auto_ban':0}
        if lane == 'a':
            row['status_code_mapping'] = json.dumps({'502':'429'})
        api('POST','/api/channel/',{'mode':'single','channel':row})
    channels = {r['name'].rsplit('-',1)[1]:r for r in api('GET','/api/channel/')['items']
                if r['name'].startswith('claude-fixture-')}
    assert set(channels) == {'a','b'}
    return token['id'], key, channels, status


def consumes():
    return api('GET','/api/log/?type=2&model_name='+MODEL)['items']


def one(label, want_lanes, want_status, visible=None, error_type=None, paid=True):
    global scenario
    before = balances()
    old_logs = {row['id'] for row in consumes()}
    delay = 0 if label in ('mapped_json','committed') else 15000
    policy = [{'stream_hedge':{'delay_ms':delay, 'retry_same_channel_first':True}}]
    if label in ('mapped_json','rule_stop','provider_error'):
        policy.append({'channel_id':channels['a']['id'],'status':429,'error_code':'bad_response_body',
                       'action':'stop' if label == 'rule_stop' else 'retry'})
    option('ChannelRetryPolicy', json.dumps(policy))
    option('AutomaticRetryKeywords', EMPTY if label == 'rule_stop' else '')
    option('AutomaticRetryStatusCodes', '503' if label == 'mapped_json' else '429')
    option('ModelPrice', json.dumps({} if paid else {MODEL:0}))
    with lock:
        scenario = label
        calls.clear()
    req = request.Request(BASE+'/v1/messages', data=json.dumps(SAMPLE).encode(),
                          headers={'Authorization':'Bearer '+key,'Content-Type':'application/json','anthropic-version':'2023-06-01'})
    start = time.monotonic()
    try:
        response = relay.open(req, timeout=5)
    except error.HTTPError as exc:
        response = exc
    with response as response:
        raw = response.read(65536).decode()
        status = response.code
        media = response.headers['Content-Type']
    elapsed = time.monotonic() - start
    with lock:
        actual = list(calls)
        assert not issues, issues
    lanes = [r[0] for r in actual]
    assert lanes == want_lanes and status == want_status and elapsed < 6, (label, lanes, status, round(elapsed,3), raw[:700])
    if want_status == 200:
        assert 'text/event-stream' in media and not any(line.startswith('{') for line in raw.splitlines()), (label,media,raw[:700])
        assert raw.count(visible) == 1, (label,raw[:700])
        if lanes == ['a','b']:
            assert 'stream-from-a' not in raw and 'stream-from-b' in raw, raw[:700]
        else:
            assert 'stream-from-b' not in raw and 'stream-from-a' in raw, raw[:700]
        if label != 'committed':
            events = [json.loads(line[6:]) for line in raw.splitlines() if line.startswith('data: {')]
            assert sum(event['type'] == 'message_stop' for event in events) == 1, raw[:700]
    else:
        assert 'application/json' in media and 'stream-from-' not in raw and 'event:' not in raw, (label,media,raw[:700])
        envelope = json.loads(raw)
        assert envelope['type'] == 'error' and envelope['error']['type'] == error_type, (label,envelope)
        assert EMPTY in envelope['error']['message'], (label,envelope)
    charge = CHARGE if lanes == ['a','b'] else 0
    if paid:
        reserved = actual[0][1]
        missing = tuple(b - r for b,r in zip(before,reserved))
        assert missing[0] == missing[1] and missing[0] > 0, (label,before,reserved)
        if lanes == ['a','b']:
            assert reserved == actual[1][1] and actual[0][2] == actual[1][2], (label,'duplicate precharge or incompatible payload')
    else:
        missing = (0,0)
    expected = tuple(x-charge for x in before)
    for _ in range(60):
        after = balances()
        if after == expected:
            break
        time.sleep(0.10)
    assert after == expected, (label,before,after,expected)
    fresh = [r for r in consumes() if r['id'] not in old_logs]
    if charge:
        assert [(r['channel'],r['prompt_tokens'],r['completion_tokens'],r['quota']) for r in fresh] == [(channels['b']['id'],7,3,CHARGE)], (label,fresh)
    elif paid:
        assert not fresh, (label,fresh)
    else:
        assert [(r['channel'],r['quota']) for r in fresh] == [(channels['a']['id'],0)], (label,fresh)
    observation = {'scenario':label,'attempts':lanes,'http':status,'elapsed_ms':round(elapsed*1000),
                   'precharge':missing[0], 'wallet_delta':before[0]-after[0], 'token_delta':before[1]-after[1], 'consume_logs':len(fresh)}
    print(json.dumps(observation), flush=True)
    return observation

try:
    token_id,key,channels,status = setup()
    observations = [one('mapped_status',['a','b'],200,'fixture-visible-b'),
                    one('mapped_json',['a','b'],200,'fixture-visible-b'),
                    one('rule_stop',['a'],429,error_type='rate_limit_error'),
                    one('audit',['a'],200,'fixture-audit',paid=False),
                    one('tool',['a'],200,'fixture-tool',paid=False),
                    one('committed',['a'],200,'fixture-visible-a'),
                    one('provider_error',['a'],429,error_type='bad_response_body'),
                    one('hosted_usage',['a'],200,'"web_search_requests":1',paid=False)]
    unsafe_b = {'id':channels['b']['id'], 'type':14, 'name':'claude-fixture-b', 'base_url':'http://fixture:18080/b',
                'key':'isolated-b', 'models':MODEL, 'group':'default', 'priority':0, 'weight':1, 'auto_ban':0,
                'param_override':json.dumps({'container':'fixture-container'})}
    api('PUT','/api/channel/',unsafe_b)
    option('RetryTimes','1')
    observations.append(one('unsafe_backup',['a'],429,error_type='rate_limit_error'))
    assert api('GET','/api/status')['start_time'] == status['start_time']
    result = dict(proof, passed=True, mapped_status_and_json_retry=True, json_stop_before_fallback=True,
                  audit_and_tool_not_replayed=True, committed_error_not_replayed=True,
                  billing_and_refund=True, hosted_usage_and_unsafe_backup=True, real_upstream_calls=0,
                  observations=observations)
    (HERE / 'isolated-claude-proof.json').write_text(json.dumps(result,indent=2)+'\n', encoding='utf-8')
    print(json.dumps(result), flush=True)
finally:
    server.shutdown()
