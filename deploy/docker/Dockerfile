# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:24-alpine AS web-build
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@9.15.9 --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=hashi-pnpm-store,target=/pnpm/store \
  pnpm config set store-dir /pnpm/store \
  && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:10.0 AS dotnet-build
WORKDIR /src
ARG TARGETARCH
COPY Directory.Build.props Hashi.slnx ./
COPY src/Hashi.Api/Hashi.Api.csproj ./src/Hashi.Api/
COPY src/Hashi.Core/Hashi.Core.csproj ./src/Hashi.Core/
COPY src/Hashi.Contracts/Hashi.Contracts.csproj ./src/Hashi.Contracts/
COPY src/Hashi.Infrastructure/Hashi.Infrastructure.csproj ./src/Hashi.Infrastructure/
RUN --mount=type=cache,id=hashi-nuget,target=/root/.nuget/packages,sharing=locked \
  target_rid="" \
  && case "${TARGETARCH:-}" in \
    amd64) target_rid="linux-x64" ;; \
    arm64) target_rid="linux-arm64" ;; \
    "") target_rid="" ;; \
    *) target_rid="linux-${TARGETARCH}" ;; \
  esac \
  && if [ -n "$target_rid" ]; then \
    dotnet restore src/Hashi.Api/Hashi.Api.csproj -r "$target_rid"; \
  else \
    dotnet restore src/Hashi.Api/Hashi.Api.csproj; \
  fi
COPY src/ ./src/
COPY --from=web-build /src/web/build ./web/build
RUN --mount=type=cache,id=hashi-nuget,target=/root/.nuget/packages,sharing=locked \
  target_rid="" \
  && case "${TARGETARCH:-}" in \
    amd64) target_rid="linux-x64" ;; \
    arm64) target_rid="linux-arm64" ;; \
    "") target_rid="" ;; \
    *) target_rid="linux-${TARGETARCH}" ;; \
  esac \
  && if [ -n "$target_rid" ]; then \
    dotnet publish src/Hashi.Api/Hashi.Api.csproj -c Release -r "$target_rid" --self-contained false -o /app/publish /p:SkipFrontendBuild=true; \
  else \
    dotnet publish src/Hashi.Api/Hashi.Api.csproj -c Release -o /app/publish /p:SkipFrontendBuild=true; \
  fi

FROM mcr.microsoft.com/dotnet/aspnet:10.0 AS final
WORKDIR /app
RUN apt-get update \
  && apt-get install -y --no-install-recommends curl \
  && rm -rf /var/lib/apt/lists/*
COPY --from=dotnet-build /app/publish .
ENV Hashi__Ports__Admin=8080
ENV Hashi__Ports__PublicDashboard=8081
ENV Hashi__Ports__PublicStatus=8082
EXPOSE 8080 8081 8082
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${Hashi__Ports__Admin:-8080}/api/health || exit 1
ENTRYPOINT ["dotnet", "Hashi.Api.dll"]
