# Hashi Pulse agent (08 §1): one static binary, nothing else.
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
COPY agents/ agents/
ARG TARGETOS TARGETARCH VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /out/hashi-pulse ./agents/pulse

FROM scratch
COPY --from=go /out/hashi-pulse /hashi-pulse
ENTRYPOINT ["/hashi-pulse"]
LABEL org.opencontainers.image.source=https://git.juzo.io/juzo/hashi \
      org.opencontainers.image.licenses=MIT
