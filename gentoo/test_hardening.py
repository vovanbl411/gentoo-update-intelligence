"""Exercise ELF inspection on real positive/negative compiler fixtures.

Run in a build-stage image with GCC and readelf. Fixtures are never executed.
"""

import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest


spec = importlib.util.spec_from_file_location(
    'hardening', Path(__file__).with_name('check-hardening.py')
)
hardening = importlib.util.module_from_spec(spec)
spec.loader.exec_module(hardening)


class HardeningTest(unittest.TestCase):
    def compile_fixture(self, flags):
        directory = tempfile.TemporaryDirectory(prefix='guit-elf-test-')
        self.addCleanup(directory.cleanup)
        source = Path(directory.name) / 'fixture.c'
        binary = Path(directory.name) / 'fixture'
        source.write_text(
            'int main(int argc, char **argv) { '
            'volatile char buffer[32]; buffer[0] = (char)argc; '
            'return buffer[0] == 42; }\n'
        )
        subprocess.run(['gcc', str(source), '-o', str(binary), *flags], check=True)
        return hardening.inspect(binary)

    def test_explicit_protections_are_detected(self):
        result = self.compile_fixture([
            '-fPIE', '-pie', '-fstack-protector-all',
            '-Wl,-z,relro,-z,now,-z,noexecstack',
        ])
        for field in ('pie', 'nonexec_stack', 'relro', 'bind_now', 'stack_protector_symbol'):
            self.assertIs(result[field], True, field)

    def test_explicitly_weakened_elf_is_detected(self):
        result = self.compile_fixture([
            '-fno-pie', '-no-pie', '-fno-stack-protector',
            '-Wl,-z,norelro,-z,lazy,-z,execstack',
        ])
        for field in ('pie', 'nonexec_stack', 'relro', 'bind_now', 'stack_protector_symbol'):
            self.assertIs(result[field], False, field)


if __name__ == '__main__':
    unittest.main()
