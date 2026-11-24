"""Verify that docker/soft-router matches its SHA-256 manifest.

Every reference file must be listed with a matching size and digest, and no
unlisted file may exist. --update rewrites the manifest from the files on disk
after an intentional edit; review the resulting diff before committing it.
"""
import argparse
import hashlib
import json
from pathlib import Path

DEFAULT_ROOT = Path(__file__).resolve().parents[1] / 'docker' / 'soft-router'
MANIFEST = 'source-manifest.json'
# The README and its Chinese translation describe the references.
UNTRACKED = {MANIFEST, 'README.md', 'README.zh-CN.md'}
PURPOSE = 'Sanitized Docker configuration references; no deployment performed'


def reference_files(root):
    return sorted(path.relative_to(root).as_posix() for path in root.rglob('*')
                  if path.is_file() and path.relative_to(root).as_posix() not in UNTRACKED)


def describe(root, relative):
    data = (root / relative).read_bytes()
    return {'path': relative, 'sha256': hashlib.sha256(data).hexdigest(), 'size': len(data)}


def verify(root):
    """Return a list of problems; an empty list means the manifest matches."""
    manifest = json.loads((root / MANIFEST).read_text(encoding='utf-8'))
    problems = []
    listed = [entry['path'] for entry in manifest['files']]
    duplicates = sorted({path for path in listed if listed.count(path) > 1})
    problems += [f'duplicate entry: {path}' for path in duplicates]
    for entry in manifest['files']:
        path = root / entry['path']
        if not path.is_file():
            problems.append(f"missing: {entry['path']}")
            continue
        actual = describe(root, entry['path'])
        if actual['size'] != entry['size'] or actual['sha256'] != entry['sha256']:
            problems.append(f"mismatch: {entry['path']}")
    problems += [f'unlisted: {path}' for path in reference_files(root) if path not in listed]
    return problems


def update(root):
    files = [describe(root, relative) for relative in reference_files(root)]
    manifest = {'purpose': PURPOSE, 'files': files}
    (root / MANIFEST).write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
    return files


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--root', type=Path, default=DEFAULT_ROOT, help=argparse.SUPPRESS)
    parser.add_argument('--update', action='store_true',
                        help='rewrite the manifest from the current files')
    args = parser.parse_args(argv)
    if args.update:
        print(f'UPDATED: {len(update(args.root))} files recorded in {MANIFEST}')
        return 0
    problems = verify(args.root)
    if problems:
        for problem in problems:
            print(f'FAIL: {problem}')
        return 1
    count = len(json.loads((args.root / MANIFEST).read_text(encoding='utf-8'))['files'])
    print(f'PASS: {count} files match {MANIFEST}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
