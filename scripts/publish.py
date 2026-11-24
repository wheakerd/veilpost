"""Publish immutable, verified archives for an existing Git tag with GitHub CLI."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

from release import release_assets, release_directory, require_tag, sha256, source_info, version_input
from tooling import ROOT, run


def validate_release(directory, version, source):
    directory = Path(directory)
    if directory.is_symlink():
        raise RuntimeError('Release directory must not be a symlink.')
    metadata_path = directory / 'manifest.json'
    if metadata_path.is_symlink():
        raise RuntimeError('Release manifest must not be a symlink.')
    metadata = json.loads(metadata_path.read_text())
    if (metadata.get('schema') != 1 or metadata.get('version') != version
            or metadata.get('commit') != source['commit'] or metadata.get('dirty') is not False
            or metadata.get('source_epoch') != source['source_epoch']
            or source['dirty']):
        raise RuntimeError('Publishing requires clean sources and a matching version/commit manifest.')
    expected = set(release_assets(version))
    artifacts = metadata.get('artifacts', [])
    if len(artifacts) != len(expected) or {item['name'] for item in artifacts} != expected:
        raise RuntimeError('Release artifact set is incomplete or unexpected.')
    for item in artifacts:
        path = directory / item['name']
        if path.is_symlink() or sha256(path) != item['sha256']:
            raise RuntimeError(f'Artifact checksum mismatch: {item["name"]}')
    files = sorted([*expected, 'manifest.json'])
    checksum_path = directory / 'SHA256SUMS'
    if checksum_path.is_symlink() or checksum_path.read_text() != ''.join(f'{sha256(directory / name)}  {name}\n' for name in files):
        raise RuntimeError('SHA256SUMS does not match the release files.')
    if {path.name for path in directory.iterdir()} != set(files) | {'SHA256SUMS'}:
        raise RuntimeError('Unexpected files in the release directory.')
    return [directory / name for name in [*files, 'SHA256SUMS']]


def publish(version, repository, dry_run=False, root=ROOT):
    version = version_input(version)
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repository):
        raise ValueError('GITHUB_REPOSITORY must be an explicit owner/repository.')
    source = source_info(root)
    require_tag(version, source['commit'], root)
    files = validate_release(release_directory(version), version, source)
    command = ['gh', 'release', 'create', version, *files, '--repo', repository,
               '--verify-tag', '--target', source['commit'], '--title', version,
               '--generate-notes']
    if '-' in version:
        command.extend(['--prerelease', '--latest=false'])
    if dry_run:
        print(json.dumps({'repository': repository, 'version': version, 'commit': source['commit'],
                          'assets': [str(path) for path in files]}, indent=2))
        return
    # CLI create refuses existing releases. Never upload with --clobber or move tags.
    # Its asset workflow creates a draft, uploads files, then publishes on success.
    run(command, cwd=root)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dry-run', action='store_true', help='Validate locally without contacting GitHub.')
    args = parser.parse_args()
    try:
        publish(os.environ.get('VERSION', ''), os.environ.get('GITHUB_REPOSITORY', ''), args.dry_run)
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
