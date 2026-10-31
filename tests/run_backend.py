"""Run Go checks with centrally stored package tests, without copying sources."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / 'backend'
SUITES = ROOT / 'tests/backend'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('test', 'vet', 'list'))
    args, options = parser.parse_known_args()

    # Go package tests need their original package's private identifiers. The
    # build overlay supplies virtual *_test.go files only to this Go invocation;
    # source directories and production builds remain untouched.
    replacements = {}
    for source in sorted(SUITES.rglob('*_test.go')):
        destination = BACKEND / 'internal' / source.relative_to(SUITES)
        if not destination.parent.is_dir():
            parser.error(f'No backend package for {source.relative_to(ROOT)}')
        if destination.exists():
            parser.error(f'Move the duplicate test into tests/: {destination.relative_to(ROOT)}')
        replacements[str(destination)] = str(source)
    if not replacements:
        parser.error('No backend test files found')

    temporary = ROOT / 'local/tmp'
    temporary.mkdir(parents=True, exist_ok=True)
    environment = os.environ.copy()
    defaults = {
        'GOMODCACHE': ROOT / 'local/cache/go-mod',
        'GOCACHE': ROOT / 'local/cache/go-build',
        'GOTMPDIR': temporary,
        'TMPDIR': temporary,
        'TMP': temporary,
        'TEMP': temporary,
    }
    for name, path in defaults.items():
        environment[name] = str(path)
    # macOS commonly aliases /var to /private/var. Tests compare configured
    # directories with canonical filesystem paths, so use one spelling here.
    environment['TMPDIR'] = str(Path(environment['TMPDIR']).resolve())

    with tempfile.TemporaryDirectory(prefix='go-test-overlay-', dir=temporary) as directory:
        overlay = Path(directory) / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': replacements}), encoding='utf-8')
        command = ['go', args.command, f'-overlay={overlay}', *(options or ['./...'])]
        print(f'Go {args.command}: {len(replacements)} test files from tests/backend/', flush=True)
        return subprocess.run(command, cwd=BACKEND, env=environment).returncode


if __name__ == '__main__':
    raise SystemExit(main())
