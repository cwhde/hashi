/**
 * Logger service with in-memory buffer, WebSocket broadcasting, and file persistence.
 */

import fs from 'node:fs';
import path from 'node:path';

const MAX_BUFFER_SIZE = 1000;
const CLEANUP_HOURS = 12;

export class Logger {
  constructor(logsDir) {
    this.logsDir = logsDir;
    this.logFile = path.join(logsDir, 'sync-history.jsonl');
    this.buffer = [];
    this.wsClients = new Set();
    this.currentRunId = null;
    
    // Ensure logs directory exists
    if (!fs.existsSync(logsDir)) {
      fs.mkdirSync(logsDir, { recursive: true });
    }
  }

  /**
   * Set current run ID for log correlation.
   * @param {string} runId - Run ID
   */
  setRunId(runId) {
    this.currentRunId = runId;
  }

  /**
   * Add a WebSocket client for live updates.
   * @param {WebSocket} ws - WebSocket connection
   */
  addClient(ws) {
    this.wsClients.add(ws);
    
    // Send last 100 messages on connect
    const backfill = this.buffer.slice(-100);
    for (const entry of backfill) {
      ws.send(JSON.stringify(entry));
    }
  }

  /**
   * Remove a WebSocket client.
   * @param {WebSocket} ws - WebSocket connection
   */
  removeClient(ws) {
    this.wsClients.delete(ws);
  }

  /**
   * Log a message.
   * @param {string} level - Log level
   * @param {string} msg - Message
   */
  log(level, msg) {
    const entry = {
      ts: new Date().toISOString(),
      level,
      msg,
      run_id: this.currentRunId,
    };
    
    // Add to buffer
    this.buffer.push(entry);
    if (this.buffer.length > MAX_BUFFER_SIZE) {
      this.buffer.shift();
    }
    
    // Broadcast to WebSocket clients
    const json = JSON.stringify(entry);
    for (const ws of this.wsClients) {
      try {
        ws.send(json);
      } catch {
        this.wsClients.delete(ws);
      }
    }
    
    // Persist to file
    try {
      fs.appendFileSync(this.logFile, json + '\n');
    } catch {
      // Ignore file write errors
    }
    
    // Also log to console
    const timestamp = entry.ts.replace('T', ' ').replace('Z', '');
    const levelUpper = level.toUpperCase().padEnd(7);
    console.log(`${timestamp} - ${levelUpper} - ${msg}`);
  }

  /**
   * Log an info message.
   * @param {string} msg - Message
   */
  info(msg) {
    this.log('INFO', msg);
  }

  /**
   * Log a debug message.
   * @param {string} msg - Message
   */
  debug(msg) {
    this.log('DEBUG', msg);
  }

  /**
   * Log a warning message.
   * @param {string} msg - Message
   */
  warn(msg) {
    this.log('WARN', msg);
  }

  /**
   * Log an error message.
   * @param {string} msg - Message
   */
  error(msg) {
    this.log('ERROR', msg);
  }

  /**
   * Log a success message.
   * @param {string} msg - Message
   */
  success(msg) {
    this.log('SUCCESS', msg);
  }

  /**
   * Get logs from buffer.
   * @param {Object} options - Query options
   * @returns {Array}
   */
  getLogs({ since, limit = 100, levels } = {}) {
    let logs = this.buffer;
    
    if (since) {
      const sinceDate = new Date(since);
      logs = logs.filter(l => new Date(l.ts) > sinceDate);
    }
    
    if (levels && levels.length > 0) {
      logs = logs.filter(l => levels.includes(l.level));
    }
    
    return logs.slice(-limit);
  }

  /**
   * Get sync history (runs).
   * @returns {Array}
   */
  getHistory() {
    const runs = new Map();
    
    for (const entry of this.buffer) {
      if (!entry.run_id) continue;
      
      if (!runs.has(entry.run_id)) {
        runs.set(entry.run_id, {
          id: entry.run_id,
          start: entry.ts,
          end: entry.ts,
          success: true,
          logs: [],
        });
      }
      
      const run = runs.get(entry.run_id);
      run.end = entry.ts;
      run.logs.push(entry);
      
      if (entry.level === 'ERROR') {
        run.success = false;
      }
      if (entry.level === 'SUCCESS') {
        run.success = true;
      }
    }
    
    return Array.from(runs.values()).reverse();
  }

  /**
   * Get logs for a specific run.
   * @param {string} runId - Run ID
   * @returns {Array}
   */
  getRunLogs(runId) {
    return this.buffer.filter(l => l.run_id === runId);
  }

  /**
   * Clean up old logs.
   */
  cleanup() {
    const cutoff = new Date(Date.now() - CLEANUP_HOURS * 60 * 60 * 1000);
    
    // Clean buffer
    this.buffer = this.buffer.filter(l => new Date(l.ts) > cutoff);
    
    // Clean file
    try {
      if (fs.existsSync(this.logFile)) {
        const content = fs.readFileSync(this.logFile, 'utf8');
        const lines = content.trim().split('\n').filter(line => {
          try {
            const entry = JSON.parse(line);
            return new Date(entry.ts) > cutoff;
          } catch {
            return false;
          }
        });
        fs.writeFileSync(this.logFile, lines.join('\n') + '\n');
      }
    } catch {
      // Ignore cleanup errors
    }
    
    this.info(`Cleaned up logs older than ${CLEANUP_HOURS} hours`);
  }
}
