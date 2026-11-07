"""Small progress view without rereading entire growing observation files."""
import json
import subprocess
from pathlib import Path


def tail_json(path):
    with path.open('rb') as stream:
        stream.seek(0, 2)
        stream.seek(max(0, stream.tell() - 1024 * 1024))
        return json.loads(stream.read().splitlines()[-1])


if __name__ == '__main__':
    out = []
    for path in sorted(Path('/opt/guit/results').iterdir()):
        if (path / 'summary.json').exists():
            summary = json.loads((path / 'summary.json').read_text())
            out.append({key: summary.get(key) for key in ['label', 'successful', 'terminal_s', 'failure']})
        elif (path / 'progress.jsonl').exists():
            last = tail_json(path / 'progress.jsonl')
            response = last['response']
            reports = (response.get('result') or {}).get('reports', [])
            errors = sum(bool(report.get(side, {}).get('error')) for report in reports for side in ['before_lookup', 'after_lookup'])
            out.append({'label': path.name, 'elapsed_s': round(last['elapsed_s'], 1), 'progress': response['progress'], 'lookup_errors_so_far': errors})
    successful = [row for row in out if row.get('successful')]
    running = subprocess.run(['pgrep', '-f', '^/usr/bin/python3 /opt/guit/matrix.py( |$)'], stdout=subprocess.DEVNULL).returncode == 0
    print(json.dumps({'runner_alive': running, 'successful_runs': len(successful), 'last_success': successful[-1] if successful else None, 'active': [row for row in out if 'progress' in row], 'failures': [row for row in out if row.get('successful') is False]}))
