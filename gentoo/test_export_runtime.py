"""Dependency-selection fault tests; run with the pinned builder's Python."""

import importlib.util
from pathlib import Path
import unittest

from portage.dep import use_reduce

spec = importlib.util.spec_from_file_location(
    'export_runtime', Path(__file__).with_name('export-runtime.py')
)
runtime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runtime)


class Installed:
    def __init__(self, packages):
        self.packages = packages

    def match(self, atom):
        return [package for package in self.packages
                if package.rsplit('-', 1)[0] == atom.cp]

    def aux_get(self, package, keys):
        return self.packages[package]


class DependenciesTest(unittest.TestCase):
    def test_missing_mandatory_dependency_fails(self):
        with self.assertRaisesRegex(ValueError, 'not installed'):
            runtime.dependencies('dev-libs/missing', Installed({}))

    def test_alternative_requires_the_whole_branch(self):
        db = Installed({'dev-libs/first-1': (), 'dev-libs/fallback-1': ()})
        expression = use_reduce(
            '|| ( ( dev-libs/first dev-libs/missing ) dev-libs/fallback )',
            opconvert=True,
        )
        self.assertEqual(runtime.dependencies(expression, db), {'dev-libs/fallback-1'})

    def test_blocker_is_not_copied(self):
        self.assertEqual(runtime.dependencies('!dev-libs/blocked', Installed({})), set())

    def test_use_filtering_and_dependency_cycle(self):
        db = Installed({
            'dev-libs/first-1': ('enabled', '8',
                                 'enabled? ( dev-libs/second ) disabled? ( dev-libs/missing )', ''),
            'dev-libs/second-1': ('', '8', '', 'dev-libs/first'),
        })
        self.assertEqual(runtime.runtime_packages(['dev-libs/first'], db), set(db.packages))


if __name__ == '__main__':
    unittest.main()
