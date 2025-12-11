# Hashi (橋)

A lightweight homelab orchestration platform that bridges Pangolin network management, Hetzner DNS, and Gatus status monitoring with a unified web interface.

## What is Hashi?

**Hashi** (橋) means "bridge" in Japanese. It automatically discovers your network topology, syncs DNS records with Hetzner, and generates status monitoring configurations for Gatus—bridging the gap between network management, DNS automation, and uptime monitoring.

The platform continuously:
1. **Discovers** your network topology from DNS records
2. **Syncs** with Pangolin to map resources to infrastructure
3. **Updates** Hetzner DNS with discovered endpoints
4. **Generates** Gatus monitoring configs for health checks

All managed through a modern web dashboard with live log streaming and configuration management.

## Why "Hashi"?

The name reflects the system's core purpose: acting as a bridge between three different platforms (Pangolin, Hetzner DNS, and Gatus), unifying them into a single cohesive orchestration layer for homelab infrastructure.

## Features

- **Web Dashboard**: Modern dark-themed UI for monitoring and management
- **Live Logs**: WebSocket-based real-time log streaming
- **Configuration Editor**: Web-based config editing with backup/restore
- **Automatic Sync**: Configurable sync intervals with manual trigger option
- **Secure Authentication**: bcrypt password hashing with session management

## Technology Stack

- **Runtime**: Node.js 20 LTS
- **Framework**: Fastify
- **Frontend**: Vanilla JS + HTMX-style patterns
- **Styling**: Custom dark theme
- **DNS Resolution**: dns-packet over UDP to Quad9 (9.9.9.9)

## Quick Start

### Using Docker Compose (Recommended)

1. Create your `config.yml` in the same directory (see Configuration section)
2. Run:
   ```bash
   docker-compose up -d
   ```
3. Access the web interface at `http://localhost:3000`

The image is automatically pulled from `git.juzo.io/juzo/hashi:latest`.

### Using Docker directly

```bash
docker run -d \
  --name hashi \
  -p 3000:3000 \
  -v $(pwd)/config.yml:/app/config.yml:ro \
  -v $(pwd)/logs:/app/logs \
  git.juzo.io/juzo/hashi:latest
```

### Building locally (Development)

If you want to build the image locally:

```bash
docker build -t hashi:local -f docker/Dockerfile .
docker run -d --name hashi -p 3000:3000 -v $(pwd)/config.yml:/app/config.yml:ro hashi:local
```

### Manual Installation

1. Install dependencies:
   ```bash
   cd hashi
   npm install
   ```

2. Start the server:
   ```bash
   npm start
   ```

3. Access the web interface at `http://localhost:3000`

## First Run

On first access, you'll be prompted to create an admin account. This will add an `auth` section to your `config.yml`:

```yaml
auth:
  username: "admin"
  password_hash: "$2b$12$..."
```

## Configuration

The application uses the same `config.yml` format as `script.py` with an additional `auth` section:

```yaml
auth:
  username: "admin"
  password_hash: "$2b$12$..."  # bcrypt hash

general:
  domain: "example.com"
  topology_source: "hosts.example.com"
  topology_cache_path: "/app/logs/topology-cache.json"
  resolver_ip: "9.9.9.9"
  gatus_output_path: "/gatus/endpoints.yaml"
  loop_interval: 300
  name_overrides: {}
  keep_subdomains: []
  ignore_subdomains: []

apis:
  pangolin:
    base_url: "https://pangolin.example.com/api/v1"
    auth_token: "xxx"
    org_id: ""
  hetzner:
    auth_token: "xxx"
    zone_id: ""

gatus_defaults:
  interval: "5m"
  allowed_http_codes: [200]
  subdomain_http_codes: {}
  subdomain_port_overrides: {}
  skip_technical_cnames: true
  aggressive_host_filtering: false
  client:
    timeout: "10s"
```

## API Endpoints

### Authentication
- `GET /auth/status` - Get auth status
- `POST /auth/register` - Register new user (first run only)
- `POST /auth/login` - Login
- `POST /auth/logout` - Logout

### Sync Operations
- `GET /api/status` - Get sync status
- `POST /api/sync` - Trigger manual sync
- `GET /api/logs` - Get logs
- `GET /api/history` - Get sync history
- `GET /api/history/:runId` - Get logs for specific run

### Configuration
- `GET /api/config` - Get configuration (tokens masked)
- `PUT /api/config` - Update configuration
- `POST /api/config/restore` - Restore from backup
- `POST /api/config/test` - Test API tokens

### WebSocket
- `ws://.../ws/logs` - Live log streaming

## Security

- Passwords are hashed with bcrypt (cost factor 12)
- Sessions are stored in-memory (cleared on restart)
- HTTP-only cookies with SameSite=Strict
- Sensitive config values are masked in API responses

## Directory Structure

```
hashi/
├── package.json
├── server.js
├── docker/
│   ├── Dockerfile
│   └── .dockerignore
├── .gitea/
│   └── workflows/
│       └── docker-build.yml
├── src/
│   ├── core/           # Business logic (ported from script.py)
│   │   ├── config.js
│   │   ├── topology.js
│   │   ├── pangolin.js
│   │   ├── hetzner.js
│   │   ├── gatus.js
│   │   └── sync.js
│   ├── routes/
│   │   ├── auth.js
│   │   ├── api.js
│   │   └── websocket.js
│   ├── services/
│   │   ├── auth.js
│   │   ├── scheduler.js
│   │   └── logger.js
│   └── utils/
│       ├── dns.js
│       └── validation.js
├── public/
│   ├── index.html
│   ├── login.html
│   ├── register.html
│   ├── css/style.css
│   └── js/app.js
├── docker-compose.yml
├── config.yml
└── logs/
    └── sync-history.jsonl
```

## Docker Image

The Docker image is automatically built and published to `git.juzo.io/juzo/hashi` on every push to the main branch (for relevant file changes).

### Available Tags

- `latest` - Latest build from main branch
- `<sha>` - Specific commit SHA
- `<version>` - Semantic version tags (when released)

### CI/CD

The Gitea Actions workflow:
- Builds multi-platform images (amd64, arm64)
- Uses layer caching for faster builds
- Only triggers on relevant file changes (not README, etc.)
- Can be manually triggered via workflow dispatch

## License

MIT
