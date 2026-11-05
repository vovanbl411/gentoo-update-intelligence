"""Container acceptance test using real ebuilds from the pinned build stage.

Run from the repository root. Docker operations are restricted to temporary
Compose projects and bind directories created by this script.
"""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time


def command(args, **kwargs):
    return subprocess.run(args, check=True, text=True, **kwargs)


def prepare(root):
    """Called inside the builder; never writes to the user's Git repository."""
    source = Path('/var/db/repos/gentoo')
    work = root / 'gentoo-work'
    work.mkdir()
    for directory in ('profiles', 'eclass'):
        shutil.copytree(source / directory, work / directory, symlinks=True)
    (work / 'metadata').mkdir()
    shutil.copy2(source / 'metadata/layout.conf', work / 'metadata/layout.conf')
    packages = ('dev-libs/libei', 'dev-libs/libxml2', 'sys-libs/glibc', 'dev-vcs/git')
    versions = {}
    for package in packages:
        shutil.copytree(source / package, work / package, symlinks=True)
        name = package.split('/')[1]
        ebuild = sorted(p for p in (work / package).glob('*.ebuild')
                        if '9999' not in p.name)[-1]
        assert 'EAPI=8' in ebuild.read_text(), ebuild
        versions[package] = ebuild.stem[len(name) + 1:]

    identity = dict(os.environ, GIT_AUTHOR_NAME='Runtime test',
                    GIT_AUTHOR_EMAIL='test@example.invalid',
                    GIT_COMMITTER_NAME='Runtime test',
                    GIT_COMMITTER_EMAIL='test@example.invalid')

    def snapshot(directory, parent=None):
        if not (directory / '.git').exists():
            command(['git', 'init', '-q', str(directory)])
        git = ['git', '-C', str(directory)]
        command(git + ['add', '.'])
        tree = subprocess.check_output(git + ['write-tree'], text=True).strip()
        args = git + ['commit-tree', tree, '-m', 'runtime fixture']
        if parent:
            args += ['-p', parent]
        commit = subprocess.check_output(args, env=identity, text=True).strip()
        command(git + ['update-ref', 'refs/heads/main', commit])
        command(git + ['symbolic-ref', 'HEAD', 'refs/heads/main'])
        return commit

    first = snapshot(work)
    # Real ebuild, artificial removal commit to exercise first-parent lookup.
    historical = versions['dev-libs/libei']
    (work / f'dev-libs/libei/libei-{historical}.ebuild').unlink()
    reference = snapshot(work, first)
    command(['git', 'clone', '--bare', str(work), str(root / 'gentoo-source.git')])

    overlay = root / 'overlay-work'
    (overlay / 'metadata').mkdir(parents=True)
    (overlay / 'metadata/layout.conf').write_text('masters = gentoo\n')
    (overlay / 'profiles').mkdir()
    (overlay / 'profiles/repo_name').write_text('test-overlay\n')
    shutil.copytree(source / 'dev-libs/libei', overlay / 'dev-libs/libei')
    overlay_commit = snapshot(overlay)
    command(['git', 'clone', '--bare', str(overlay), str(root / 'overlays/test-overlay.git')])

    (root / 'fixture.json').write_text(json.dumps({
        'versions': versions, 'first': first, 'reference': reference,
        'overlay_commit': overlay_commit,
    }, indent=2) + '\n')


