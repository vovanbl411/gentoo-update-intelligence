"""Summarize preserved observations; nested spans remain separate from wall time."""
import argparse
import collections
import hashlib
import json
from pathlib import Path
import re
import statistics


def fields(text):
    return {row.split()[0].rstrip(':'): int(row.split()[1]) for row in (text or '').splitlines() if len(row.split()) >= 2 and row.split()[1].isdigit()}


def disk(row):
    return next(list(map(int, line.split()[3:])) for line in row['diskstats'].splitlines() if line.split()[2] == 'vda')


def max_gap_seconds(rows, field):
    points = [int(row[field]) for row in rows]
    return max(((end - start) / 1e9 for start, end in zip(points, points[1:])), default=0)


def host_gap_seconds(rows, start_ns, end_ns):
    points = sorted(int(row['wall_ns']) for row in rows)
    return max(((end - start) / 1e9 for start, end in zip(points, points[1:]) if start < end_ns and end > start_ns), default=0)


def observation_gaps(rows, host, start_ns, end_ns):
    return {
        'samples': len(rows),
        'guest_wall_gap_max_s': max_gap_seconds(rows, 'wall_ns'),
        'guest_monotonic_gap_max_s': max_gap_seconds(rows, 'mono_ns'),
        'host_wall_gap_max_s': host_gap_seconds(host, start_ns, end_ns),
    }


def observation_failure(sampling, threshold_s=10):
    if max(sampling['guest_wall_gap_max_s'], sampling['guest_monotonic_gap_max_s'], sampling['host_wall_gap_max_s']) <= threshold_s:
        return None
    return (
        f'observation gap exceeds {threshold_s}s '
        f'(guest wall={sampling["guest_wall_gap_max_s"]:.1f}s, '
        f'guest monotonic={sampling["guest_monotonic_gap_max_s"]:.1f}s, '
        f'host wall={sampling["host_wall_gap_max_s"]:.1f}s)'
    )


def trace(path):
    starts = {}
    durations = collections.defaultdict(float)
    counts = collections.Counter()
    text = path.read_text()
    for line in text.splitlines():
        if 'COLDTRACE ' not in line:
            continue
        event = json.loads(line.split('COLDTRACE ', 1)[1])
        if event['event'] == 'start':
            starts[event['id']] = event
        else:
            begin = starts.pop(event['id'])
            assert event['mono_ns'] >= begin['mono_ns']
            stage = event['stage']
            if stage == 'worker.checkout':
                stage += '.master' if '/master-' in event['key'] else '.main'
            durations[stage] += event['duration_ns'] / 1e9
            counts[stage] += 1
    assert not starts, f'unclosed spans: {path}: {starts}'
    return {'seconds': dict(durations), 'counts': dict(counts), 'generated': text.count('metadata generated repository='), 'cache_hits': text.count('metadata cache hit repository=')}


def normalized(value):
    if isinstance(value, dict):
        return {key: ('<analysis-id>' if key == 'id' and isinstance(item, str) and re.fullmatch(r'[0-9a-f-]{36}', item) else normalized(item)) for key, item in value.items()}
    if isinstance(value, list):
        return [normalized(item) for item in value]
    if isinstance(value, str):
        return re.sub(r"/var/tmp/gentoo-update-intelligence/metadata-[0-9]+(?=/|$|[\s'\"])", '/var/tmp/gentoo-update-intelligence/metadata-<temporary>', value)
    return value


