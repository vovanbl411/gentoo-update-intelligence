"""Freeze task inputs, including untracked Parser fixes, without reading .env files."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('repository', type=Path)
    parser.add_argument('destination', type=Path)
    args = parser.parse_args()
    repo = args.repository.resolve()
    dest = args.destination.resolve()
    (dest / 'source').mkdir(parents=True, exist_ok=False)
    (dest / 'evidence').mkdir(exist_ok=True)

    def git(*words):
        return subprocess.check_output(['git', '-C', str(repo), *words])

    scope = ['backend', 'testdata', 'gentoo', 'scripts', 'config/backend.yaml']
    paths = set(git('ls-files', '-z', '--', *scope).decode().split('\0'))
    paths.update(git('ls-files', '--others', '--exclude-standard', '-z', '--', 'backend', 'testdata/parser').decode().split('\0'))
    manifest = []
    for name in sorted(paths - {''}):
        path = Path(name)
        if any(part.startswith('.env') or part in ['credentials', 'secrets', 'sessions', 'telemetry'] for part in path.parts):
            raise ValueError('sensitive path excluded: ' + name)
        original = repo / path
        if not original.exists():
            continue
        if original.is_symlink():
            raise ValueError('inspect symlink before copying: ' + name)
        target = dest / 'source' / path
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(original, target)
        manifest.append({'path': name, 'sha256': hashlib.sha256(original.read_bytes()).hexdigest()})
    (dest / 'evidence/source-manifest.json').write_text(json.dumps({'head': git('rev-parse', 'HEAD').decode().strip(), 'files': manifest}, indent=2) + '\n')
    (dest / 'evidence/source.patch').write_bytes(git('diff', 'HEAD', '--', *scope))
    (dest / 'evidence/initial-status.txt').write_bytes(git('status', '--short'))
