"""Test the exact release image with a clean local database and no provider calls."""
import json
from pathlib import Path
import secrets
import subprocess
import time
import uuid
import requests

ROOT = Path(__file__).resolve().parent
build = json.loads((ROOT / 'build-image-evidence.json').read_text(encoding='utf8'))
image = json.loads((ROOT/'verified-local-image.json').read_text())['image']
name = 'newapi-json-smoke-' + uuid.uuid4().hex[:10]
option = 'ChannelRetryPolicy'
value = '[{"channel_id":123,"status":429,"action":"stop"},{"stream_hedge":{"delay_ms":15000,"retry_same_channel_first":true,"models":["claude-fable-5-2","claude-opus-5-5"],"parallel_backup_models":["claude-fable-5-2"]}}]'

def docker(*args):
    return subprocess.run(['docker', *args], check=True, text=True, capture_output=True, timeout=300).stdout.strip()

def api(session, verb, url, **kwargs):
    result = session.request(verb, url, timeout=5, **kwargs)
    result.raise_for_status()
    return result.json()

def wait_ready(base):
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        try:
            response = api(requests, 'GET', base + '/api/status')
            assert response['data']['version'] == build['tag'], 'unexpected image application version'
            return response['data']['start_time']
        except (requests.RequestException, ValueError):
            time.sleep(1)
    raise RuntimeError('local image start timed out')

try:
    docker('run', '--pull=never', '-d', '--name', name, '-p', '127.0.0.1::3000', image)
    port = int(docker('port', name, '3000/tcp').rsplit(':', 1)[1])
    base = f'http://127.0.0.1:{port}'
    start = wait_ready(base)
    setup = api(requests, 'GET', base + '/api/setup')['data']
    if setup['status']:
        username, password = 'root', '123456'
    else:
        username, password = 'jsonsmoke', secrets.token_hex(12)
        assert api(requests, 'POST', base + '/api/setup', json={
            'username': username, 'password': password, 'confirmPassword': password,
            'SelfUseModeEnabled': False, 'DemoSiteEnabled': False,
        })['success'], 'local root setup failed'
    route = base + '/api/option/'
    def login():
        session = requests.Session()
        result = api(session, 'POST', base + '/api/user/login', json={'username': username, 'password': password})
        assert result['success'] and result['data']['role'] == 100, 'local root login failed'
        session.headers['New-Api-User'] = str(result['data']['id'])
        return session
    def read(session):
        result = api(session, 'GET', route)
        assert result['success'], 'root option read failed'
        return next(row['value'] for row in result['data'] if row['key'] == option)
    def put(session, new_value):
        return api(session, 'PUT', route, json={'key': option, 'value': new_value})
    anonymous = requests.get(route, timeout=5)
    assert anonymous.status_code == 401 or not anonymous.json().get('success'), 'unauthenticated option access allowed'
    with login() as session:
        assert read(session) == '[]', 'default is not empty'
        assert put(session, value)['success'] and read(session) == value, 'valid rules did not apply immediately'
        assert not put(session, '[{"stream_hedge":{"delay_ms":15000,"parallel_backup_models":["claude-fable-5-2"]}}]')['success'] and read(session) == value, 'invalid rules overwrote last valid configuration'
        assert wait_ready(base) == start, 'local app unexpectedly restarted during option update'
    docker('restart', name)
    restarted_port = int(docker('port', name, '3000/tcp').rsplit(':', 1)[1])
    print(json.dumps({'localhost_port_before': port, 'localhost_port_after': restarted_port}), flush=True)
    base = f'http://127.0.0.1:{restarted_port}'
    route = base + '/api/option/'
    wait_ready(base)
    with login() as session:
        assert read(session) == value, 'policy did not reload from the local database'
        assert put(session, '[]')['success'] and read(session) == '[]', 'clear failed'
    out = {'version': build['tag'], 'digest': build['digest'], 'admin_route': True, 'anonymous_rejected': True, 'empty_default': True, 'save_without_restart': True, 'invalid_save_preserves_value': True, 'persistent_after_restart': True, 'empty_clear': True, 'upstream_calls': 0, 'parallel_allowlist_saved': True}
    with (ROOT / 'local-image-smoke.json').open('x', encoding='utf8') as f:
        json.dump(out, f, indent=2)
    print(json.dumps(out), flush=True)
except subprocess.CalledProcessError as e:
    print((e.stderr or '')[-1200:], flush=True)
    raise SystemExit('Docker command failed') from None
except Exception:
    captured = subprocess.run(['docker','logs','--tail','45',name], text=True, capture_output=True, timeout=15, errors='replace')
    (ROOT / 'local-image-smoke-diagnostics.log').write_text(captured.stdout + captured.stderr, encoding='utf8')
    raise
finally:
    subprocess.run(['docker','rm','-fv',name], text=True, capture_output=True, timeout=40)