def summarize(path, host):
    summary = json.loads((path / 'summary.json').read_text())
    guest_log = path / 'guest.jsonl'
    rows = [json.loads(line) for line in guest_log.read_text().splitlines()] if guest_log.exists() else []
    summary['sampling'] = observation_gaps(rows, host, summary['wall_start_ns'], summary['wall_end_ns'])
    quality_failure = observation_failure(summary['sampling'])
    if quality_failure:
        summary['measurement_quality_failure'] = quality_failure
    if not summary['successful']:
        return summary
    if 'measurement_quality_failure' in summary:
        summary['successful'] = False
        summary['failure'] = summary['measurement_quality_failure']
        return summary
    first, last = rows[0], rows[-1]
    metrics = {'cpu_user_s': 0, 'cpu_system_s': 0, 'cgroup_read_bytes': 0, 'cgroup_write_bytes': 0, 'logical_wchar_sampled_lower_bound': 0}
    components = {}
    names = {line.split()[1]: line.split()[0].lstrip('/') for line in (path / 'container-ids.json').read_text().splitlines()}
    for name in first['containers']:
        a, b = first['containers'][name], last['containers'][name]
        ca, cb = fields(a['cpu.stat']), fields(b['cpu.stat'])
        metrics['cpu_user_s'] += (cb['user_usec'] - ca['user_usec']) / 1e6
        metrics['cpu_system_s'] += (cb['system_usec'] - ca['system_usec']) / 1e6
        component = names.get(name.removeprefix('docker-').removesuffix('.scope'), name)
        components[component] = {
            'cpu_user_s': (cb['user_usec'] - ca['user_usec']) / 1e6,
            'cpu_system_s': (cb['system_usec'] - ca['system_usec']) / 1e6,
            'sampled_memory_peak_bytes': max(int(row['containers'][name]['memory.current']) for row in rows),
            'sampled_anon_peak_bytes': max(fields(row['containers'][name]['memory.stat']).get('anon', 0) for row in rows),
            'sampled_file_peak_bytes': max(fields(row['containers'][name]['memory.stat']).get('file', 0) for row in rows),
        }
        ea, eb = fields(a['memory.events']), fields(b['memory.events'])
        for event in ['oom', 'oom_kill']:
            assert eb.get(event, 0) == ea.get(event, 0) == 0, 'OOM invalidates run'
        for field, metric in [('rbytes', 'cgroup_read_bytes'), ('wbytes', 'cgroup_write_bytes')]:
            def total(value):
                return sum(int(word.split('=')[1]) for line in (value or '').splitlines() for word in line.split()[1:] if word.startswith(field + '='))
            metrics[metric] += total(b['io.stat']) - total(a['io.stat'])
            components[component][metric] = total(b['io.stat']) - total(a['io.stat'])
        prior = {pid: fields(value).get('wchar', 0) for pid, value in a['process_io'].items()}
        for row in rows[1:]:
            current = row['containers'].get(name)
            assert current is not None, 'container disappeared during measurement'
            for pid, value in current['process_io'].items():
                if value is None:
                    continue
                count = fields(value).get('wchar', 0)
                metrics['logical_wchar_sampled_lower_bound'] += max(0, count - prior.get(pid, 0))
                prior[pid] = count
    metrics['sampled_memory_peak_bytes'] = max(sum(int(c['memory.current']) for c in row['containers'].values()) for row in rows)
    for field in ['anon', 'file']:
        metrics[f'sampled_{field}_peak_bytes'] = max(sum(fields(c['memory.stat']).get(field, 0) for c in row['containers'].values()) for row in rows)
    metrics['guest_min_available_bytes'] = min(fields(row['meminfo'])['MemAvailable'] * 1024 for row in rows)
    metrics['min_free_bytes'] = min(row['free_bytes'] for row in rows)
    da, db = disk(first), disk(last)
    metrics['device_read_bytes'] = (db[2] - da[2]) * 512
    metrics['device_write_bytes'] = (db[6] - da[6]) * 512
    interval_s = (last['mono_ns'] - first['mono_ns']) / 1e9
    metrics['device_busy_fraction'] = (db[9] - da[9]) / 1000 / interval_s
    metrics['device_mean_read_ms'] = (db[3] - da[3]) / max(1, db[0] - da[0])
    metrics['device_mean_write_ms'] = (db[7] - da[7]) / max(1, db[4] - da[4])
    cpu_a = list(map(int, first['stat'].splitlines()[0].split()[1:]))
    cpu_b = list(map(int, last['stat'].splitlines()[0].split()[1:]))
    metrics['guest_iowait_s'] = (cpu_b[4] - cpu_a[4]) / 100
    metrics['guest_steal_s'] = (cpu_b[7] - cpu_a[7]) / 100
    metrics['guest_major_faults'] = fields(last['vmstat'])['pgmajfault'] - fields(first['vmstat'])['pgmajfault']
    metrics['samples'] = len(rows)
    metrics['observer_sampled_rss_peak_bytes'] = max(fields(row['observer']['status']).get('VmRSS', 0) * 1024 for row in rows)
    metrics['observer_logical_wchar'] = fields(last['observer']['io'])['wchar'] - fields(first['observer']['io'])['wchar']
    metrics['observer_write_bytes'] = fields(last['observer']['io'])['write_bytes'] - fields(first['observer']['io'])['write_bytes']
    for field, index in [('user', 13), ('system', 14)]:
        # Guest Debian amd64 uses USER_HZ=100, recorded separately in guest versions.
        metrics[f'observer_cpu_{field}_s'] = (int(last['observer']['stat'].split()[index]) - int(first['observer']['stat'].split()[index])) / 100
    host_rows = [r for r in host if summary['wall_start_ns'] <= r['wall_ns'] <= summary['wall_end_ns']]
    if host_rows:
        va, vb = fields(host_rows[0]['vmstat']), fields(host_rows[-1]['vmstat'])
        metrics['host_swap_in_pages'] = vb['pswpin'] - va['pswpin']
        metrics['host_swap_out_pages'] = vb['pswpout'] - va['pswpout']
        metrics['host_load1_max'] = max(float(r['loadavg'].split()[0]) for r in host_rows)
    for line in (path / 'containers.json').read_text().splitlines():
        assert not json.loads(line)['OOMKilled'], 'container OOM invalidates run'
    summary['metrics'] = metrics
    summary['components'] = components
    summary['worker'] = trace(path / 'cold-worker.log')
    summary['backend'] = trace(path / 'cold-backend.log')
    full = json.loads((path / 'result.json').read_text())
    content = json.dumps(normalized(full), sort_keys=True, ensure_ascii=False, separators=(',', ':')).encode()
    summary['normalized_sha256'] = hashlib.sha256(content).hexdigest()
    return summary


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('results', type=Path)
    parser.add_argument('host', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    host = [json.loads(line) for line in args.host.read_text().splitlines()]
    runs = [summarize(path, host) for path in sorted(args.results.iterdir()) if (path / 'summary.json').exists()]
    groups = {}
    for run in runs:
        if not run['successful']:
            continue
        label = run['label']
        key = f"plan{run['plan']}-" + ('control-' if '-control-' in label else 'instrumented-') + label.rsplit('-', 1)[1]
        groups.setdefault(key, []).append(run)
    aggregate = {}
    for key, group in groups.items():
        aggregate[key] = {'n': len(group)}
        for field in ['terminal_s', 'first_partial_s', 'first_metadata_s']:
            values = [run[field] for run in group]
            aggregate[key][field] = {'median': statistics.median(values), 'min': min(values), 'max': max(values)}
        for field in group[0]['metrics']:
            values = [run['metrics'][field] for run in group]
            aggregate[key][field] = {'median': statistics.median(values), 'min': min(values), 'max': max(values)}
    by_plan = collections.defaultdict(set)
    for run in runs:
        if run['successful']:
            by_plan[run['plan']].add(run['normalized_sha256'])
    for plan, hashes in by_plan.items():
        assert len(hashes) == 1, f'normalized result differs for plan {plan}: {hashes}'
    args.output.write_text(json.dumps({'runs': runs, 'aggregate': aggregate, 'normalized_equal': {str(k): len(v) == 1 for k, v in by_plan.items()}}, indent=2) + '\n')
