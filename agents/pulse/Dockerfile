FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/hashi-pulse ./cmd/hashi-pulse

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hashi-pulse /usr/local/bin/hashi-pulse
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/hashi-pulse"]
