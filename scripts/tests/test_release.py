from contextlib import redirect_stdout
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from publish import publish, validate_release
from release import archive, release_assets, require_tag, sha256, source_info, version_input, write_manifest


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.repo = self.root / 'repo'
        self.repo.mkdir()
        self.git('init', '-q')
        (self.repo / 'source.txt').write_text('fixture source\n')
        self.git('add', '.')
        self.git('-c', 'user.name=Release Test', '-c', 'user.email=release-test@example.invalid',
                 '-c', 'commit.gpgsign=false', 'commit', '-qm', 'fixture')
        self.version = 'v1.2.3-rc.1'
        self.git('tag', self.version)
        self.source = source_info(self.repo)
        self.output = self.root / self.version
        self.output.mkdir()
        self.assets = release_assets(self.version)
        for name in self.assets:
            (self.output / name).write_bytes(b'fixture artifact')
        write_manifest(self.output, self.version, self.source, self.assets)

    def git(self, *args):
        return subprocess.run(['git', '-c', 'core.hooksPath=/dev/null', *args], cwd=self.repo, check=True,
                              text=True, capture_output=True).stdout.strip()

    def test_semver_and_injection_inputs(self):
        for version in ['v0.1.0', 'v1.2.3-rc.1', 'v1.2.3-beta.2']:
            self.assertEqual(version_input(version), version)
        for version in ['', '1.2.3', 'v01.2.3', 'v1.2.3-01', '../v1.2.3', 'v1.2.3;echo unsafe',
                        'v1.2.3\n', 'v1.2.3+metadata']:
            with self.subTest(version=version), self.assertRaises(ValueError):
                version_input(version)

    def test_archives_are_reproducible_and_preserve_execution_mode(self):
        source = self.root / 'program'
        source.write_bytes(b'fixture binary')
        first, second = self.root / 'first.tgz', self.root / 'second.tgz'
        archive(first, [('veilpost-api', source, True)], 1700000000)
        os.utime(source, (1800000000, 1800000000))
        archive(second, [('veilpost-api', source, True)], 1700000000)
        self.assertEqual(sha256(first), sha256(second))
        with tarfile.open(first) as tar:
            member = tar.getmember('veilpost-api')
            self.assertEqual(member.mode, 0o755)
            self.assertEqual(member.uid, 0)
            self.assertEqual(member.mtime, 1700000000)

    def test_archive_rejects_symlinks_and_traversal(self):
        source = self.root / 'source'
        source.write_text('fixture')
        link = self.root / 'link'
        link.symlink_to(source)
        for name, path in [('veilpost-api', link), ('../escape', source)]:
            with self.assertRaises(RuntimeError):
                archive(self.root / 'bad.tgz', [(name, path, False)], 1)

    def test_valid_manifest_and_checksums(self):
        files = validate_release(self.output, self.version, self.source)
        self.assertEqual(len(files), 7)
        self.assertIn(f'veilpost-{self.version}-agent-linux-arm64.tar.gz', [path.name for path in files])

    def test_dirty_sources_or_manifest_cannot_publish(self):
        with self.assertRaisesRegex(RuntimeError, 'clean sources'):
            validate_release(self.output, self.version, {**self.source, 'dirty': True})
        write_manifest(self.output, self.version, {**self.source, 'dirty': True}, self.assets)
        with self.assertRaisesRegex(RuntimeError, 'clean sources'):
            validate_release(self.output, self.version, self.source)

    def test_corrupted_artifacts_and_checksums_cannot_publish(self):
        target = self.output / self.assets[0]
        target.write_bytes(b'tampered')
        with self.assertRaisesRegex(RuntimeError, 'checksum mismatch'):
            validate_release(self.output, self.version, self.source)
        write_manifest(self.output, self.version, self.source, self.assets)
        (self.output / 'SHA256SUMS').write_text('tampered')
        with self.assertRaisesRegex(RuntimeError, 'SHA256SUMS'):
            validate_release(self.output, self.version, self.source)

    def test_missing_or_extra_assets_cannot_publish(self):
        (self.output / 'private.env').write_text('fixture secret')
        with self.assertRaisesRegex(RuntimeError, 'Unexpected files'):
            validate_release(self.output, self.version, self.source)
        (self.output / 'private.env').unlink()
        metadata = json.loads((self.output / 'manifest.json').read_text())
        metadata['artifacts'][0]['name'] = '../private.env'
        (self.output / 'manifest.json').write_text(json.dumps(metadata))
        with self.assertRaisesRegex(RuntimeError, 'artifact set'):
            validate_release(self.output, self.version, self.source)

    def test_tag_must_match_source_commit(self):
        with self.assertRaisesRegex(RuntimeError, 'tag must point'):
            require_tag(self.version, '0' * 40, self.repo)

    def test_dry_run_never_contacts_github(self):
        with patch.dict(os.environ, {'RELEASE_DIR': str(self.root)}), patch('publish.run') as remote:
            with redirect_stdout(io.StringIO()):
                publish(self.version, 'example/project', dry_run=True, root=self.repo)
            remote.assert_not_called()

    def test_publish_uses_verified_tag_and_no_overwrite_flags(self):
        with patch.dict(os.environ, {'RELEASE_DIR': str(self.root)}), patch('publish.run') as remote:
            publish(self.version, 'example/project', root=self.repo)
            command = remote.call_args.args[0]
            self.assertEqual(command[:4], ['gh', 'release', 'create', self.version])
            self.assertIn('--verify-tag', command)
            self.assertIn('--prerelease', command)
            self.assertNotIn('--clobber', command)


if __name__ == '__main__':
    unittest.main()
