"""Inspect representative ELF files exported by Dockerfile.base.

Run inside its build stage with binutils/readelf available. This checks ELF
properties, not vulnerability absence or CPU/kernel enforcement of CET.
"""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys


def inspect(path):
    output = subprocess.check_output(
        ['readelf', '-W', '-h', '-l', '-d', '-s', '-n', str(path)],
        text=True, env={**os.environ, 'LC_ALL': 'C'},
    )
    elf_type = re.search(r'^\s*Type:\s+(\S+)', output, re.M)
    stack = next((line for line in output.splitlines() if 'GNU_STACK' in line), None)
    stack_flags = ''.join(stack.split()[6:-1]) if stack else None
    return {
        'pie': bool(elf_type and elf_type[1] == 'DYN' and re.search(r'^\s*INTERP\s', output, re.M)),
        'nonexec_stack': 'E' not in stack_flags if stack_flags is not None else None,
        'relro': 'GNU_RELRO' in output,
        'bind_now': bool(re.search(r'\(BIND_NOW\)|\(FLAGS\).*BIND_NOW|\(FLAGS_1\).*\bNOW\b', output)),
        'stack_protector_symbol': bool(re.search(r'\b__stack_chk_fail(?:_local)?\b', output)),
        'cet_ibt': bool(re.search(r'x86 feature:.*\bIBT\b', output)),
        'cet_shstk': bool(re.search(r'x86 feature:.*\bSHSTK\b', output)),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path('/runtime'))
    parser.add_argument('--require-basic', action='store_true',
                        help='Require PIE, non-executable GNU_STACK and GNU_RELRO')
    args = parser.parse_args()
    if not args.root.is_absolute() or not args.root.is_dir():
        raise ValueError('Expected exported rootfs as an existing absolute directory')
    targets = {name: '/usr/bin/' + name
               for name in ('git', 'bash', 'wget', 'cp', 'grep', 'sed', 'find', 'tar', 'gawk')}
    targets['python'] = sys.executable
    report = {}
    for name, path in targets.items():
        # The rootfs was exported from this build stage; resolve its installed
        # symlink layout here, then inspect the copied file, not the source ELF.
        installed_path = os.path.realpath(path)
        exported = args.root / installed_path.lstrip('/')
        if not exported.is_file():
            raise ValueError(f'Missing exported ELF for {name}: {exported}')
        report[name] = {'path': installed_path, **inspect(exported)}
    print(json.dumps(report, indent=2, sort_keys=True))
    if args.require_basic:
        missing = [name for name, result in report.items()
                   if not all(result[key] is True for key in ('pie', 'nonexec_stack', 'relro'))]
        if missing:
            raise SystemExit('Missing basic ELF properties: ' + ', '.join(missing))


if __name__ == '__main__':
    main()
