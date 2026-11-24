import json
import os
from pathlib import Path
import socket
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from dev_server import DevServer, start_all


class DevServerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.script = self.root / 'fixture.py'
        self.script.write_text('''from http.server import BaseHTTPRequestHandler, HTTPServer
import json, os, sys
from pathlib import Path
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"owned-fixture")
    def log_message(self, *args):
        pass
server = HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler)
if len(sys.argv) > 2:
    Path(sys.argv[2]).write_text(json.dumps({"pid": os.getpid(), "ip": server.server_address[0], "port": server.server_address[1]}))
server.serve_forever()
''')

    def server(self, ready=lambda body: body == b'owned-fixture', wait_seconds=3):
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', 0))
            port = probe.getsockname()[1]
        return DevServer('fixture', self.root, self.root / 'runtime', port,
                         lambda: ([sys.executable, str(self.script), str(port)], os.environ.copy()),
                         ready, wait_seconds=wait_seconds)

    def test_start_reuse_and_stop_owned_process(self):
        server = self.server()
        with server.locked():
            self.addCleanup(server.stop)
            self.assertTrue(server.start())
            state = server.read_state()
            self.assertFalse(server.start())
            self.assertEqual(state, server.read_state())
            self.assertTrue(server.status())
            server.stop()
            self.assertFalse(server.state_file.exists())
            self.assertFalse(server.running(state))

    def test_occupied_port_is_not_taken_over(self):
        server = self.server()
        with socket.socket() as listener, server.locked():
            listener.bind(('127.0.0.1', server.port))
            listener.listen()
            with patch.object(server, 'prepare') as prepare:
                with self.assertRaisesRegex(RuntimeError, 'occupied'):
                    server.start()
                prepare.assert_not_called()
            self.assertEqual(listener.getsockname()[1], server.port)

    def test_status_uses_recorded_port_and_reads_legacy_identity(self):
        server = self.server()
        with server.locked():
            self.addCleanup(server.stop)
            server.start()
            port = server.port
            server.port = 1
            self.assertTrue(server.status())
            self.assertFalse(server.start())
            state = server.read_state()
            self.assertEqual(state['port'], port)
            # Records written by the original frontend script had no port field.
            server.port = port
            state.pop('port')
            server.state_file.write_text(json.dumps(state))
            self.assertTrue(server.status())

    def test_failed_readiness_stops_only_new_process(self):
        server = self.server(ready=lambda _: False, wait_seconds=0.3)
        with server.locked():
            self.addCleanup(server.stop)
            with self.assertRaisesRegex(RuntimeError, 'not become ready'):
                server.start()
            self.assertIsNotNone(server.child.poll())
            self.assertFalse(server.state_file.exists())

    def test_stale_pid_record_never_sends_signal(self):
        server = self.server()
        with server.locked(), patch('dev_server.os.killpg') as signal_process:
            server.state_file.write_text(json.dumps({'pid': os.getpid(), 'start_ticks': 'stale',
                                                    'session': os.getpid(), 'cwd': str(self.root)}))
            server.stop()
            signal_process.assert_not_called()

    def test_invalid_record_fails_without_signalling(self):
        server = self.server()
        with server.locked(), patch('dev_server.os.killpg') as signal_process:
            server.state_file.write_text('{"pid": 1}')
            with self.assertRaisesRegex(RuntimeError, 'Invalid process record'):
                server.stop()
            signal_process.assert_not_called()

    def test_group_failure_preserves_preexisting_services(self):
        existing, created, failing = Mock(), Mock(), Mock()
        existing.start.return_value = False
        created.start.return_value = True
        failing.start.side_effect = RuntimeError('fixture failure')
        with self.assertRaisesRegex(RuntimeError, 'fixture failure'):
            start_all([existing, created, failing])
        existing.stop.assert_not_called()
        created.stop.assert_called_once()
        failing.stop.assert_not_called()

    def test_dynamic_port_is_discovered_from_the_started_process(self):
        announcement = self.root / 'listen.json'
        def discover(state):
            if not announcement.exists():
                return None
            info = json.loads(announcement.read_text())
            return info if info['pid'] == state['pid'] else None
        server = DevServer('dynamic', self.root, self.root / 'runtime', 0,
                           lambda: ([sys.executable, str(self.script), '0', str(announcement)], os.environ.copy()),
                           lambda body: body == b'owned-fixture', discover=discover)
        with server.locked():
            self.addCleanup(server.stop)
            server.start()
            state = server.read_state()
            self.assertGreater(state['port'], 0)
            self.assertEqual(state['ip'], '127.0.0.1')
            self.assertEqual(state['port'], json.loads(announcement.read_text())['port'])
            self.assertTrue(server.status())

    def test_missing_dynamic_announcement_cannot_claim_readiness(self):
        server = DevServer('dynamic', self.root, self.root / 'runtime', 0,
                           lambda: ([sys.executable, str(self.script), '0'], os.environ.copy()),
                           lambda _: True, discover=lambda _: None, wait_seconds=0.3)
        with server.locked():
            self.addCleanup(server.stop)
            with self.assertRaisesRegex(RuntimeError, 'not become ready'):
                server.start()
            self.assertFalse(server.state_file.exists())
            self.assertIsNotNone(server.child.poll())

    def test_changed_backend_parameters_restart_frontend(self):
        server = self.server()
        parameters = {'backend_host': '127.0.0.1', 'backend_port': 10001}
        server.parameters = lambda: dict(parameters)
        with server.locked():
            self.addCleanup(server.stop)
            server.start()
            first_pid = server.read_state()['pid']
            parameters['backend_port'] = 10002
            self.assertTrue(server.start())
            self.assertNotEqual(server.read_state()['pid'], first_pid)
            self.assertEqual(server.read_state()['parameters'], parameters)

    def test_reconfiguration_preflight_failure_preserves_running_frontend(self):
        server = self.server()
        parameters = {'backend_port': 10001}
        server.parameters = lambda: dict(parameters)
        with server.locked():
            self.addCleanup(server.stop)
            server.start()
            first = server.read_state()
            parameters['backend_port'] = 10002
            with patch.object(server, 'prepare', side_effect=RuntimeError('preflight')):
                with self.assertRaisesRegex(RuntimeError, 'preflight'):
                    server.start()
            self.assertEqual(server.read_state(), first)
            self.assertTrue(server.status())


if __name__ == '__main__':
    unittest.main()
