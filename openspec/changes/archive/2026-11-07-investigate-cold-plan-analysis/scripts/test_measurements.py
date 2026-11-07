"""Checks for evidence processing and the deliberately narrow normalization."""
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from summarize import host_gap_seconds, max_gap_seconds, normalized, observation_failure, observation_gaps, trace
from matrix import verify_report, backend_base, has_metadata_result, postgres_ready


class EvidenceTests(unittest.TestCase):
    def test_normalization_preserves_provenance_content(self):
        value = {
            'id': '6d5f1a34-b786-4076-a030-a84977b4cd0a',
            'sha256': '123456',
            'source_commit': 'abcdef',
            'version': '1.2.3',
            'content': 'metadata-123 is evidence',
            'args': ['/var/tmp/gentoo-update-intelligence/metadata-123/gentoo'],
            'stderr': "path '/var/tmp/gentoo-update-intelligence/metadata-123'",
            'other_path': '/var/tmp/gentoo-update-intelligence/metadata-123-custom/gentoo',
        }
        result = normalized(value)
        self.assertEqual(result['id'], '<analysis-id>')
        self.assertEqual(result['args'], ['/var/tmp/gentoo-update-intelligence/metadata-<temporary>/gentoo'])
        self.assertIn('metadata-<temporary>', result['stderr'])
        for key in ['sha256', 'source_commit', 'version', 'content', 'other_path']:
            self.assertEqual(result[key], value[key])
        self.assertEqual(value['id'], '6d5f1a34-b786-4076-a030-a84977b4cd0a')

    def test_new_slot_install_is_explicitly_unavailable(self):
        # Captured contract of fixture 2 entry 37: this is not a failed lookup.
        entry = {'operation': 'install', 'new_slot': True, 'package': {'category': 'sys-kernel', 'name': 'gentoo-kernel-bin'}}
        skipped = {'query': None, 'result': None, 'error': None, 'skip_reason': 'BINDING_REJECTED'}
        report = {'before_lookup': skipped.copy(), 'after_lookup': skipped.copy(), 'report': {'gentoo': {'status': 'unavailable', 'diagnostics': [{'code': 'OPERATION_UNSUPPORTED'}]}}}
        self.assertEqual(verify_report(entry, report), 'binding_rejected')
        report['after_lookup']['error'] = {'code': 'METADATA_LOOKUP_FAILED'}
        with self.assertRaises(AssertionError):
            verify_report(entry, report)

    def test_resume_targets_container_not_guest_http_server(self):
        with patch('matrix.docker', return_value='172.18.0.4\n'):
            self.assertEqual(backend_base(), 'http://172.18.0.4:8080/api/v1/analyses')
        with patch('matrix.docker', return_value='127.0.0.1\n'):
            with self.assertRaises(AssertionError):
                backend_base()

    def test_unavailable_report_is_not_first_ready_metadata(self):
        unavailable = {'before_lookup': {'result': None}, 'after_lookup': {'result': None}}
        self.assertFalse(has_metadata_result({'reports': [unavailable]}))
        target_only = {'before_lookup': {'result': None}, 'after_lookup': {'result': {'status': 'found'}}}
        self.assertTrue(has_metadata_result({'reports': [unavailable, target_only]}))

    def test_temporary_postgres_is_not_ready_for_seed(self):
        with patch('matrix.subprocess.run', return_value=SimpleNamespace(returncode=0, stdout='docker-entrypoi\n')):
            self.assertFalse(postgres_ready())
        with patch('matrix.subprocess.run', side_effect=[SimpleNamespace(returncode=0, stdout='postgres\n'), SimpleNamespace(returncode=2, stdout='')]):
            self.assertFalse(postgres_ready())
        with patch('matrix.subprocess.run', side_effect=[SimpleNamespace(returncode=0, stdout='postgres\n'), SimpleNamespace(returncode=0, stdout='1\n')]):
            self.assertTrue(postgres_ready())

    def write_log(self, directory, events):
        path = Path(directory) / 'worker.log'
        path.write_text(''.join('COLDTRACE ' + json.dumps(event) + '\n' for event in events))
        return path

    def event(self, ident, stage, kind, mono, duration=0):
        return {'id': ident, 'stage': stage, 'key': 'gentoo/test/package/1', 'event': kind, 'mono_ns': mono, 'duration_ns': duration}

    def test_nested_spans_are_reported_separately(self):
        events = [self.event(1, 'worker.prepare', 'start', 0), self.event(2, 'worker.egencache', 'start', 1), self.event(2, 'worker.egencache', 'end', 4, 3), self.event(1, 'worker.prepare', 'end', 5, 5)]
        with tempfile.TemporaryDirectory() as directory:
            result = trace(self.write_log(directory, events))
        self.assertEqual(result['seconds'], {'worker.prepare': 5e-9, 'worker.egencache': 3e-9})

    def test_unclosed_span_rejects_incomplete_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.write_log(directory, [self.event(1, 'worker.prepare', 'start', 0)])
            with self.assertRaisesRegex(AssertionError, 'unclosed spans'):
                trace(path)

    def test_nonmonotonic_interval_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.write_log(directory, [self.event(1, 'worker.prepare', 'start', 10), self.event(1, 'worker.prepare', 'end', 9)])
            with self.assertRaises(AssertionError):
                trace(path)

    def test_observation_gap_is_detected_on_both_sides_of_run_window(self):
        rows = [{'wall_ns': 0}, {'wall_ns': 20_000_000_000}, {'wall_ns': 21_000_000_000}]
        self.assertEqual(host_gap_seconds(rows, 8_000_000_000, 12_000_000_000), 20)
        self.assertEqual(host_gap_seconds(rows, 20_000_000_000, 21_000_000_000), 1)
        self.assertEqual(max_gap_seconds([{'mono_ns': 1}, {'mono_ns': 1_500_000_001}], 'mono_ns'), 1.5)

    def test_sampling_gap_invalidates_measurement_window(self):
        guest = [{'wall_ns': 5_000_000_000, 'mono_ns': 1_000_000_000}, {'wall_ns': 6_000_000_000, 'mono_ns': 2_000_000_000}]
        host = [{'wall_ns': 0}, {'wall_ns': 20_000_000_000}]
        gaps = observation_gaps(guest, host, 5_000_000_000, 6_000_000_000)
        self.assertEqual(gaps['samples'], 2)
        self.assertIn('exceeds 10s', observation_failure(gaps))


if __name__ == '__main__':
    unittest.main()
