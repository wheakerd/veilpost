"""Ownership-checked detached development processes (Linux only)."""
from collections import deque
from contextlib import contextmanager
import fcntl
import ipaddress
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
from urllib.request import ProxyHandler, build_opener


def process_identity(pid):
    try:
        fields = Path(f'/proc/{pid}/stat').read_text().rsplit(')', 1)[1].split()
        if fields[0] == 'Z':
            return None
        return {
            'pid': pid, 'start_ticks': fields[19], 'session': os.getsid(pid),
            'cwd': str(Path(f'/proc/{pid}/cwd').resolve(strict=True)),
        }
    except (FileNotFoundError, ProcessLookupError, PermissionError):
        return None


class DevServer:
    def __init__(self, name, cwd, runtime, port, prepare, ready, ready_path='/', wait_seconds=25,
                 discover=None, parameters=lambda: {}):
        self.name, self.cwd, self.runtime = name, Path(cwd).resolve(), Path(runtime)
        self.port, self.prepare, self.ready = port, prepare, ready
        self.ready_path, self.wait_seconds = ready_path, wait_seconds
        self.state_file = self.runtime / 'process.json'
        self.log_file = self.runtime / 'server.log'
        self.child = None
        self.discover, self.parameters = discover, parameters

    def address(self, state=None):
        port = state.get('port', self.port) if state else self.port
        host = state.get('ip', '127.0.0.1') if state else '127.0.0.1'
        host = f'[{host}]' if ':' in host else host
        return f'http://{host}:{port}'

    @property
    def url(self):
        return self.address()

    @contextmanager
    def locked(self):
        self.runtime.mkdir(parents=True, exist_ok=True, mode=0o700)
        with (self.runtime / 'control.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            yield self

    def read_state(self):
        if not self.state_file.exists():
            return None
        state = json.loads(self.state_file.read_text())
        if not isinstance(state, dict) or type(state.get('pid')) is not int or state['pid'] <= 1:
            raise RuntimeError(f'Invalid process record: {self.state_file}')
        if 'port' in state and (type(state['port']) is not int or not 1 <= state['port'] <= 65535):
            raise RuntimeError(f'Invalid process port: {self.state_file}')
        if 'ip' in state:
            ipaddress.ip_address(state['ip'])
        return state

    def running(self, state):
        return bool(state and state.get('cwd') == str(self.cwd)
                    and state.get('session') == state['pid']
                    and process_identity(state['pid']) == {
                        key: state.get(key) for key in ['pid', 'start_ticks', 'session', 'cwd']
                    })

    def write_state(self, state):
        with tempfile.NamedTemporaryFile(mode='w', dir=self.runtime, prefix='.process-', delete=False) as output:
            temporary = Path(output.name)
            try:
                output.write(json.dumps(state, indent=2) + '\n')
                output.flush()
                temporary.replace(self.state_file)
            finally:
                temporary.unlink(missing_ok=True)

    def is_ready(self, state=None):
        try:
            with build_opener(ProxyHandler({})).open(self.address(state) + self.ready_path, timeout=1) as response:
                return response.status == 200 and self.ready(response.read())
        except (OSError, TimeoutError, ValueError):
            return False

    def status(self):
        state = self.read_state()
        if not self.running(state):
            print(f'{self.name}: STOPPED')
            return False
        ready = self.is_ready(state)
        print(f'{self.name}: {"RUNNING" if ready else "NOT READY"} {self.address(state)} (PID {state["pid"]})')
        print(f'Log: {self.log_file}')
        return ready

    def logs(self):
        if self.log_file.exists():
            with self.log_file.open(errors='replace') as log:
                print(''.join(deque(log, maxlen=60)), end='')
        else:
            print(f'{self.name}: no log yet.')

    def stop(self):
        state = self.read_state()
        if not self.running(state):
            self.state_file.unlink(missing_ok=True)
            print(f'{self.name}: STOPPED (no owned process)')
            return
        for sig, timeout in [(signal.SIGTERM, 12), (signal.SIGKILL, 3)]:
            try:
                os.killpg(state['pid'], sig)
            except ProcessLookupError:
                break
            deadline = time.monotonic() + timeout
            while self.running(state) and time.monotonic() < deadline:
                if self.child:
                    self.child.poll()
                time.sleep(0.05)
            if not self.running(state):
                break
        if self.running(state):
            raise RuntimeError(f'Failed to stop {self.name}; process record retained.')
        if self.child:
            self.child.wait(timeout=1)
        self.state_file.unlink(missing_ok=True)
        print(f'{self.name}: STOPPED (PID {state["pid"]})')

    def start(self):
        state = self.read_state()
        parameters = self.parameters()
        prepared = None
        if self.running(state):
            if state.get('parameters', {}) != parameters:
                prepared = self.prepare()
                self.stop()
            else:
                if not self.is_ready(state):
                    raise RuntimeError(f'{self.name} is running but not ready; inspect {self.log_file}')
                self.status()
                return False
        if self.port:
            with socket.socket() as probe:
                probe.settimeout(1)
                if probe.connect_ex(('127.0.0.1', self.port)) == 0:
                    raise RuntimeError(f'{self.url} is occupied; the other process was left untouched.')
        command, env = prepared if prepared else self.prepare()
        with self.log_file.open('ab', buffering=0) as log:
            log.write(f'\n--- start {time.strftime("%Y-%m-%d %H:%M:%S %z")} ---\n'.encode())
            self.child = subprocess.Popen(command, cwd=self.cwd, env=env, stdin=subprocess.DEVNULL,
                                          stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        state = process_identity(self.child.pid)
        if not state:
            self.child.wait(timeout=1)
            raise RuntimeError(f'{self.name} exited immediately; inspect {self.log_file}')
        state['parameters'] = parameters
        if self.port:
            state['port'] = self.port
        try:
            self.write_state(state)
        except OSError:
            os.killpg(self.child.pid, signal.SIGTERM)
            self.child.wait(timeout=5)
            raise
        try:
            deadline = time.monotonic() + self.wait_seconds
            while time.monotonic() < deadline:
                if self.child.poll() is not None:
                    self.state_file.unlink(missing_ok=True)
                    raise RuntimeError(f'{self.name} exited; inspect {self.log_file}')
                if 'port' not in state and self.discover:
                    endpoint = self.discover(state)
                    if endpoint:
                        ipaddress.ip_address(endpoint['ip'])
                        if type(endpoint['port']) is not int or not 1 <= endpoint['port'] <= 65535:
                            raise RuntimeError('Backend reported an invalid bound port.')
                        state.update(ip=endpoint['ip'], port=endpoint['port'])
                        self.write_state(state)
                if 'port' in state and self.is_ready(state):
                    self.status()
                    return True
                time.sleep(0.1)
            raise RuntimeError(f'{self.name} did not become ready; inspect {self.log_file}')
        except BaseException:
            self.stop()
            raise


def start_all(servers):
    started = []
    try:
        for server in servers:
            if server.start():
                started.append(server)
    except Exception:
        for server in reversed(started):
            server.stop()
        raise
