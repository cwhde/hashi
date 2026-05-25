/**
 * Authentication service with session management.
 */

import bcrypt from 'bcrypt';
import crypto from 'node:crypto';
import fs from 'node:fs';
import yaml from 'js-yaml';

const BCRYPT_COST = 12;
const SESSION_TIMEOUT_MS = 24 * 60 * 60 * 1000; // 24 hours

export class AuthService {
  constructor(config, configPath) {
    this.config = config;
    this.configPath = configPath;
    this.sessions = new Map(); // sessionId -> { username, createdAt, lastActivity }
    
    // Migrate plaintext password to hash if needed
    this.migratePlaintextPassword();
  }

  /**
   * Migrate plaintext password to hash.
   */
  migratePlaintextPassword() {
    const auth = this.config.auth || {};
    
    if (auth.password && !auth.password_hash) {
      const hash = bcrypt.hashSync(auth.password, BCRYPT_COST);
      
      // Update config
      this.config.config.auth = {
        ...auth,
        password_hash: hash,
      };
      delete this.config.config.auth.password;
      
      // Save config
      this.saveConfig();
    }
  }

  /**
   * Save configuration to file.
   */
  saveConfig() {
    const content = yaml.dump(this.config.config, {
      lineWidth: -1,
      noRefs: true,
      quotingType: '"',
    });
    fs.writeFileSync(this.configPath, content, 'utf8');
  }

  /**
   * Get authentication status.
   * @returns {{registered: boolean, loggedIn: boolean}}
   */
  getStatus() {
    const auth = this.config.auth || {};
    const registered = !!(auth.username && auth.password_hash);
    return { registered };
  }

  /**
   * Register a new user.
   * @param {string} username - Username
   * @param {string} password - Password
   * @returns {{success: boolean, error?: string}}
   */
  async register(username, password) {
    const auth = this.config.auth || {};
    
    // Check if already registered
    if (auth.username && auth.password_hash) {
      return { success: false, error: 'User already registered' };
    }
    
    // Validate input
    if (!username || username.length < 3) {
      return { success: false, error: 'Username must be at least 3 characters' };
    }
    if (!password || password.length < 8) {
      return { success: false, error: 'Password must be at least 8 characters' };
    }
    
    // Hash password
    const hash = await bcrypt.hash(password, BCRYPT_COST);
    
    // Update config
    this.config.config.auth = {
      username,
      password_hash: hash,
    };
    
    // Save config
    this.saveConfig();
    
    return { success: true };
  }

  /**
   * Login and create session.
   * @param {string} username - Username
   * @param {string} password - Password
   * @returns {{success: boolean, sessionId?: string, error?: string}}
   */
  async login(username, password) {
    const auth = this.config.auth || {};
    
    // Check if registered
    if (!auth.username || !auth.password_hash) {
      return { success: false, error: 'No user registered' };
    }
    
    // Verify username
    if (username !== auth.username) {
      return { success: false, error: 'Invalid credentials' };
    }
    
    // Verify password
    const valid = await bcrypt.compare(password, auth.password_hash);
    if (!valid) {
      return { success: false, error: 'Invalid credentials' };
    }
    
    // Create session
    const sessionId = crypto.randomBytes(32).toString('hex');
    this.sessions.set(sessionId, {
      username,
      createdAt: Date.now(),
      lastActivity: Date.now(),
    });
    
    return { success: true, sessionId };
  }

  /**
   * Validate a session.
   * @param {string} sessionId - Session ID
   * @returns {boolean}
   */
  validateSession(sessionId) {
    if (!sessionId) return false;
    
    const session = this.sessions.get(sessionId);
    if (!session) return false;
    
    // Check timeout
    if (Date.now() - session.lastActivity > SESSION_TIMEOUT_MS) {
      this.sessions.delete(sessionId);
      return false;
    }
    
    // Update last activity
    session.lastActivity = Date.now();
    return true;
  }

  /**
   * Logout and clear session.
   * @param {string} sessionId - Session ID
   */
  logout(sessionId) {
    if (sessionId) {
      this.sessions.delete(sessionId);
    }
  }

  /**
   * Clean up expired sessions.
   */
  cleanupSessions() {
    const now = Date.now();
    for (const [sessionId, session] of this.sessions) {
      if (now - session.lastActivity > SESSION_TIMEOUT_MS) {
        this.sessions.delete(sessionId);
      }
    }
  }
}
