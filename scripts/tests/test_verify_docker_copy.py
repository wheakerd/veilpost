from contextlib import redirect_stderr, redirect_stdout
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('verify_docker_copy', SCRIPTS / 'verify-docker-copy.py')
verify_docker_copy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify_docker_copy)


class VerifyDockerCopyTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / 'bundle').mkdir()
        (self.root / 'bundle' / 'compose.yaml').write_text('services: {}\n')
        (self.root / 'bundle' / '.env.example').write_text('DOMAIN=vpn.example.com\n')
        (self.root / 'README.md').write_text('# References\n')
        verify_docker_copy.update(self.root)

    def run_main(self, *arguments):
        output = io.StringIO()
        with redirect_stdout(output):
            status = verify_docker_copy.main(['--root', str(self.root), *arguments])
        return status, output.getvalue()

    def manifest(self):
        return json.loads((self.root / verify_docker_copy.MANIFEST).read_text())

    def test_update_records_hidden_files_and_skips_documentation(self):
        manifest = self.manifest()
        self.assertEqual(set(manifest), {'purpose', 'files'})
        self.assertEqual([entry['path'] for entry in manifest['files']],
                         ['bundle/.env.example', 'bundle/compose.yaml'])
        self.assertEqual(manifest['files'][1]['size'], len('services: {}\n'))

    def test_matching_files_pass(self):
        status, output = self.run_main()
        self.assertEqual(status, 0)
        self.assertEqual(output, 'PASS: 2 files match source-manifest.json\n')

    def test_modified_file_fails(self):
        (self.root / 'bundle' / 'compose.yaml').write_text('services: {changed: {}}\n')
        status, output = self.run_main()
        self.assertEqual(status, 1)
        self.assertIn('FAIL: mismatch: bundle/compose.yaml', output)

    def test_same_size_content_change_fails(self):
        (self.root / 'bundle' / 'compose.yaml').write_text('services: []\n')
        self.assertEqual(verify_docker_copy.verify(self.root), ['mismatch: bundle/compose.yaml'])

    def test_missing_file_fails(self):
        (self.root / 'bundle' / 'compose.yaml').unlink()
        self.assertEqual(verify_docker_copy.verify(self.root), ['missing: bundle/compose.yaml'])

    def test_unlisted_file_fails(self):
        (self.root / 'bundle' / 'extra.sh').write_text('#!/bin/sh\n')
        self.assertEqual(verify_docker_copy.verify(self.root), ['unlisted: bundle/extra.sh'])

    def test_readme_changes_are_not_tracked(self):
        (self.root / 'README.md').write_text('# Edited\n')
        self.assertEqual(verify_docker_copy.verify(self.root), [])

    def test_duplicate_entry_fails(self):
        manifest = self.manifest()
        manifest['files'].append(dict(manifest['files'][0]))
        (self.root / verify_docker_copy.MANIFEST).write_text(json.dumps(manifest))
        self.assertEqual(verify_docker_copy.verify(self.root), ['duplicate entry: bundle/.env.example'])

    def test_update_repairs_manifest(self):
        (self.root / 'bundle' / 'compose.yaml').write_text('services: {changed: {}}\n')
        status, output = self.run_main('--update')
        self.assertEqual((status, output), (0, 'UPDATED: 2 files recorded in source-manifest.json\n'))
        self.assertEqual(verify_docker_copy.verify(self.root), [])

    def test_source_option_was_removed(self):
        with redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as raised:
            verify_docker_copy.main(['--root', str(self.root), '--source'])
        self.assertEqual(raised.exception.code, 2)


class RepositoryManifestTests(unittest.TestCase):
    def test_repository_references_match_manifest(self):
        self.assertEqual(verify_docker_copy.verify(verify_docker_copy.DEFAULT_ROOT), [])

    def test_manifest_has_no_source_path(self):
        manifest = json.loads((verify_docker_copy.DEFAULT_ROOT / verify_docker_copy.MANIFEST).read_text())
        self.assertNotIn('source', manifest)
        self.assertNotIn('capturedAt', manifest)


if __name__ == '__main__':
    unittest.main()
