import pathlib
import runpy
import tempfile
import unittest

inspect = runpy.run_path(str(pathlib.Path(__file__).resolve().parents[2] / 'scripts/check-release.py'))['inspect']


class PrivacyCheckTest(unittest.TestCase):
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
            link.symlink_to(path)
            self.assertIn('symlink', inspect(link, 'link.txt')[0])


if __name__ == '__main__':
    unittest.main()
