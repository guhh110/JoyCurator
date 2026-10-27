ROOT := $(abspath .)
export GOMODCACHE := $(ROOT)/local/cache/go-mod
export GOCACHE := $(ROOT)/local/cache/go-build
export GOTMPDIR := $(ROOT)/local/tmp
export TMPDIR := $(ROOT)/local/tmp
.PHONY: prepare install backend frontend dev-backend dev-frontend check build
prepare:
	mkdir -p local/tmp local/cache/go-mod local/cache/go-build local/data local/logs local/media/inbox local/media/movies local/media/tv
install: prepare
	cd frontend && pnpm install --frozen-lockfile
	cd backend && go mod download
dev-backend: backend
dev-frontend: frontend
backend: prepare
	cd backend && go run ./cmd/joycurator
frontend:
	cd frontend && pnpm dev
check: prepare
	cd backend && go vet ./... && go test -race ./...
	cd frontend && pnpm test && pnpm build
build: prepare
	cd frontend && pnpm build
	mkdir -p local/bin
	cd backend && go build -o ../local/bin/joycurator ./cmd/joycurator
