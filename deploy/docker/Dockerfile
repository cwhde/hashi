# syntax=docker/dockerfile:1

FROM node:24-bookworm AS web-build
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile || pnpm install --no-frozen-lockfile
COPY web/ ./
RUN pnpm run build

FROM mcr.microsoft.com/dotnet/sdk:10.0-preview AS dotnet-build
WORKDIR /src
COPY Directory.Build.props Hashi.slnx ./
COPY src/ ./src/
COPY agents/ ./agents/
RUN dotnet restore src/Hashi.Api/Hashi.Api.csproj
COPY --from=web-build /src/web/build ./web/build
RUN dotnet publish src/Hashi.Api/Hashi.Api.csproj -c Release -o /app/publish /p:SkipFrontendBuild=true

FROM mcr.microsoft.com/dotnet/aspnet:10.0-preview AS final
WORKDIR /app
RUN apt-get update \
  && apt-get upgrade -y \
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
