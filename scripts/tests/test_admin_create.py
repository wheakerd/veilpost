"""Exercise the production Shell entry point with a PATH that has no Python."""
import os
import pty
from pathlib import Path
import shutil
import select
import stat
import subprocess
import tempfile
import termios
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/admin-create.sh'


class AdminCreateTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for tool in ('mkdir', 'mktemp', 'chmod', 'stat', 'openssl', 'env', 'stty'):
            self.bin.joinpath(tool).symlink_to(shutil.which(tool))
        self.backend = self.root / 'veilpost-api'
        self.backend.write_text('''#!/bin/sh
umask 077
printf '%s\\n' "$@" >> "$TRACE_ARGS"
IFS= read -r received
printf '%s\\n' "$received" > "$TRACE_PASSWORD"
env > "$TRACE_ENV"
printf '%s\\n' "$received"
printf '%s\\n' "$received" >&2
exit "${BACKEND_RESULT:-0}"
''')
        self.backend.chmod(0o700)
        self.env = {
            'PATH': str(self.bin), 'VPN_API_BIN': str(self.backend),
            'VPN_DATA_DIR': str(self.root / 'data'),
            'VPN_CREDENTIALS_DIR': str(self.root / 'credentials'),
            'TRACE_ARGS': str(self.root / 'args'), 'TRACE_PASSWORD': str(self.root / 'received'),
            'TRACE_ENV': str(self.root / 'environment'), 'PACKAGE_LOG': str(self.root / 'packages'),
        }

    def run_script(self, **extra):
        return subprocess.run(['/bin/sh', str(SCRIPT)], env=self.env | extra,
                              input='', capture_output=True, text=True)

    def credentials(self):
        return list((self.root / 'credentials').glob('native-*'))

    def password(self, path):
        return dict(line.split('=', 1) for line in path.read_text().splitlines())['password']

    def test_generation_without_python_and_secret_transport(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        path, = self.credentials()
        password = self.password(path)
        self.assertRegex(password, r'^[0-9a-f]{64}$')
        self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
        self.assertEqual(stat.S_IMODE(path.parent.stat().st_mode), 0o700)
        self.assertEqual((self.root / 'received').read_text(), password + '\n')
        self.assertEqual((self.root / 'args').read_text().splitlines(),
                         ['admin-create', '--username', 'admin', '--password-stdin'])
        self.assertNotIn(password, result.stdout + result.stderr)
        self.assertNotIn(password, (self.root / 'environment').read_text())
        self.assertIn('管理员已创建: admin', result.stdout)

    def test_fresh_passwords_never_overwrite_existing_file(self):
        first = self.run_script(USERNAME='owner.name', GENERATE_PASSWORD='1')
        self.assertEqual(first.returncode, 0, first.stderr)
        path, = self.credentials()
        original = path.read_bytes()
        self.assertEqual(self.run_script(USERNAME='owner.name').returncode, 0)
        self.assertEqual(path.read_bytes(), original)
        self.assertEqual(len({self.password(file) for file in self.credentials()}), 2)

    def test_backend_error_retains_password_and_never_retries(self):
        result = self.run_script(BACKEND_RESULT='1')
        self.assertNotEqual(result.returncode, 0)
        path, = self.credentials()
        self.assertNotIn(self.password(path), result.stdout + result.stderr)
        self.assertIn('账号创建未确认', result.stderr)
        self.assertNotIn('管理员已创建:', result.stdout)
        self.assertEqual((self.root / 'args').read_text().count('admin-create'), 1)

    def test_password_reset_uses_the_same_secret_transport(self):
        result = self.run_script(ADMIN_ACTION='reset-password')
        self.assertEqual(result.returncode, 0, result.stderr)
        path, = self.credentials()
        password = self.password(path)
        self.assertEqual((self.root / 'received').read_text(), password + '\n')
        self.assertEqual((self.root / 'args').read_text().splitlines(),
                         ['admin-reset-password', '--username', 'admin', '--password-stdin'])
        self.assertNotIn(password, result.stdout + result.stderr)
        self.assertIn('密码已重置并撤销全部会话: admin', result.stdout)
        failed = self.run_script(ADMIN_ACTION='reset-password', BACKEND_RESULT='1')
        self.assertNotEqual(failed.returncode, 0)
        self.assertIn('密码重置未确认', failed.stderr)

    def test_two_factor_reset_needs_no_password(self):
        result = self.run_script(ADMIN_ACTION='reset-two-factor', USERNAME='owner.name')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'args').read_text().splitlines(),
                         ['admin-reset-two-factor', '--username', 'owner.name'])
        self.assertEqual(self.credentials(), [])
        self.assertIn('已移除两步验证并撤销全部会话: owner.name', result.stdout)
        failed = self.run_script(ADMIN_ACTION='reset-two-factor', BACKEND_RESULT='1')
        self.assertNotEqual(failed.returncode, 0)
        self.assertIn('两步验证未重置', failed.stderr)

    def test_unsafe_inputs_do_not_create_files_or_call_backend(self):
        for values in ({'USERNAME': '../admin'}, {'USERNAME': '-owner'}, {'GENERATE_PASSWORD': 'yes'},
                       {'GENERATE_PASSWORD': '0'}, {'VPN_COMPOSE_PROJECT': '../other'}, {'VPN_DATA_DIR': ''},
                       {'ADMIN_ACTION': 'delete'}):
            with self.subTest(values=values):
                self.assertNotEqual(self.run_script(**values).returncode, 0)
                self.assertEqual(self.credentials(), [])
                self.assertFalse((self.root / 'args').exists())

    def test_insecure_or_linked_secret_directory_is_rejected(self):
        directory = self.root / 'credentials'
        directory.mkdir()
        directory.chmod(0o755)
        self.assertNotEqual(self.run_script().returncode, 0)
        directory.rmdir()
        directory.symlink_to(self.bin, target_is_directory=True)
        self.assertNotEqual(self.run_script().returncode, 0)
        self.assertFalse((self.root / 'args').exists())

    def test_write_failure_never_calls_backend(self):
        directory = self.root / 'credentials'
        directory.mkdir(mode=0o700)
        directory.chmod(0o500)
        self.addCleanup(directory.chmod, 0o700)
        self.assertNotEqual(self.run_script().returncode, 0)
        self.assertFalse((self.root / 'args').exists())

    def test_release_layout_finds_adjacent_backend(self):
        script = self.root / 'admin-create.sh'
        shutil.copy2(SCRIPT, script)
        result = subprocess.run(['/bin/sh', 'admin-create.sh'], cwd=self.root, env=self.env | {'VPN_API_BIN': ''},
                                input='', capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.root / 'args').exists())

    def test_manual_terminal_input_hides_and_restores_echo(self):
        master, slave = pty.openpty()
        self.addCleanup(os.close, master)
        self.addCleanup(os.close, slave)
        original = termios.tcgetattr(slave)
        process = subprocess.Popen(['/bin/sh', str(SCRIPT)], stdin=slave, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, env=self.env | {'USERNAME': 'owner'})
        self.addCleanup(lambda: process.kill() if process.poll() is None else None)
        self.assertTrue(select.select([process.stdout], [], [], 5)[0], 'Password prompt timed out')
        prompt = os.read(process.stdout.fileno(), 1024)
        self.assertIn('密码'.encode(), prompt)
        self.assertFalse(termios.tcgetattr(slave)[3] & termios.ECHO)
        os.write(master, b'manual-password-example\nmanual-password-example\n')
        output, error = process.communicate(timeout=5)
        self.assertEqual(process.returncode, 0, error.decode())
        self.assertEqual(termios.tcgetattr(slave), original)
        self.assertNotIn(b'manual-password-example', prompt + output + error)
        self.assertEqual((self.root / 'received').read_text(), 'manual-password-example\n')
        self.assertEqual(self.credentials(), [])

    def run_functions(self, statements, extra=None):
        # Test definitions from the shipped script without invoking its main entry point.
        definitions = SCRIPT.read_text().rsplit('\nmain "$@"', 1)[0]
        return subprocess.run(['/bin/sh', '-c', definitions + '\n' + statements],
                              env=self.env | (extra or {}), input='', capture_output=True, text=True)

    def package_command(self, name, code=0):
        path = self.bin / name
        path.write_text(f'#!/bin/sh\nprintf "%s\\n" "{name} $*" >> "$PACKAGE_LOG"\nexit {code}\n')
        path.chmod(0o700)

    def test_distribution_package_manager_mapping(self):
        expected = {
            ('ubuntu', ''): ['apt-get update', 'apt-get install -y --no-install-recommends openssl'],
            ('linuxmint', 'ubuntu debian'): ['apt-get update', 'apt-get install -y --no-install-recommends openssl'],
            ('fedora', ''): ['dnf install -y openssl'],
            ('rocky', 'rhel fedora'): ['dnf install -y openssl'],
            ('alpine', ''): ['apk add --no-cache openssl'],
            ('manjaro', 'arch'): ['pacman -S --needed --noconfirm openssl'],
            ('opensuse-leap', 'suse'): ['zypper --non-interactive install --no-recommends openssl'],
        }
        for name in ('apt-get', 'dnf', 'yum', 'apk', 'pacman', 'zypper'):
            self.package_command(name)
        for (identity, like), commands in expected.items():
            with self.subTest(identity=identity):
                log = self.root / 'packages'
                log.unlink(missing_ok=True)
                result = self.run_functions(f'as_root() {{ "$@"; }}\ninstall_packages {identity} "{like}" openssl')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(log.read_text().splitlines(), commands)
        (self.bin / 'dnf').unlink()
        result = self.run_functions('as_root() { "$@"; }\ninstall_packages centos "rhel" openssl')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.root / 'packages').read_text().endswith('yum install -y openssl\n'))

    def test_missing_tool_installs_then_rechecks_and_continues(self):
        (self.bin / 'openssl').unlink()
        result = self.run_functions('''generate=true
install_packages() {
  printf '%s\\n' "$*" > "$PACKAGE_LOG"
  /bin/ln -s "$REAL_OPENSSL" "$PATH/openssl"
}
ensure_tools
openssl version >/dev/null
printf 'continued\\n'
''', {'REAL_OPENSSL': shutil.which('openssl')})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('continued', result.stdout)
        self.assertTrue((self.root / 'packages').read_text().rstrip().endswith('openssl'))

    def test_failed_or_ineffective_installation_stops(self):
        self.package_command('dnf', 1)
        result = self.run_functions('as_root() { "$@"; }\ninstall_packages fedora "" openssl\nprintf continued')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn('continued', result.stdout)
        (self.bin / 'openssl').unlink()
        result = self.run_functions('generate=true\ninstall_packages() { :; }\nensure_tools\nprintf continued')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn('continued', result.stdout)

    def test_unknown_distribution_does_not_try_arbitrary_manager(self):
        self.package_command('apt-get')
        result = self.run_functions('install_packages unknown "" openssl')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / 'packages').exists())

    def test_noninteractive_sudo_never_prompts_for_password(self):
        self.package_command('sudo', 1)
        result = self.run_functions('effective_uid() { printf 1000; }\nas_root apk add openssl')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / 'packages').read_text(), 'sudo -n -- apk add openssl\n')


if __name__ == '__main__':
    unittest.main()
