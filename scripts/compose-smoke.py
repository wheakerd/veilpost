"""Real authentication/persistence smoke test, restricted to an empty CI stack."""
import http.cookiejar
import json
import os
import secrets
import time
from urllib.error import URLError
from urllib.request import HTTPCookieProcessor, ProxyHandler, Request, build_opener
from compose import command, endpoint

if os.environ.get('CI') != 'true' or os.environ.get('VPN_COMPOSE_PROJECT') != 'veilpost-ci':
    raise SystemExit('This test only runs with CI=true and VPN_COMPOSE_PROJECT=veilpost-ci.')

base = endpoint('frontend', 3000)
client = build_opener(ProxyHandler({}), HTTPCookieProcessor(http.cookiejar.CookieJar()))


def request(path, data=None):
    body = None if data is None else json.dumps(data).encode()
    req = Request(base + path, data=body, headers={'Origin': base, 'Content-Type': 'application/json', 'X-Requested-With': 'Veilpost'})
    with client.open(req, timeout=5) as response:
        return None if response.status == 204 else json.load(response)


assert request('/api/auth/session').get('setupRequired') is True, 'Refusing to modify a non-empty authentication database'
username = 'smoke_' + secrets.token_hex(8)
password = secrets.token_urlsafe(32)
command('exec', '-T', 'backend', '/usr/local/bin/veilpost-api', 'admin-create', '--username', username, '--password-stdin', input=password + '\n')
assert request('/api/auth/login', {'username': username, 'password': password})['status'] == 'authenticated'
command('restart', 'backend')
for attempt in range(60):
    try:
        if request('/api/ready')['status'] == 'ready': break
    except (URLError, ValueError): pass
    time.sleep(.5)
else: raise RuntimeError('Backend did not recover after restart')
assert request('/api/auth/session')['user']['username'] == username
request('/api/auth/logout', {})
assert request('/api/auth/session')['status'] == 'anonymous'
print('PASS: container login, durable session after backend restart, and logout')
