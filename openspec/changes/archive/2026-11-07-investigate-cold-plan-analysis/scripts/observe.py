"""One-second local kernel/cgroup observations; never reads process environments."""
import argparse
import json
import os
from pathlib import Path
import time


def read(path):
    try:
        return Path(path).read_text()
    except (FileNotFoundError, ProcessLookupError, PermissionError):
        return None


def sample(guest):
    now = {'mono_ns': time.monotonic_ns(), 'wall_ns': time.time_ns()}
    for name in ['stat', 'meminfo', 'vmstat', 'diskstats', 'loadavg']:
        now[name] = read('/proc/' + name)
    fs = os.statvfs('/')
    now['free_bytes'] = fs.f_bavail * fs.f_frsize
    now['observer'] = {name: read('/proc/self/' + name) for name in ['stat', 'io', 'status']}
    if guest:
        now['containers'] = {}
        for group in Path('/sys/fs/cgroup/system.slice').glob('docker-*.scope'):
            data = {name: read(group / name) for name in ['cpu.stat', 'memory.current', 'memory.peak', 'memory.stat', 'memory.events', 'io.stat', 'cgroup.procs']}
            data['process_io'] = {pid: read('/proc/' + pid + '/io') for pid in (data['cgroup.procs'] or '').split()}
            now['containers'][group.name] = data
    return now


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('output')
    p.add_argument('--guest', action='store_true')
    p.add_argument('--stop-file', required=True)
    a = p.parse_args()
    deadline = time.monotonic()
    with open(a.output, 'x') as out:
        while not Path(a.stop_file).exists():
            out.write(json.dumps(sample(a.guest)) + '\n')
            out.flush()
            deadline += 1
            time.sleep(max(0, deadline - time.monotonic()))
