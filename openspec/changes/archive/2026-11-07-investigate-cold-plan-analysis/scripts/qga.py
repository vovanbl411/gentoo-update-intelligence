"""Local QEMU guest-agent command transport; no SSH credentials required."""
import argparse
import base64
import json
import socket
import time


def call(path, command, arguments=None):
    with socket.socket(socket.AF_UNIX) as sock:
        sock.settimeout(15)
        sock.connect(path)
        stream = sock.makefile('rwb')
        stream.write((json.dumps({'execute': command, 'arguments': arguments or {}}) + '\n').encode())
        stream.flush()
        reply = json.loads(stream.readline())
        if 'error' in reply:
            raise RuntimeError(reply['error'])
        return reply['return']


def execute(path, args):
    pid = call(path, 'guest-exec', {'path': args[0], 'arg': args[1:], 'capture-output': True})['pid']
    while True:
        result = call(path, 'guest-exec-status', {'pid': pid})
        if result.get('exited'):
            return result
        time.sleep(0.25)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--socket', default='/var/tmp/guit-cold-plan-analysis/agent.sock')
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    result = execute(args.socket, args.command)
    for name in ['out-data', 'err-data']:
        if name in result:
            import sys
            target = sys.stdout.buffer if name == 'out-data' else sys.stderr.buffer
            target.write(base64.b64decode(result[name]))
    raise SystemExit(result.get('exitcode', 1))
