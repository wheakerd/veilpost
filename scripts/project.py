"""Root Makefile's implementation; the same commands run locally and in CI."""
from contextlib import ExitStack
import json
import os
import shutil
import subprocess
import sys

from dev_server import DevServer, start_all
from tooling import ROOT, FRONTEND, BACKEND, AGENT, agent_make, backend_make, check_frontend_tools, frontend, go_binary, go_environment, run

HELP = """Veilpost — root commands
  make install   Install locked frontend and Go dependencies
  make start     Start Docker Compose frontend/backend (see make admin-create)
  make stop      Stop only this repository's owned development processes
  make restart   Restart both services (backend rebuilds on startup)
  make status    Show both process and HTTP readiness states
  make logs      Show the last 60 lines of each service log
  make test      Script tests, Go/SQLite and agent race tests, real-backend browser tests
  make check     Formatting, vet, dependency/reference checks, then all tests
  make build     Build dist/frontend, app/backend/bin/veilpost-api and app/agent/bin/veilpost-agent
  make release VERSION=v0.1.0   Build local versioned release archives
  make publish VERSION=v0.1.0   Publish checked archives for an existing tag
  make admin-create  Create an administrator; generate a password when no terminal is attached
  make admin-create GENERATE_PASSWORD=1 USERNAME=admin  Generate and save a random password
  make admin-reset-password USERNAME=admin    Reset a password and revoke the account's sessions
  make admin-reset-two-factor USERNAME=admin  Remove lost two-step verification and revoke sessions
  make native-start  Optional native development mode
See docs/releasing.md for release policy and CI triggers.
"""


def servers():
    listen_file = ROOT / '.runtime/backend-dev/listen.json'

    def frontend_parameters():
        state = backend.read_state()
        if backend.running(state) and backend.is_ready(state):
            return {'backend_host': state.get('ip', '127.0.0.1'), 'backend_port': state['port']}
        return {}

    def prepare_frontend():
        check_frontend_tools()
        if not (FRONTEND / 'node_modules/@rsbuild/core/package.json').is_file():
            raise RuntimeError('Frontend dependencies missing; run make install.')
        command = [shutil.which('node'), str(FRONTEND / 'scripts/dev.ts'),
                   '--host', '127.0.0.1', '--port', '3000']
        parameters = frontend_parameters()
        if parameters:
            command.extend(['--backend-host', parameters['backend_host'],
                            '--backend-port', str(parameters['backend_port'])])
        env = os.environ.copy()
        for key in ['DEV_BACKEND_HOST', 'DEV_BACKEND_PORT']:
            env.pop(key, None)
        return command, env

    def prepare_backend():
        backend_make('build')
        env = os.environ.copy()
        env.update(VPN_HTTP_ADDR='127.0.0.1:0', VPN_SHUTDOWN_TIMEOUT='10s', VPN_ENV='development', VPN_DATA_DIR=str(ROOT / '.runtime/backend-data'))
        listen_file.unlink(missing_ok=True)
        return [str(BACKEND / 'bin/veilpost-api'), '--listen-file', str(listen_file)], env

    def discover_backend(state):
        if not listen_file.exists():
            return None
        info = json.loads(listen_file.read_text())
        if info.get('pid') != state['pid']:
            return None
        if info.get('ip') != '127.0.0.1':
            raise RuntimeError('Development backend must report its loopback address.')
        return info

    backend = DevServer('backend', BACKEND, ROOT / '.runtime/backend-dev', 0, prepare_backend,
                        lambda body: json.loads(body) == {'status': 'ok', 'service': 'veilpost'},
                        '/api/health', discover=discover_backend)
    return [
        backend,
        DevServer('frontend', FRONTEND, ROOT / '.runtime/frontend-dev', 3000, prepare_frontend,
                  lambda body: b'id="root"' in body, parameters=frontend_parameters),
    ]


def lifecycle(action, selection=None):
    all_servers = servers()
    selected = [server for server in all_servers if selection is None or server.name == selection]
    with ExitStack() as stack:
        # Frontend-only launches also lock the backend while discovering its address.
        for server in all_servers:
            stack.enter_context(server.locked())
        if action in ('stop', 'restart'):
            for server in reversed(selected):
                server.stop()
        if action in ('start', 'restart'):
            start_all(selected)
        if action == 'status':
            return 0 if all([server.status() for server in selected]) else 1
        if action == 'logs':
            for server in selected:
                print(f'{server.name} log:')
                server.logs()
    return 0


def install():
    frontend('install', '--frozen-lockfile')
    run([go_binary(), 'mod', 'download'], cwd=BACKEND, env=go_environment())
    run([go_binary(), 'mod', 'download'], cwd=AGENT, env=go_environment())


def build():
    frontend('run', 'build')
    backend_make('build')
    agent_make('build')


def test():
    workers = os.environ.get('TEST_WORKERS') or '4'
    if not workers.isdecimal() or not 1 <= int(workers) <= 32:
        raise RuntimeError('TEST_WORKERS must be an integer from 1 to 32.')
    run([sys.executable, '-m', 'unittest', 'discover', '-s', 'scripts/tests', '-v'])
    backend_make('test')
    agent_make('test')
    frontend('run', 'build')
    frontend('run', 'test:dev')
    frontend('run', 'test:e2e', f'--workers={workers}')


def check():
    frontend('run', 'format:check')
    frontend('run', 'lint')
    frontend('x', 'prettier', '--config', '.prettierrc.json', '--check',
             '../../README.md', '../../README.zh-CN.md', '../backend/README.md', '../backend/README.zh-CN.md',
             '../agent/README.md', '../agent/README.zh-CN.md', '../../docs', '../../.github')
    backend_make('fmt-check')
    backend_make('vet')
    run([go_binary(), 'mod', 'verify'], cwd=BACKEND, env=go_environment())
    agent_make('fmt-check')
    agent_make('vet')
    run([go_binary(), 'mod', 'verify'], cwd=AGENT, env=go_environment())
    run([sys.executable, 'scripts/verify-docker-copy.py'])
    test()


def main():
    action = sys.argv[1] if len(sys.argv) == 2 else 'help'
    if action == 'help':
        print(HELP)
        return 0
    if action in ('start', 'stop', 'restart', 'status', 'logs'):
        return lifecycle(action)
    commands = {'install': install, 'test': test, 'check': check, 'build': build}
    if action not in commands:
        raise RuntimeError(f'Unknown command: {action}')
    commands[action]()
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
