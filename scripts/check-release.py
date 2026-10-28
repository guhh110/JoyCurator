"""Reject private runtime files and common secrets; never print matched values."""
import argparse
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = re.compile(
    r"(?:^|/)(?:local|node_modules|\.ssh|\.aws)(?:/|$)"
    r"|(?:^|/)(?:\.env(?:\..*)?|\.envrc|\.npmrc|\.netrc|credentials\.json|secrets\.json)$"
    r"|\.(?:db(?:-.*)?|sqlite3?(?:-.*)?|sql|log|bak|backup|pem|key|p12|pfx"
    r"|mkv|mp4|avi|mov|srt|ass|zip|tar|gz|7z)$",
    re.IGNORECASE,
)
PATTERNS = {
    "private key": rb"-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----",
    "GitHub token": rb"(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{50,})",
    "AWS access key": rb"(?:AKIA|ASIA)[A-Z0-9]{16}",
    "JWT token": rb"eyJ[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}",
    "credentials in URL": rb"https?://[^\s/\"'<>:]+:[^\s/\"'<>@]+@",
    "personal macOS path": rb"/Users/[A-Za-z0-9_.-]+/",
}


def inspect(path, name, private_paths=True):
    problems = []
    if private_paths and name != '.env.example' and PRIVATE.search(name):
        return [f"{name}: private file must not be published"]
    if path.is_symlink():
        return [f"{name}: symlink is not allowed in release inputs"]
    if not path.is_file():
        return [f"{name}: missing input file"]
    if path.stat().st_size > 10 * 1024 * 1024:
        return [f"{name}: unexpectedly large source or web asset"]
    data = path.read_bytes()
    if data.startswith(b"SQLite format 3\x00"):
        problems.append(f"{name}: SQLite database content")
    for rule, pattern in PATTERNS.items():
        match = re.search(pattern, data)
        if match:
            line = data[:match.start()].count(b'\n') + 1
            problems.append(f"{name}:{line}: {rule}")
    return problems


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assets', action='store_true', help='also check generated frontend')
    args = parser.parse_args()
    tracked = subprocess.check_output(['git', 'ls-files', '-z'], cwd=ROOT).decode().split('\0')
    errors = []
    for name in filter(None, tracked):
        errors.extend(inspect(ROOT / name, name))
    if args.assets:
        directory = ROOT / 'local/build/frontend'
        if not (directory / 'index.html').is_file():
            errors.append('frontend build: index.html missing')
        for path in directory.rglob('*'):
            if path.is_dir():
                continue
            name = path.relative_to(directory).as_posix()
            if name != 'index.html' and not name.startswith('assets/'):
                errors.append(f"frontend build: unexpected root file {name}")
            if path.suffix == '.map':
                errors.append(f"frontend build: source map {name}")
            errors.extend(inspect(path, name))
    for error in errors:
        print(error, file=sys.stderr)
    if errors:
        return 1
    print('Privacy check passed: tracked files' + (' and frontend assets' if args.assets else ''))
    return 0


if __name__ == '__main__':
    sys.exit(main())
