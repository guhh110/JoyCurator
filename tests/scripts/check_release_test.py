import pathlib
import os
import runpy
import tempfile
import unittest
import subprocess

checker = runpy.run_path(str(pathlib.Path(__file__).resolve().parents[2] / 'scripts/check-release.py'))
inspect = checker['inspect']
source_files = checker['source_files']


class PrivacyCheckTest(unittest.TestCase):
    def test_current_sources_include_new_files_and_exclude_deleted_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            subprocess.run(['git', 'init', '-q', directory], check=True)
            (root / 'before.go').write_text('package example')
            subprocess.run(['git', 'add', 'before.go'], cwd=root, check=True)
            (root / 'before.go').unlink()
            (root / 'after.go').write_text('package example')
            self.assertEqual(source_files(root), ['after.go'])

    def test_private_file_paths(self):
        for name in ('local/data/app.db', 'backend/test.sqlite3-wal',
                     'frontend/.env.production', 'private.key', 'credentials.json'):
            with self.subTest(name=name):
                self.assertIn('private file', inspect(pathlib.Path(name), name)[0])

    def test_disguised_database(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'data.txt'
            path.write_bytes(b'SQLite format 3\x00' + b'\0' * 100)
            self.assertIn('SQLite database', inspect(path, 'data.txt')[0])

    def test_secret_is_reported_without_printing_value(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'config.ts'
            secret = 'gh' + 'p_' + 'a' * 36
            path.write_text(secret)
            result = '\n'.join(inspect(path, 'config.ts'))
            self.assertIn('GitHub token', result)
            self.assertNotIn(secret, result)

    def test_safe_fixture_and_symlink(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'fixture.txt'
            path.write_text('fixture-password-2026')
            self.assertEqual(inspect(path, 'fixture.txt'), [])
            link = path.with_name('link.txt')
            try:
                link.symlink_to(path)
            except OSError as exc:
                if os.name == 'nt' and getattr(exc, 'winerror', None) == 1314:
                    self.skipTest('Windows symlink privilege unavailable')
                raise
            self.assertIn('symlink', inspect(link, 'link.txt')[0])


if __name__ == '__main__':
    unittest.main()
