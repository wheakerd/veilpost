"""Build local release archives. Publishing is a separate, guarded command."""
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile

from tooling import ROOT, FRONTEND_DIST, BACKEND, AGENT, frontend, go_binary, go_environment, run

ARCHITECTURES = ['amd64', 'arm64']

VERSION_PATTERN = re.compile(
    r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
    r'(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
    r'(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?'
)


def version_input(value):
    if not VERSION_PATTERN.fullmatch(value):
        raise ValueError('VERSION must be vMAJOR.MINOR.PATCH, optionally with a SemVer prerelease suffix.')
    return value


def source_info(root=ROOT):
    commit = run(['git', 'rev-parse', 'HEAD'], cwd=root, capture=True).strip()
    epoch = int(run(['git', 'show', '-s', '--format=%ct', 'HEAD'], cwd=root, capture=True).strip())
    dirty = bool(run(['git', 'status', '--porcelain', '--untracked-files=normal'], cwd=root, capture=True).strip())
    return {'commit': commit, 'source_epoch': epoch, 'dirty': dirty}


def require_tag(version, commit, root=ROOT):
    resolved = run(['git', 'rev-parse', '--verify', f'refs/tags/{version}^{{commit}}'], cwd=root, capture=True).strip()
    if resolved != commit:
        raise RuntimeError('Release tag must point to the checked-out source commit.')


def sha256(path):
    with Path(path).open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def archive(output, entries, epoch):
    """Fixed ordering, ownership, modes and timestamps; no host paths or symlinks."""
    with Path(output).open('wb') as raw:
        with gzip.GzipFile(filename='', fileobj=raw, mode='wb', mtime=0) as zipped:
            with tarfile.open(fileobj=zipped, mode='w', format=tarfile.PAX_FORMAT) as tar:
                for name, path, executable in sorted(entries):
                    path = Path(path)
                    if path.is_symlink() or not path.is_file():
                        raise RuntimeError(f'Only regular files can be released: {path}')
                    if name.startswith('/') or '..' in Path(name).parts:
                        raise RuntimeError('Unsafe archive member name.')
                    content = path.read_bytes()
                    info = tarfile.TarInfo(name)
                    info.size, info.mtime, info.mode = len(content), epoch, 0o755 if executable else 0o644
                    info.uid = info.gid = 0
                    info.uname = info.gname = ''
                    tar.addfile(info, io.BytesIO(content))


def write_manifest(directory, version, source, assets):
    metadata = {
        'schema': 1, 'version': version, **source,
        'artifacts': [{'name': name, 'sha256': sha256(directory / name)} for name in sorted(assets)],
    }
    (directory / 'manifest.json').write_text(json.dumps(metadata, indent=2, sort_keys=True) + '\n')
    checksums = ''.join(f'{sha256(directory / name)}  {name}\n' for name in sorted([*assets, 'manifest.json']))
    (directory / 'SHA256SUMS').write_text(checksums)


def release_directory(version):
    base = Path(os.environ.get('RELEASE_DIR') or ROOT / 'dist/releases')
    return base.resolve() / version


def release_assets(version):
    """Every archive a release must contain, in publishing order."""
    return [f'veilpost-{version}-frontend.tar.gz',
            *(f'veilpost-{version}-linux-{arch}.tar.gz' for arch in ARCHITECTURES),
            *(f'veilpost-{version}-agent-linux-{arch}.tar.gz' for arch in ARCHITECTURES)]


def go_build(go, directory, package, binary, architecture, version, commit):
    env = go_environment()
    env.update(CGO_ENABLED='0', GOOS='linux', GOARCH=architecture)
    flags = f'-s -w -X main.version={version} -X main.commit={commit}'
    run([go, 'build', '-mod=readonly', '-trimpath', '-buildvcs=false', '-ldflags', flags,
         '-o', binary, package], cwd=directory, env=env)


def build_release(version):
    version = version_input(version)
    source = source_info()
    if os.environ.get('CI'):
        if source['dirty']:
            raise RuntimeError('CI releases require a clean working tree.')
        require_tag(version, source['commit'])
    output = release_directory(version)
    if output.exists():
        raise RuntimeError(f'Release output already exists: {output}; use another RELEASE_DIR instead of overwriting it.')
    go = go_binary()
    frontend('run', 'build')
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.release-', dir=output.parent) as temporary:
        stage = Path(temporary)
        assets = []
        frontend_name = f'veilpost-{version}-frontend.tar.gz'
        dist = FRONTEND_DIST
        if not (dist / 'index.html').is_file():
            raise RuntimeError('Frontend production entry is missing.')
        entries = []
        for path in dist.rglob('*'):
            if path.is_symlink():
                raise RuntimeError('Frontend distribution must not contain symlinks.')
            if path.is_file():
                entries.append((f'frontend/{path.relative_to(dist).as_posix()}', path, False))
        archive(stage / frontend_name, entries, source['source_epoch'])
        assets.append(frontend_name)
        for architecture in ARCHITECTURES:
            binary = stage / f'veilpost-api-{architecture}'
            go_build(go, BACKEND, './cmd/api', binary, architecture, version, source['commit'])
            name = f'veilpost-{version}-linux-{architecture}.tar.gz'
            archive(stage / name, [
                ('veilpost-api', binary, True),
                ('admin-create.sh', ROOT / 'scripts/admin-create.sh', True),
                ('backend.env.example', BACKEND / '.env.example', False),
                ('README.md', ROOT / 'docs/releasing.md', False),
            ], source['source_epoch'])
            binary.unlink()
            assets.append(name)
        for architecture in ARCHITECTURES:
            binary = stage / f'veilpost-agent-{architecture}'
            go_build(go, AGENT, './cmd/veilpost-agent', binary, architecture, version, source['commit'])
            name = f'veilpost-{version}-agent-linux-{architecture}.tar.gz'
            archive(stage / name, [
                ('veilpost-agent', binary, True),
                ('README.md', AGENT / 'README.md', False),
            ], source['source_epoch'])
            binary.unlink()
            assets.append(name)
        if assets != release_assets(version):
            raise RuntimeError('Release archives do not match the expected asset list.')
        # A changed lockfile/source during the build must not get a clean manifest.
        after = source_info()
        if after != source:
            raise RuntimeError('Source state changed during packaging; retry from a stable checkout.')
        write_manifest(stage, version, source, assets)
        stage.rename(output)
    print(f'Release archives: {output}')
    if source['dirty']:
        print('Local preview only: manifest is marked dirty and publishing will be refused.')
    return output


if __name__ == '__main__':
    try:
        build_release(os.environ.get('VERSION', ''))
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
