/**
 * API routes for sync operations and configuration.
 */

import { validateConfig, testPangolinToken, testHetznerToken } from '../utils/validation.js';

export default async function apiRoutes(fastify, options) {
  const { state } = options;

  // Auth check for all API routes
  fastify.addHook('preHandler', async (request, reply) => {
    const sessionId = request.cookies.session_id;
    if (!state.auth.validateSession(sessionId)) {
      return reply.code(401).send({ error: 'Unauthorized' });
    }
  });

  // GET /api/status - Get sync status
  fastify.get('/status', async (request, reply) => {
    const timeUntilNext = state.scheduler.getTimeUntilNextRun();
    
    return {
      state: state.syncState,
      lastRun: state.lastRun,
      lastResult: state.lastResult,
      nextRun: state.nextRun,
      timeUntilNext: timeUntilNext ? Math.ceil(timeUntilNext / 1000) : null,
      loopInterval: state.config.loopInterval,
    };
  });

  // POST /api/sync - Trigger manual sync
  fastify.post('/sync', async (request, reply) => {
    if (state.syncState === 'RUNNING') {
      return reply.code(409).send({ error: 'Sync already running' });
    }
    
    // Run sync asynchronously
    const runId = await state.scheduler.runSync(true);
    
    return { started: true, run_id: runId };
  });

  // GET /api/logs - Get logs
  fastify.get('/logs', async (request, reply) => {
    const { since, limit, levels } = request.query;
    
    const parsedLevels = levels ? levels.split(',') : null;
    const logs = state.logger.getLogs({
      since,
      limit: limit ? Number.parseInt(limit, 10) : 100,
      levels: parsedLevels,
    });
    
    return { logs };
  });

  // GET /api/history - Get sync history
  fastify.get('/history', async (request, reply) => {
    const runs = state.logger.getHistory();
    return { runs };
  });

  // GET /api/history/:runId - Get logs for a specific run
  fastify.get('/history/:runId', async (request, reply) => {
    const { runId } = request.params;
    const logs = state.logger.getRunLogs(runId);
    
    if (logs.length === 0) {
      return reply.code(404).send({ error: 'Run not found' });
    }
    
    return { logs };
  });

  // GET /api/config - Get configuration
  fastify.get('/config', async (request, reply) => {
    return state.config.toJSON(true); // Mask sensitive fields
  });

  // PUT /api/config - Update configuration
  fastify.put('/config', async (request, reply) => {
    const updates = request.body;
    
    // Validate updates
    const validation = validateConfig(updates);
    if (!validation.valid) {
      return reply.code(400).send({ error: 'Validation failed', errors: validation.errors });
    }
    
    try {
      // Create backup
      state.config.backup();
      
      // Apply updates
      state.config.update(updates);
      state.config.save();
      
      // Reload sync engine
      state.sync.reloadConfig();
      
      // Restart scheduler if interval changed
      state.scheduler.restart();
      
      state.logger.info('Configuration updated successfully');
      
      return { success: true, applied: true };
    } catch (error) {
      state.logger.error(`Failed to update configuration: ${error.message}`);
      return reply.code(500).send({ error: error.message });
    }
  });

  // POST /api/config/restore - Restore configuration from backup
  fastify.post('/config/restore', async (request, reply) => {
    try {
      const restored = state.config.restore();
      
      if (!restored) {
        return reply.code(404).send({ error: 'No backup found' });
      }
      
      // Reload sync engine
      state.sync.reloadConfig();
      
      // Restart scheduler
      state.scheduler.restart();
      
      state.logger.info('Configuration restored from backup');
      
      return { success: true, restored: true };
    } catch (error) {
      state.logger.error(`Failed to restore configuration: ${error.message}`);
      return reply.code(500).send({ error: error.message });
    }
  });

  // POST /api/config/test - Test API tokens
  fastify.post('/config/test', async (request, reply) => {
    const { type } = request.body;
    const results = {};
    
    if (type === 'pangolin' || type === 'all') {
      const pangolinUrl = state.config.pangolinBaseUrl;
      const pangolinToken = state.config.pangolinAuthToken;
      
      if (pangolinUrl && pangolinToken) {
        results.pangolin = await testPangolinToken(pangolinUrl, pangolinToken);
      } else {
        results.pangolin = { valid: false, error: 'Missing URL or token' };
      }
    }
    
    if (type === 'hetzner' || type === 'all') {
      const hetznerToken = state.config.hetznerAuthToken;
      
      if (hetznerToken) {
        results.hetzner = await testHetznerToken(hetznerToken);
      } else {
        results.hetzner = { valid: false, error: 'Missing token' };
      }
    }
    
    return results;
  });
}
