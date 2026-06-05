# syntax=docker/dockerfile:1.7

FROM mcr.microsoft.com/dotnet/sdk:10.0-alpine AS build
WORKDIR /src

# Install build dependencies for Native AOT
RUN apk add --no-cache clang musl-dev build-base zlib-dev

# Copy project files and restore
COPY Hashi.Pulse.csproj ./
RUN dotnet restore

# Copy source code and publish
COPY . ./
RUN dotnet publish -c Release -o /out /p:PublishAot=true

# Final runtime image
FROM mcr.microsoft.com/dotnet/runtime-deps:10.0-alpine
COPY --from=build /out/Hashi.Pulse /usr/local/bin/hashi-pulse
USER app
ENTRYPOINT ["/usr/local/bin/hashi-pulse"]
