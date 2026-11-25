"""Root Makefile's implementation; the same commands run locally and in CI."""
import os
import subprocess
import sys

from tooling import BACKEND, AGENT, agent_make, backend_make, frontend, go_binary, go_environment, run

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
  make native-stop / native-restart / native-status / native-logs
See docs/releasing.md for release policy and CI triggers.
"""


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
