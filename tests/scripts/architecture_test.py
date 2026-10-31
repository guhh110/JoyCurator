"""Check the four business modules and their infrastructure boundary."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]
INTERNAL = ROOT / 'backend/internal'


def imports(source):
    return re.findall(r'^\s*(?:\w+\s+)?"([\w./-]+)"', source.read_text(), re.M)


class ArchitectureTest(unittest.TestCase):
    def test_development_has_no_compatibility_implementations(self):
        for source in INTERNAL.rglob('*.go'):
            if 'webui/dist' in str(source):
                continue
            self.assertNotRegex(source.stem.lower(), r'compat|migration|legacy')
            self.assertNotRegex(source.read_text(), r'\b(?:MigrateLegacyRules|ParsedMetadata|MetadataFromParsed|type Parsed)\b')

    def test_four_modules_do_not_access_sql_or_http(self):
        for module in ('recognition', 'matching', 'planning', 'organizer'):
            self.assertTrue((INTERNAL / module).is_dir())
            for source in (INTERNAL / module).rglob('*.go'):
                for dependency in imports(source):
                    with self.subTest(source=str(source.relative_to(ROOT)), dependency=dependency):
                        self.assertNotIn(dependency, ('database/sql', 'net/http'))
                        self.assertFalse(dependency.startswith((
                            'joycurator/internal/infra/sqlite',
                            'joycurator/internal/infra/tmdb',
                            'joycurator/internal/app')))

    def test_recognition_and_matching_are_independent_of_io_adapters(self):
        for module in ('recognition', 'matching'):
            for source in (INTERNAL / module).rglob('*.go'):
                self.assertFalse(any(value.startswith('joycurator/internal/infra/')
                                     for value in imports(source)), str(source))
        for source in (INTERNAL / 'recognition').rglob('*.go'):
            self.assertNotIn('joycurator/internal/matching', source.read_text())
        # planning submits through a port; it doesn't import the execution module.
        for source in (INTERNAL / 'planning').glob('*.go'):
            self.assertNotIn('joycurator/internal/organizer', source.read_text())

    def test_http_does_not_bypass_use_cases(self):
        sources = list((INTERNAL / 'app').glob('http_*.go'))
        self.assertGreater(len(sources), 0)
        for source in sources:
            content = source.read_text()
            for dependency in ('database/sql', 'internal/infra/'):
                self.assertNotIn(dependency, content, str(source))
            self.assertNotRegex(content, r'\.(?:Exec|QueryRow|Begin)(?:Context)?\(')

    def test_sql_is_confined_and_old_packages_are_removed(self):
        for old in ('adapters', 'transport', 'store', 'tmdb', 'scanner', 'fsops',
                    'execution', 'extraction', 'identification', 'catalog',
                    'configuration', 'organization', 'library', 'settings'):
            self.assertFalse((INTERNAL / old).exists(), old)
        for source in INTERNAL.rglob('*.go'):
            if 'webui/dist' in str(source):
                continue
            if 'infra/sqlite' not in source.as_posix():
                self.assertNotIn('"database/sql"', source.read_text(), str(source))
        for source in (INTERNAL / 'infra').rglob('*.go'):
            self.assertNotRegex(source.read_text(), r'\b(?:DecideMatch|CompareCandidate|buildPreview|runBatch)\(')


if __name__ == '__main__':
    unittest.main()
