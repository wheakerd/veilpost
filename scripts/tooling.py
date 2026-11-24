"""Shared, repository-scoped tool selection and subprocess execution."""
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
FRONTEND = ROOT / 'app/frontend'
FRONTEND_DIST = ROOT / 'dist/frontend'
BACKEND = ROOT / 'app/backend'
AGENT = ROOT / 'app/agent'


def run(args, cwd=ROOT, env=None, capture=False):
    result = subprocess.run(
        [str(arg) for arg in args], cwd=cwd, env=env,
        check=True, text=True, capture_output=capture,
    )
    return result.stdout if capture else result


def go_environment():
    env = os.environ.copy()
    env.update(GOTOOLCHAIN='local', GOTELEMETRY='off')
    env.setdefault('GOCACHE', str(ROOT / '.runtime/go-build'))
    env.setdefault('GOMODCACHE', str(ROOT / '.runtime/go-mod'))
    return env


def go_binary():
    version = (BACKEND / '.go-version').read_text().strip()
    local = ROOT / f'.runtime/toolchains/go{version}/bin/go'
    binary = os.environ.get('GO') or (str(local) if local.is_file() else shutil.which('go'))
    if not binary:
        raise RuntimeError(f'Go {version} is required; install the version in app/backend/.go-version.')
    actual = run([binary, 'version'], env=go_environment(), capture=True).split()[2]
    if actual != f'go{version}':
        raise RuntimeError(f'Go {version} required, found {actual}.')
    return binary


def check_frontend_tools():
    import json
    package = json.loads((FRONTEND / 'package.json').read_text())
    expected_node = (FRONTEND / '.nvmrc').read_text().strip()
    expected_bun = package['packageManager'].split('@')[1]
    for tool, expected in [('node', expected_node), ('bun', expected_bun)]:
        binary = shutil.which(tool)
        if not binary:
            raise RuntimeError(f'{tool} {expected} is required.')
        actual = run([binary, '--version'], capture=True).strip().removeprefix('v')
        if actual != expected:
            raise RuntimeError(f'{tool} {expected} required, found {actual}; select the pinned version first.')


def frontend(*args):
    check_frontend_tools()
    return run(['bun', *args], cwd=FRONTEND)


def backend_make(target):
    return run(['make', '-C', BACKEND, f'GO={go_binary()}', target], env=go_environment())


def agent_make(target):
    return run(['make', '-C', AGENT, f'GO={go_binary()}', target], env=go_environment())