def test_image(root, image, label):
    data = root / label
    data.mkdir()
    for directory in ('metadata', 'cache'):
        (data / directory).mkdir(mode=0o777)
        (data / directory).chmod(0o777)
    override = data / 'compose.yaml'
    override.write_text(f'''services:
  gentoo:
    image: {image}
    environment:
      GENTOO_REPOSITORY_URL: /fixtures/gentoo-source.git
    volumes: !override
      - {root}/gentoo-source.git:/fixtures/gentoo-source.git:ro
      - {data}/metadata:/metadata
  metadata-worker:
    image: {image}
    volumes: !override
      - {data}/metadata:/metadata:ro
      - {root}/overlays:/overlays:ro
      - {data}/cache:/metadata-cache
''')
    project = root.name + '-' + label
    compose = ['docker', 'compose', '--env-file', '/dev/null', '-p', project,
               '-f', 'compose.yaml', '-f', str(override)]
    env = dict(os.environ, POSTGRES_PASSWORD='runtime-test-only')

    def run(*args, **kwargs):
        return command(compose + list(args), env=env, **kwargs)

    def output(*args):
        return run(*args, stdout=subprocess.PIPE).stdout.strip()

    fixture = json.loads((root / 'fixture.json').read_text())
    results = {}
    try:
        run('up', '-d', '--no-build', '--pull', 'never', '--wait',
            '--wait-timeout', '60', 'gentoo', 'metadata-worker')
        deadline = time.monotonic() + 30
        while True:
            probe = subprocess.run(compose + [
                'exec', '-T', 'gentoo', 'git', '-C', '/metadata/gentoo.git',
                'rev-parse', '--verify', 'refs/gentoo/published',
            ], env=env, text=True, capture_output=True)
            if probe.returncode == 0:
                assert probe.stdout.strip() == fixture['reference']
                break
            if time.monotonic() >= deadline:
                raise AssertionError('sync did not publish the fixture revision')
            time.sleep(1)

        ids = [output('ps', '-q', service) for service in ('gentoo', 'metadata-worker')]
        images = subprocess.check_output([
            'docker', 'inspect', '--format', '{{.Image}}', *ids,
        ], text=True).splitlines()
        assert images[0] == images[1]
        status = output('exec', '-T', 'metadata-worker', 'cat', '/proc/1/status')
        uid = output('exec', '-T', 'metadata-worker', 'id', '-u', 'portage')
        assert f'Uid:\t{uid}\t{uid}\t{uid}\t{uid}' in status
        run('exec', '-T', '--user', 'portage', 'metadata-worker', 'sh', '-ec',
            'test -w /metadata-cache; test -w /var/tmp/gentoo-update-intelligence; '
            'if touch /metadata/readonly-probe; then exit 1; fi')
        network = json.loads(subprocess.check_output([
            'docker', 'network', 'inspect', project + '_metadata-network',
        ], text=True))[0]
        assert network['Internal']
        worker_networks = json.loads(subprocess.check_output([
            'docker', 'inspect', '--format', '{{json .NetworkSettings.Networks}}', ids[1],
        ], text=True))
        assert list(worker_networks) == [project + '_metadata-network']

        primary = {'Name': 'gentoo', 'URL': '/fixtures/gentoo-source.git',
                   'Path': '/metadata/gentoo.git', 'Commit': fixture['reference']}
        requests = []
        for package, version in fixture['versions'].items():
            category, name = package.split('/')
            requests.append((package, {
                'Repository': primary, 'Category': category, 'Name': name,
                'Version': version, 'Profile': 'profiles/default/linux/amd64/23.0',
            }))
        requests.append(('overlay', {
            'Repository': {'Name': 'test-overlay', 'URL': '/overlays/test-overlay.git',
                           'Path': '/overlays/test-overlay.git',
                           'Commit': fixture['overlay_commit']},
            'Masters': [primary], 'Category': 'dev-libs', 'Name': 'libei',
            'Version': fixture['versions']['dev-libs/libei'],
            'Profile': 'gentoo:profiles/default/linux/amd64/23.0',
        }))
        client = '''import sys, urllib.request, urllib.error
req = urllib.request.Request("http://127.0.0.1:8090/prepare",
    data=sys.stdin.buffer.read(), headers={"Content-Type": "application/json"})
try:
    with urllib.request.urlopen(req, timeout=120) as response:
        sys.stdout.buffer.write(response.read())
except urllib.error.HTTPError as error:
    print(error.read().decode(), file=sys.stderr)
    raise
'''

        def request(payload):
            return json.loads(run(
                'exec', '-T', '--user', 'portage', 'metadata-worker',
                'python3', '-c', client, input=json.dumps(payload),
                stdout=subprocess.PIPE,
            ).stdout)

        for name, payload in requests:
            response = request(payload)
            result = response['result']
            content = base64.b64decode(result['Content'])
            assert b'EAPI=8\n' in content, (name, content)
            assert hashlib.sha256(content).hexdigest() == result['SHA256']
            assert result['Provenance']['portage_version']
            assert not response['cached']
            if name == 'dev-libs/libei':
                assert response['source_commit'] == fixture['first']
                assert result['Provenance']['selection_reason'] == 'first_parent_history'
            if name == 'overlay':
                assert result['Provenance']['master_revisions'][0]['commit'] == fixture['reference']
            results[name] = response
            print(f'PASS {label}: {name} {payload["Version"]} generated', flush=True)
        repeat = request(requests[0][1])
        assert repeat['cached']
        assert repeat['result']['Content'] == results[requests[0][0]]['result']['Content']
        (data / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
        print(f'PASS {label}: sync, image identity, UID, mounts, network and cache reuse', flush=True)
        return results
    finally:
        with (data / 'containers.log').open('w') as log:
            run('logs', '--no-color', 'gentoo', 'metadata-worker', stdout=log)
        run('down', '--timeout', '5')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', default='gentoo-update-intelligence/gentoo-runtime:local')
    parser.add_argument('--builder-image', default='gentoo-update-intelligence/gentoo-base-builder:test')
    parser.add_argument('--baseline-image')
    parser.add_argument('--prepare', type=Path, help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.prepare:
        prepare(args.prepare)
        return
    root = Path(tempfile.mkdtemp(prefix='guit-compact-test-'))
    root.chmod(0o755)
    (root / 'overlays').mkdir()
    script = Path(__file__).resolve()
    print('Evidence:', root, flush=True)
    command([
        'docker', 'run', '--rm', '--network', 'none',
        '-v', f'{root}:/fixtures', '-v', f'{script}:/test-runtime.py:ro',
        '--entrypoint', 'python3', args.builder_image,
        '/test-runtime.py', '--prepare', '/fixtures',
    ])
    baseline = test_image(root, args.baseline_image, 'baseline') if args.baseline_image else None
    results = test_image(root, args.image, 'compact')
    if baseline:
        for name in results:
            assert results[name]['result']['Content'] == baseline[name]['result']['Content'], name
        print('PASS: metadata contents match baseline for all requests', flush=True)


if __name__ == '__main__':
    main()
