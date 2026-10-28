# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
RUN npm install --global pnpm@11.28.5
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/local/build/frontend/ ./internal/webui/dist/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags production \
    -ldflags "-s -w -X main.version=$VERSION" -o /out/joycurator ./cmd/joycurator

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app /data /source /target \
    && chown -R 1000:1000 /app /data /source /target
WORKDIR /app
COPY --from=backend /out/joycurator /app/joycurator
ENV JOYCURATOR_ADDR=0.0.0.0:8081 \
    JOYCURATOR_DATA_DIR=/data \
    JOYCURATOR_SOURCE_DIR=/source \
    JOYCURATOR_TARGET_DIR=/target
USER 1000:1000
EXPOSE 8081
ENTRYPOINT ["/app/joycurator"]
