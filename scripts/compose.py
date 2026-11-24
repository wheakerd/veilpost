"""Manage this project's development Compose stack without deleting its data."""
import json
import os
import re
import subprocess
import sys
import webbrowser
from urllib.request import ProxyHandler, build_opener
from tooling import ROOT


def command(*arguments, capture=False, input=None):
    (ROOT / '.runtime/go-mod').mkdir(parents=True, exist_ok=True)
    project = os.environ.get('VPN_COMPOSE_PROJECT', 'veilpost-dev')
    if not re.fullmatch(r'[a-z0-9][a-z0-9_-]{0,62}', project):
        raise ValueError('Invalid VPN_COMPOSE_PROJECT')
    env = dict(os.environ, LOCAL_UID=str(os.getuid()), LOCAL_GID=str(os.getgid()))
    env.setdefault('GO_MODULE_CACHE_CONTEXT', str(ROOT / '.runtime/go-mod'))
    result = subprocess.run(['docker', 'compose', '-p', project, '-f', str(ROOT / 'compose.yaml'), *arguments],
                            cwd=ROOT, env=env, text=True, input=input, capture_output=capture, check=True)
    return result.stdout if capture else None


def endpoint(service, port):
    return 'http://' + command('port', service, str(port), capture=True).strip()


def status():
    command('ps')
    client = build_opener(ProxyHandler({}))
    for service, port in [('frontend', 3000), ('backend', 8080)]:
        address = endpoint(service, port)
        with client.open(address + '/api/ready', timeout=5) as response:
            if response.status != 200 or json.load(response).get('status') != 'ready':
                raise RuntimeError(f'{service} is not ready.')
        print(f'{service}: {address} (ready)')


def admin_create():
    subprocess.run(['sh', str(ROOT / 'scripts/admin-create.sh')], cwd=ROOT, check=True)


def main(action):
    if action in ('start', 'restart'):
        args = ['up', '-d', '--build', '--wait', '--wait-timeout', '180']
        if action == 'restart': args.append('--force-recreate')
        command(*args)
        status()
        ci = bool(os.environ.get('CI')) and os.environ.get('CI') != 'false'
        if os.environ.get('OPEN_BROWSER') != 'false' and not ci:
            webbrowser.open(endpoint('frontend', 3000), new=2)
    elif action == 'stop': command('stop')
    elif action == 'status': status()
    elif action == 'logs': command('logs', '--tail', '60')
    elif action == 'admin-create': admin_create()
    else: raise ValueError('Unknown Compose command.')


if __name__ == '__main__':
    try: main(sys.argv[1])
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
