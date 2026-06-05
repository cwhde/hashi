# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:10.0-alpine AS build
WORKDIR /src

# Install build dependencies for Native AOT
RUN apk add --no-cache clang musl-dev build-base zlib-dev

ARG TARGETARCH
# Maps TARGETARCH (amd64/arm64) to .NET RIDs: linux-x64 and linux-arm64 (musl variations)
RUN if [ "$TARGETARCH" = "amd64" ]; then \
      RID="linux-musl-x64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
      RID="linux-musl-arm64"; \
    else \
      RID="linux-musl-x64"; \
    fi && \
    echo "RID=$RID" > /tmp/rid

# Copy project files and restore
COPY Hashi.Pulse.csproj ./
RUN --mount=type=cache,target=/root/.nuget/packages \
    . /tmp/rid && \
    dotnet restore -r $RID

# Copy source code and publish
COPY . ./
RUN --mount=type=cache,target=/root/.nuget/packages \
    . /tmp/rid && \
    dotnet publish -c Release -r $RID --no-restore -o /out /p:PublishAot=true

# Final runtime image
FROM mcr.microsoft.com/dotnet/runtime-deps:10.0-alpine
COPY --from=build /out/Hashi.Pulse /usr/local/bin/hashi-pulse
USER app
ENTRYPOINT ["/usr/local/bin/hashi-pulse"]
