#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=${1:-dev}
case "$version" in *[!a-zA-Z0-9._-]*|'') echo '版本号只能包含字母、数字、点、下划线和连字符' >&2; exit 1;; esac
mkdir -p local/releases
for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  os=${platform%/*}
  arch=${platform#*/}
  (cd backend && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -tags production \
    -ldflags "-s -w -X main.version=$version" -o "../local/releases/joycurator_${version}_${os}_${arch}" ./cmd/joycurator)
done
python3 - "$version" <<'PY'
import hashlib
import pathlib
import sys
root = pathlib.Path('local/releases')
files = sorted(root.glob(f'joycurator_{sys.argv[1]}_*'))
with (root / 'SHA256SUMS').open('w') as output:
    for file in files:
        digest = hashlib.sha256()
        with file.open('rb') as source:
            for chunk in iter(lambda: source.read(1024 * 1024), b''):
                digest.update(chunk)
        output.write(f'{digest.hexdigest()}  {file.name}\n')
PY
