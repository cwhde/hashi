import Fastify from 'fastify';
import fastifyStatic from '@fastify/static';
import fastifyCookie from '@fastify/cookie';
import fastifyFormBody from '@fastify/formbody';
import fastifyWebsocket from '@fastify/websocket';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import fs from 'fs';
import crypto from 'crypto';

import { Config } from './src/core/config.js';
import { DNSSync } from './src/core/sync.js';
import { AuthService } from './src/services/auth.js';
import { Logger } from './src/services/logger.js';
import { Scheduler } from './src/services/scheduler.js';
import authRoutes from './src/routes/auth.js';
import apiRoutes from './src/routes/api.js';
import websocketRoutes from './src/routes/websocket.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Initialize Fastify
const fastify = Fastify({
  logger: false, // We use our own logger
});

// Global state
const state = {
  config: null,
  sync: null,
  auth: null,
  logger: null,
  scheduler: null,
  syncState: 'IDLE', // IDLE | RUNNING | ERROR
  lastRun: null,
  lastResult: null,
  nextRun: null,
  currentRunId: null,
};

// Initialize logger
state.logger = new Logger(join(__dirname, 'logs'));

// Load configuration
const configPath = join(__dirname, 'config.yml');
try {
  state.config = new Config(configPath);
  state.logger.info('Configuration loaded successfully');
} catch (error) {
  state.logger.error(`Failed to load configuration: ${error.message}`);
  process.exit(1);
}

// Initialize auth service
state.auth = new AuthService(state.config, configPath);

// Initialize DNS sync
state.sync = new DNSSync(state.config, state.logger);

// Initialize scheduler
state.scheduler = new Scheduler(state, state.logger);

// Register plugins
await fastify.register(fastifyCookie, {
  secret: crypto.randomBytes(32).toString('hex'),
  parseOptions: {
    httpOnly: true,
    sameSite: 'strict',
    secure: process.env.NODE_ENV === 'production',
    path: '/',
  },
});

await fastify.register(fastifyFormBody);
await fastify.register(fastifyWebsocket);

await fastify.register(fastifyStatic, {
  root: join(__dirname, 'public'),
  prefix: '/',
});

// Auth check decorator
fastify.decorate('authenticate', async (request, reply) => {
  const sessionId = request.cookies.session_id;
  if (!sessionId || !state.auth.validateSession(sessionId)) {
    // Check if this is an API request or page request
    const isApiRequest = request.url.startsWith('/api/') || request.url.startsWith('/ws/');
    if (isApiRequest) {
      reply.code(401).send({ error: 'Unauthorized' });
    } else {
      reply.redirect('/login.html');
    }
    return false;
  }
  return true;
});

// Register routes
await fastify.register(authRoutes, { prefix: '/auth', state });
await fastify.register(apiRoutes, { prefix: '/api', state });
await fastify.register(websocketRoutes, { prefix: '/ws', state });

// Protected routes - redirect to login if not authenticated
fastify.addHook('preHandler', async (request, reply) => {
  const publicPaths = [
    '/login.html',
    '/register.html',
    '/auth/',
    '/css/',
    '/js/',
    '/favicon.ico',
  ];
  
  // Allow public paths
  const isPublic = publicPaths.some(p => request.url.startsWith(p));
  if (isPublic) return;
  
  // Check auth status
  const authStatus = state.auth.getStatus();
  
  // If not registered, redirect to register
  if (!authStatus.registered && request.url !== '/register.html') {
    return reply.redirect('/register.html');
  }
  
  // If registered but not logged in, redirect to login
  const sessionId = request.cookies.session_id;
  if (authStatus.registered && !state.auth.validateSession(sessionId)) {
    if (!request.url.startsWith('/auth/')) {
      return reply.redirect('/login.html');
    }
  }
});

// Root redirect
fastify.get('/', async (request, reply) => {
  const authStatus = state.auth.getStatus();
  if (!authStatus.registered) {
    return reply.redirect('/register.html');
  }
  const sessionId = request.cookies.session_id;
  if (!state.auth.validateSession(sessionId)) {
    return reply.redirect('/login.html');
  }
  return reply.sendFile('index.html');
});

// Start server
const start = async () => {
  try {
    const port = process.env.PORT || 3000;
    const host = process.env.HOST || '0.0.0.0';
    
    await fastify.listen({ port, host });
    state.logger.info(`DNS Sync Web Application started on http://${host}:${port}`);
    
    // Start scheduler
    state.scheduler.start();
    
    // Clean up old logs on startup
    state.logger.cleanup();
    
  } catch (err) {
    state.logger.error(`Server startup failed: ${err.message}`);
    process.exit(1);
  }
};

start();
