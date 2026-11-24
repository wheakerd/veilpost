"""Run the real Go/SQLite backend and frontend against disposable test data."""
import fcntl
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import threading
import time

from tooling import ROOT, FRONTEND, BACKEND, backend_make, check_frontend_tools

runtime = ROOT / '.runtime/e2e'
runtime.mkdir(parents=True, exist_ok=True, mode=0o700)
stop = threading.Event()
signal.signal(signal.SIGTERM, lambda *_: stop.set())
signal.signal(signal.SIGINT, lambda *_: stop.set())

with (runtime / 'control.lock').open('a') as lock:
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    backend_make('build')
    check_frontend_tools()
    with tempfile.TemporaryDirectory(prefix='data-', dir=runtime) as temporary:
        data = Path(temporary)
        announcement = data / 'listen.json'
        env = dict(os.environ, VPN_ENV='development', VPN_DATA_DIR=str(data),
                   VPN_HTTP_ADDR='127.0.0.1:0', VPN_PUBLIC_ORIGIN='http://127.0.0.1:4173', OPEN_BROWSER='false')
        children = []
        try:
            backend = subprocess.Popen([str(BACKEND / 'bin/veilpost-api'), '--listen-file', str(announcement)], env=env, cwd=BACKEND)
            children.append(backend)
            for _ in range(200):
                if backend.poll() is not None: raise RuntimeError('Real test backend failed to start')
                if announcement.exists(): break
                time.sleep(.05)
            info = json.loads(announcement.read_text())
            assert info['pid'] == backend.pid
            (runtime / 'session.json').write_text(json.dumps({'pid': os.getpid(), 'dataDir': str(data), 'backend': info}))
            frontend = subprocess.Popen([shutil.which('node'), 'scripts/dev.ts', '--host', '127.0.0.1', '--port', '4173',
                                         '--backend-host', info['ip'], '--backend-port', str(info['port']), '--no-open'],
                                        cwd=FRONTEND, env=env)
            children.append(frontend)
            while not stop.wait(.2):
                if any(child.poll() is not None for child in children): raise RuntimeError('Test service exited unexpectedly')
        finally:
            for child in reversed(children):
                if child.poll() is None:
                    child.terminate()
                    try: child.wait(timeout=12)
                    except subprocess.TimeoutExpired: child.kill(); child.wait()
            (runtime / 'session.json').unlink(missing_ok=True)
