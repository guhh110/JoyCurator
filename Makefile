ROOT := $(abspath .)
export GOMODCACHE := $(ROOT)/local/cache/go-mod
export GOCACHE := $(ROOT)/local/cache/go-build
export GOTMPDIR := $(ROOT)/local/tmp
export TMPDIR := $(ROOT)/local/tmp
export PYTHONDONTWRITEBYTECODE := 1
.PHONY: prepare install backend frontend check test-backend test-frontend test-scripts assets build release
VERSION ?= dev
export VERSION
prepare:
	mkdir -p local/tmp local/cache/go-mod local/cache/go-build local/data local/logs local/source local/target
install: prepare
	cd frontend && pnpm install --frozen-lockfile
	cd backend && go mod download
backend: prepare
	cd backend && go run ./cmd/joycurator
frontend:
	cd frontend && pnpm dev
check: prepare
	python3 -m unittest discover -s tests/scripts -p '*_test.py'
	python3 scripts/check-release.py
	python3 tests/run_backend.py vet ./...
	python3 tests/run_backend.py test -race ./...
	cd frontend && pnpm test && pnpm build
test-backend: prepare
	python3 tests/run_backend.py test -race ./...
test-frontend:
	cd frontend && pnpm test
test-scripts: prepare
	python3 -m unittest discover -s tests/scripts -p '*_test.py'
assets: prepare
	python3 scripts/check-release.py
	cd frontend && pnpm build
	python3 scripts/check-release.py --assets
	rm -rf backend/internal/webui/dist
	mkdir -p backend/internal/webui/dist
	cp -R local/build/frontend/. backend/internal/webui/dist/
build: assets
	mkdir -p local/bin
	cd backend && CGO_ENABLED=0 go build -trimpath -tags production -ldflags "-s -w -X main.version=$$VERSION" -o ../local/bin/joycurator ./cmd/joycurator
release: assets
	sh scripts/release.sh "$$VERSION"
