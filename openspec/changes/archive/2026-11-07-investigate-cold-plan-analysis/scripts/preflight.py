"""Check exact fixture queries against copied, frozen repositories."""
import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import subprocess
import time


def check(root, revisions, env, query):
    name = query['Repository']
    mirror = root / 'mirrors' / (name + '.git')
    commit = revisions[name]['commit']
    path = f"{query['Category']}/{query['Name']}/{query['Name']}-{query['Version']}.ebuild"
    start = time.monotonic()
    command = ['git', '-C', str(mirror), 'log', '--first-parent', '--full-history', '-1', '--format=%H %P', commit, '--', ':(literal)' + path]
    try:
        history = subprocess.run(command, env=env, text=True, capture_output=True, timeout=60, check=True)
        tree = subprocess.run(['git', '-C', str(mirror), 'ls-tree', commit, '--', ':(literal)' + path], env=env, text=True, capture_output=True, timeout=10, check=True)
        if not history.stdout.strip() and not tree.stdout.strip():
            # This proof requires a complete mirror. Reject shallow inputs below.
            result = {'status': 'not_found', 'proof': 'empty first-parent path history and absent reference entry', 'command': command, 'stdout': history.stdout, 'reference_tree': tree.stdout}
        else:
            resolved = subprocess.run([str(root / 'preflight'), 'resolve', str(mirror), commit, name, query['Category'], query['Name'], query['Version']], env=env, text=True, capture_output=True, timeout=60)
            found = resolved.returncode == 0 and json.loads(resolved.stdout)['Status'] == 'found'
            result = {'status': 'resolved' if found else 'error', 'returncode': resolved.returncode, 'stdout': resolved.stdout, 'stderr': resolved.stderr}
    except subprocess.TimeoutExpired:
        result = {'status': 'timeout'}
    except subprocess.CalledProcessError as error:
        result = {'status': 'error', 'stderr': error.stderr}
    return {'query': query, 'reference': commit, 'duration_s': time.monotonic() - start, **result}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('root', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    revisions = json.loads((args.root / 'evidence/repositories.json').read_text())
    env = os.environ.copy()
    env['GIT_CONFIG_COUNT'] = str(len(revisions))
    for index, (name, revision) in enumerate(revisions.items()):
        assert revision['shallow'] == 'false', 'complete history required'
        env[f'GIT_CONFIG_KEY_{index}'] = 'safe.directory'
        env[f'GIT_CONFIG_VALUE_{index}'] = str(args.root / 'mirrors' / (name + '.git'))
    queries = {}
    for plan in [2, 3]:
        for query in json.loads((args.root / f'evidence/plan-{plan}-queries.json').read_text())['Queries']:
            queries[tuple(sorted(query.items()))] = query
    failed = False
    with args.output.open('x') as output, concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        for row in pool.map(lambda query: check(args.root, revisions, env, query), queries.values()):
            output.write(json.dumps(row) + '\n')
            output.flush()
            failed |= row['status'] != 'resolved'
    raise SystemExit(1 if failed else 0)
