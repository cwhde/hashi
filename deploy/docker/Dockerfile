# syntax=docker/dockerfile:1.7
# Multi-arch without emulation: both build stages run on $BUILDPLATFORM; Go cross-compiles (pure-Go SQLite, no cgo).

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
RUN npm install -g pnpm@12.15.0
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY api/openapi.yaml /src/api/openapi.yaml
COPY web/ ./
RUN mkdir -p /src/internal/web/dist && pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY agents/ agents/
COPY internal/ internal/
COPY --from=web /src/internal/web/dist/ internal/web/dist/
ARG TARGETOS TARGETARCH VERSION=dev COMMIT=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X git.juzo.io/juzo/hashi/internal/app.Version=${VERSION} -X git.juzo.io/juzo/hashi/internal/app.Commit=${COMMIT}" \
      -o /out/hashi ./cmd/hashi \
 && for a in amd64 arm64; do CGO_ENABLED=0 GOOS=linux GOARCH=$a go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /out/pulse/hashi-pulse-linux-$a ./agents/pulse; done \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=go /out/hashi /usr/local/bin/hashi
COPY --from=go /out/pulse/ /usr/local/share/hashi/pulse/
COPY --from=go --chown=65532:65532 /out/data /data
VOLUME /data
EXPOSE 8080 8081 8082
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD ["/usr/local/bin/hashi", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/hashi"]
LABEL org.opencontainers.image.source=https://git.juzo.io/juzo/hashi \
      org.opencontainers.image.licenses=MIT
