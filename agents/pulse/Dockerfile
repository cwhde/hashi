# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
WORKDIR /src
ARG TARGETOS=linux
ARG TARGETARCH
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
  go mod download
COPY cmd/ ./cmd/
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  target_os="${TARGETOS:-linux}" \
  && target_arch="${TARGETARCH:-$(go env GOARCH)}" \
  && CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags="-s -w" -o /out/hashi-pulse ./cmd/hashi-pulse

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hashi-pulse /usr/local/bin/hashi-pulse
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/hashi-pulse"]
