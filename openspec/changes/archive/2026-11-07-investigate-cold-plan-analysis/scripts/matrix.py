"""Isolated guest-only cold/warm matrix. Existing run directories are refused."""
import datetime
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.request

ROOT = Path('/opt/guit')
RESULTS = ROOT / 'results'
BASE = None
NAMES = ['cold-backend', 'cold-worker', 'cold-postgres']


def run(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, **kwargs).stdout


def docker(*args, **kwargs):
    return run('docker', *args, **kwargs)


def backend_base():
    import ipaddress
    address = docker('inspect', '--format', '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}', 'cold-backend').strip()
    assert not ipaddress.ip_address(address).is_loopback
    return f'http://{address}:8080/api/v1/analyses'


def request(url, data=None):
    req = urllib.request.Request(url, data=None if data is None else json.dumps(data).encode(), headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=30) as response:
        raw = response.read()
        return json.loads(raw), raw


def has_metadata_result(result):
    return result is not None and any(
        (report.get(side, {}).get('result') or {}).get('status') == 'found'
        for report in result.get('reports', [])
        for side in ['before_lookup', 'after_lookup']
    )


def measure(label, plan):
    dest = RESULTS / label
    dest.mkdir(exist_ok=False)
    stop = dest / 'observer.stop'
    observer = subprocess.Popen([sys.executable, str(ROOT / 'observe.py'), str(dest / 'guest.jsonl'), '--guest', '--stop-file', str(stop)])
    start = time.monotonic()
    summary = {'label': label, 'plan': plan, 'wall_start_ns': time.time_ns(), 'successful': False}
    try:
        initial, raw = request(BASE, {'plan': (ROOT / f'ebuild-updates-{plan}.txt').read_text()})
        (dest / 'initial.json').write_bytes(raw)
        ident = initial['id']
        summary['id'] = ident
        previous_poll_wall_ns = time.time_ns()
        previous = 0
        with (dest / 'progress.jsonl').open('x') as progress:
            deadline = start
            while time.monotonic() - start < 7200:
                response, raw = request(BASE + '/' + ident + '/partial-report')
                wall_ns = time.time_ns()
                gap_s = (wall_ns - previous_poll_wall_ns) / 1e9
                if gap_s > 10:
                    if response['status'] in ['completed', 'failed']:
                        (dest / 'late-result.json').write_bytes(request(BASE + '/' + ident)[1])
                    raise RuntimeError(f'observation poll gap {gap_s:.1f}s exceeds 10s')
                previous_poll_wall_ns = wall_ns
                elapsed = time.monotonic() - start
                result = response.get('result')
                completed = response['progress']['completed']
                if completed < previous:
                    raise RuntimeError('progress decreased')
                previous = completed
                progress.write(json.dumps({'elapsed_s': elapsed, 'wall_ns': wall_ns, 'response': response}) + '\n')
                progress.flush()
                if result is not None and 'first_partial_s' not in summary:
                    summary['first_partial_s'] = elapsed
                if has_metadata_result(result) and 'first_metadata_s' not in summary:
                    summary['first_metadata_s'] = elapsed
                if response['status'] in ['completed', 'failed']:
                    summary['terminal_s'] = elapsed
                    summary['status'] = response['status']
                    full, raw = request(BASE + '/' + ident)
                    (dest / 'result.json').write_bytes(raw)
                    verify(full, plan)
                    summary['successful'] = True
                    break
                deadline += 1
                time.sleep(max(0, deadline - time.monotonic()))
            else:
                raise TimeoutError('7200 second analysis deadline')
    except Exception as exc:
        summary['failure'] = repr(exc)
        raise
    finally:
        summary['wall_end_ns'] = time.time_ns()
        stop.touch()
        observer.wait(timeout=10)
        for name in NAMES:
            since = datetime.datetime.fromtimestamp(summary['wall_start_ns'] / 1e9, datetime.timezone.utc).isoformat()
            (dest / (name + '.log')).write_text(subprocess.run(['docker', 'logs', '--since', since, name], text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT).stdout)
        (dest / 'containers.json').write_text(docker('inspect', '--format', '{{json .State}}', *NAMES))
        (dest / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    print(json.dumps(summary), flush=True)
    return dest


def verify_report(entry, report):
    before, after = report['before_lookup'], report['after_lookup']
    if entry['operation'] == 'install':
        assert entry['new_slot']
        assert entry['package'] == {'category': 'sys-kernel', 'name': 'gentoo-kernel-bin'}
        for lookup in [before, after]:
            assert lookup['query'] is None and lookup['result'] is None
            assert lookup['error'] is None and lookup['skip_reason'] == 'BINDING_REJECTED'
        assert report['report']['gentoo']['status'] == 'unavailable'
        assert [d['code'] for d in report['report']['gentoo']['diagnostics']] == ['OPERATION_UNSUPPORTED']
        return 'binding_rejected'
    target_only = entry['operation'] == 'reinstall' and not entry['installed']['versions']
    for side, lookup in [('before', before), ('after', after)]:
        assert lookup['error'] is None, (side, lookup['error'])
        if side == 'before' and target_only:
            assert lookup['query'] is None and lookup['result'] is None
            assert lookup['skip_reason'] == 'BINDING_REJECTED'
            continue
        version = entry['target'] if side == 'after' else entry['installed']['versions'][0]
        assert lookup['query'] == {'repository': version['repository'], 'category': entry['package']['category'], 'name': entry['package']['name'], 'version': version['version']}
        assert lookup['result']['status'] == 'found'
    return 'after_only' if target_only else 'comparison'


def verify(full, plan):
    assert full['status'] == 'completed', full['status']
    assert full['error'] is None
    expected_entries, expected_queries = (41, 80) if plan == 2 else (43, 79)
    assert full['progress']['completed'] == full['progress']['total'] == expected_queries
    result = full['result']
    assert result['plan']['status'] == 'parsed'
    assert len(result['plan']['entries']) == len(result['reports']) == expected_entries
    after_only, rejected = [], []
    for index, report in enumerate(result['reports']):
        assert report['report']['entry_index'] == index
        kind = verify_report(result['plan']['entries'][index], report)
        if kind == 'after_only':
            after_only.append(index)
        if kind == 'binding_rejected':
            rejected.append(index)
    assert len(after_only) == (0 if plan == 2 else 7), after_only
    assert rejected == ([37] if plan == 2 else []), rejected


def postgres_ready():
    identity = subprocess.run(['docker', 'exec', 'cold-postgres', 'cat', '/proc/1/comm'], text=True, capture_output=True)
    if identity.returncode != 0 or identity.stdout.strip() != 'postgres':
        return False
    query = subprocess.run(['docker', 'exec', 'cold-postgres', 'psql', '-U', 'guit', '-d', 'guit', '-tAc', 'SELECT 1'], text=True, capture_output=True)
    return query.returncode == 0 and query.stdout.strip() == '1'


def setup(pair, variant, plan):
    global BASE
    pair.mkdir(exist_ok=False)
    for name in ['pg', 'cache']:
        (pair / name).mkdir()
    docker('run', '-d', '--name', 'cold-postgres', '--network', 'cold-research', '-e', 'POSTGRES_HOST_AUTH_METHOD=trust', '-e', 'POSTGRES_USER=guit', '-e', 'POSTGRES_DB=guit', '-v', f'{pair}/pg:/var/lib/postgresql/data', 'postgres:17-alpine')
    for _ in range(120):
        if postgres_ready():
            break
        time.sleep(1)
    else:
        raise TimeoutError('postgres startup')
    docker('exec', '-i', 'cold-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'guit', '-d', 'guit', input=(ROOT / f'seed-{plan}.sql').read_text())
    docker('run', '-d', '--name', 'cold-worker', '--network', 'cold-research', '--entrypoint', '/usr/local/bin/metadata-worker', '-e', 'METADATA_WORKER_LISTEN_ADDR=:8090', '-e', 'METADATA_WORKER_CACHE_PATH=/metadata-cache', '-e', 'METADATA_WORKER_TEMP_PATH=/var/tmp/gentoo-update-intelligence', '-e', 'METADATA_WORKER_PORTAGE_USER=portage', '-e', 'METADATA_WORKER_PORTAGE_GROUP=portage', '-e', 'METADATA_WORKER_PREPARATION_LIMIT=1', '-e', 'METADATA_WORKER_GENERATION_LIMIT=1', '-v', f'{ROOT}/mirrors/gentoo.git:/metadata/gentoo.git:ro', '-v', f'{ROOT}/mirrors:/overlays:ro', '-v', f'{pair}/cache:/metadata-cache', '-v', f'{ROOT}/{variant}-worker:/usr/local/bin/metadata-worker:ro', 'gentoo-update-intelligence/gentoo-runtime:local')
    docker('run', '-d', '--name', 'cold-backend', '--network', 'cold-research', '-p', '8080:8080', '-e', 'GENTOO_UPDATE_INTELLIGENCE_DATABASE_URL=postgres://guit@cold-postgres:5432/guit?sslmode=disable', '-v', f'{ROOT}/mirrors/gentoo.git:/metadata/gentoo.git:ro', '-v', f'{ROOT}/mirrors:/overlays:ro', '-v', f'{ROOT}/backend-{plan}.yaml:/etc/backend.yaml:ro', '-v', f'{ROOT}/{variant}-backend:/usr/local/bin/gentoo-update-intelligence-server:ro', 'gentoo-update-intelligence-backend:latest', '--config', '/etc/backend.yaml')
    address = docker('inspect', '--format', '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}', 'cold-backend').strip()
    BASE = f'http://{address}:8080/api/v1/analyses'
    for _ in range(120):
        try:
            urllib.request.urlopen(f'http://{address}:8080/', timeout=1)
        except urllib.error.HTTPError as error:
            if error.code == 404:
                break
        except OSError:
            pass
        time.sleep(1)
    else:
        raise TimeoutError('backend startup')


def cleanup():
    # Only these three containers belong to this guest experiment.
    docker('stop', *NAMES)
    docker('rm', *NAMES)


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--resume', action='store_true')
    parser.add_argument('--only', nargs='+', help='run only the named pairs')
    args = parser.parse_args()
    RESULTS.mkdir(exist_ok=True)
    if subprocess.run(['docker', 'network', 'inspect', 'cold-research'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0:
        docker('network', 'create', '--internal', 'cold-research')
    schedule = [(f'plan{plan}-pair{pairnum}-instrumented', plan, 'instrumented') for plan in [2, 3] for pairnum in [1, 2, 3]]
    schedule.append(('plan2-pair4-control', 2, 'control'))
    schedule.append(('plan3-pair2-retry', 3, 'instrumented'))
    if args.only:
        by_label = {label: item for item in schedule for label in [item[0]]}
        unknown = set(args.only) - set(by_label)
        assert not unknown, f'unknown pair labels: {sorted(unknown)}'
        schedule = [by_label[label] for label in args.only]
    else:
        schedule = schedule[:-1]
    for label, plan, variant in schedule:
        pair = ROOT / label
        existing = [RESULTS / (label + '-' + state) for state in ['cold', 'warm']]
        if args.resume and all((p / 'summary.json').exists() and json.loads((p / 'summary.json').read_text())['successful'] for p in existing):
            continue
        if pair.exists():
            assert args.resume, 'existing pair requires --resume'
            mounts = json.loads(docker('inspect', '--format', '{{json .Mounts}}', 'cold-worker'))
            assert any(m['Destination'] == '/metadata-cache' and m['Source'] == str(pair / 'cache') for m in mounts), 'resume cache does not match pair'
        else:
            setup(pair, variant, plan)
        BASE = backend_base()
        for state in ['cold', 'warm']:
            dest = RESULTS / (label + '-' + state)
            if dest.exists():
                assert args.resume and json.loads((dest / 'summary.json').read_text())['successful'], 'failed runs require diagnosis'
                verify(json.loads((dest / 'result.json').read_text()), plan)
                continue
            dest = measure(label + '-' + state, plan)
            (dest / 'container-ids.json').write_text(docker('inspect', '--format', '{{.Name}} {{.Id}}', *NAMES))
        cleanup()
