/**
 * Scheduler service for automatic sync cycles.
 */

import crypto from 'node:crypto';

export class Scheduler {
  constructor(state, logger) {
    this.state = state;
    this.logger = logger;
    this.timer = null;
    this.nextRunTime = null;
  }

  /**
   * Start the scheduler.
   */
  start() {
    const interval = this.state.config.loopInterval * 1000;
    this.logger.info(`Starting scheduler with ${this.state.config.loopInterval}s interval`);
    
    // Schedule first run
    this.scheduleNextRun(interval);
  }

  /**
   * Stop the scheduler.
   */
  stop() {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.nextRunTime = null;
  }

  /**
   * Restart the scheduler with new interval.
   */
  restart() {
    this.stop();
    this.start();
  }

  /**
   * Schedule the next sync run.
   * @param {number} delayMs - Delay in milliseconds
   */
  scheduleNextRun(delayMs) {
    this.nextRunTime = Date.now() + delayMs;
    this.state.nextRun = new Date(this.nextRunTime).toISOString();
    
    this.timer = setTimeout(() => {
      this.runSync();
    }, delayMs);
  }

  /**
   * Run a sync cycle.
   * @param {boolean} manual - Whether this is a manual trigger
   * @returns {Promise<{success: boolean, runId: string}>}
   */
  async runSync(manual = false) {
    // Don't run if already running
    if (this.state.syncState === 'RUNNING') {
      return { success: false, runId: null, error: 'Sync already running' };
    }
    
    const runId = crypto.randomBytes(8).toString('hex');
    this.state.currentRunId = runId;
    this.state.syncState = 'RUNNING';
    this.logger.setRunId(runId);
    
    const startTime = Date.now();
    
    try {
      this.logger.info(`Starting sync cycle (${manual ? 'manual' : 'scheduled'})`);
      
      const result = await this.state.sync.runOnce();
      
      const duration = ((Date.now() - startTime) / 1000).toFixed(2);
      
      this.state.lastRun = new Date().toISOString();
      this.state.lastResult = {
        success: result.success,
        summary: result.summary,
        duration: `${duration}s`,
        runId,
      };
      
      if (result.success) {
        this.state.syncState = 'IDLE';
      } else {
        this.state.syncState = 'ERROR';
        // Auto-transition to IDLE after 5 seconds
        setTimeout(() => {
          if (this.state.syncState === 'ERROR') {
            this.state.syncState = 'IDLE';
          }
        }, 5000);
      }
      
      // Clean up logs after each sync
      this.logger.cleanup();
      
      // Schedule next run if not manual
      if (!manual || this.timer === null) {
        const interval = this.state.config.loopInterval * 1000;
        this.scheduleNextRun(interval);
      }
      
      return { success: result.success, runId };
      
    } catch (error) {
      this.logger.error(`Sync failed with exception: ${error.message}`);
      
      this.state.lastRun = new Date().toISOString();
      this.state.lastResult = {
        success: false,
        error: error.message,
        runId,
      };
      this.state.syncState = 'ERROR';
      
      // Auto-transition to IDLE after 5 seconds
      setTimeout(() => {
        if (this.state.syncState === 'ERROR') {
          this.state.syncState = 'IDLE';
        }
      }, 5000);
      
      // Schedule next run
      const interval = this.state.config.loopInterval * 1000;
      this.scheduleNextRun(interval);
      
      return { success: false, runId, error: error.message };
    } finally {
      this.state.currentRunId = null;
      this.logger.setRunId(null);
    }
  }

  /**
   * Get time until next run.
   * @returns {number} - Milliseconds until next run
   */
  getTimeUntilNextRun() {
    if (!this.nextRunTime) return null;
    return Math.max(0, this.nextRunTime - Date.now());
  }
}
