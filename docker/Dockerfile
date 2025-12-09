# syntax=docker/dockerfile:1

# =============================================================================
# Hashi (橋) - Minimal Docker build
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Build dependencies
# -----------------------------------------------------------------------------
FROM node:20-alpine AS builder

WORKDIR /app

# Install build deps for native modules (bcrypt)
RUN apk add --no-cache python3 make g++

COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npm cache clean --force

# -----------------------------------------------------------------------------
# Stage 2: Minimal production image
# -----------------------------------------------------------------------------
FROM node:20-alpine

LABEL org.opencontainers.image.source="https://git.juzo.io/juzo/hashi"
LABEL org.opencontainers.image.description="Hashi - Homelab orchestration"
LABEL org.opencontainers.image.licenses="MIT"

WORKDIR /app

# Create non-root user
RUN addgroup -g 1001 -S hashi && adduser -S -u 1001 -G hashi hashi

# Copy only what's needed from builder
COPY --from=builder /app/node_modules ./node_modules

# Copy app source
COPY package.json server.js ./
COPY public ./public
COPY src ./src

# Setup runtime
RUN mkdir -p /app/logs && chown -R hashi:hashi /app
USER hashi

EXPOSE 3000
CMD ["node", "server.js"]
