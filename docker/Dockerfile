# syntax=docker/dockerfile:1

# =============================================================================
# Hashi (橋) - Multi-stage optimized Docker build
# =============================================================================
# This Dockerfile is optimized for:
# - Minimal image size using Alpine
# - Layer caching for faster rebuilds
# - Security best practices (non-root user)
# - Smart layer ordering (dependencies before source)
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Dependencies
# -----------------------------------------------------------------------------
# Install dependencies separately to maximize layer caching.
# This layer only rebuilds when package.json or package-lock.json changes.
FROM node:20-alpine AS deps

WORKDIR /app

# Install build dependencies for native modules (bcrypt)
RUN apk add --no-cache python3 make g++

# Copy only package files first (better layer caching)
COPY package.json package-lock.json ./

# Install production dependencies only
RUN npm ci --only=production && \
    npm cache clean --force

# -----------------------------------------------------------------------------
# Stage 2: Production image
# -----------------------------------------------------------------------------
FROM node:20-alpine AS production

# Add labels for container registry linking
LABEL org.opencontainers.image.source="https://git.juzo.io/juzo/hashi"
LABEL org.opencontainers.image.description="Hashi - Homelab orchestration for Pangolin, Hetzner DNS, and Gatus"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.title="Hashi"
LABEL org.opencontainers.image.vendor="juzo"

WORKDIR /app

# Create non-root user for security
RUN addgroup -g 1001 -S hashi && \
    adduser -S -u 1001 -G hashi hashi

# Copy production dependencies from deps stage
COPY --from=deps /app/node_modules ./node_modules

# Copy source files (ordered by change frequency for better caching)
# Less frequently changed files first
COPY --chown=hashi:hashi package.json ./
COPY --chown=hashi:hashi public ./public
COPY --chown=hashi:hashi src ./src
COPY --chown=hashi:hashi server.js ./

# Create directories for runtime data
RUN mkdir -p /app/logs && chown -R hashi:hashi /app/logs

# Switch to non-root user
USER hashi

# Expose application port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/auth/status || exit 1

# Start the application
CMD ["node", "server.js"]
