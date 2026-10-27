/**
 * DNS Sync Web Application - Main JavaScript
 */

// State
let ws = null;
let currentLogFilter = 'all';
let logs = [];

// Initialize on load
document.addEventListener('DOMContentLoaded', () => {
  initTabs();
  initLogFilters();
  initWebSocket();
  loadStatus();
  loadConfig();
  loadHistory();
  
  // Set up periodic status refresh
  setInterval(loadStatus, 5000);
  
  // Set up form submission
  document.getElementById('settings-form').addEventListener('submit', saveConfig);
});

// Tab navigation
function initTabs() {
  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      // Update tab buttons
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      
      // Update tab content
      document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
      const tabId = tab.dataset.tab + '-tab';
      document.getElementById(tabId).classList.add('active');
      
      // Load data for specific tabs
      if (tab.dataset.tab === 'history') {
        loadHistory();
      }
    });
  });
}

// Log filter buttons
function initLogFilters() {
  document.querySelectorAll('.log-filter').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.log-filter').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentLogFilter = btn.dataset.level;
      renderLogs();
    });
  });
}

// WebSocket connection for live logs
function initWebSocket() {
  const protocol = globalThis.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${globalThis.location.host}/ws/logs`;
  
  ws = new WebSocket(wsUrl);
  
  ws.onopen = () => {
    console.log('WebSocket connected');
  };
  
  ws.onmessage = (event) => {
    try {
      const entry = JSON.parse(event.data);
      logs.push(entry);
      
      // Keep only last 1000 entries
      if (logs.length > 1000) {
        logs.shift();
      }
      
      renderLogs();
    } catch (err) {
      console.error('Failed to parse log entry:', err);
    }
  };
  
  ws.onclose = () => {
    console.log('WebSocket closed, reconnecting in 3s...');
    setTimeout(initWebSocket, 3000);
  };
  
  ws.onerror = (err) => {
    console.error('WebSocket error:', err);
  };
}

// Render log entries
function renderLogs() {
  const container = document.getElementById('log-entries');
  
  let filteredLogs = logs;
  if (currentLogFilter !== 'all') {
    filteredLogs = logs.filter(l => l.level === currentLogFilter);
  }
  
  // Show last 100 entries
  const toShow = filteredLogs.slice(-100);
  
  container.innerHTML = toShow.map(entry => `
    <div class="log-entry">
      <span class="log-timestamp">${formatTimestamp(entry.ts)}</span>
      <span class="log-level ${entry.level.toLowerCase()}">${entry.level}</span>
      <span class="log-message">${escapeHtml(entry.msg)}</span>
    </div>
  `).join('');
  
  // Auto-scroll to bottom
  container.scrollTop = container.scrollHeight;
}

// Clear logs
function clearLogs() {
  logs = [];
  renderLogs();
}

// Load sync status
async function loadStatus() {
  try {
    const response = await fetch('/api/status');
    if (!response.ok) {
      if (response.status === 401) {
        globalThis.location.href = '/login.html';
        return;
      }
      throw new Error('Failed to load status');
    }
    
    const data = await response.json();
    
    // Update status indicator
    const indicator = document.getElementById('status-indicator');
    const text = document.getElementById('status-text');
    indicator.className = 'status-indicator ' + data.state.toLowerCase();
    text.textContent = data.state;
    
    // Update sync button
    const syncBtn = document.getElementById('sync-btn');
    const syncBtnText = document.getElementById('sync-btn-text');
    const syncSpinner = document.getElementById('sync-spinner');
    
    if (data.state === 'RUNNING') {
      syncBtn.disabled = true;
      syncBtnText.textContent = 'Syncing...';
      syncSpinner.style.display = 'inline-block';
    } else {
      syncBtn.disabled = false;
      syncBtnText.textContent = 'Sync Now';
      syncSpinner.style.display = 'none';
    }
    
    // Update last run
    document.getElementById('last-run').textContent = data.lastRun 
      ? formatRelativeTime(data.lastRun)
      : '-';
    
    // Update next run
    document.getElementById('next-run').textContent = data.timeUntilNext
      ? `in ${formatDuration(data.timeUntilNext)}`
      : '-';
    
    // Update last result
    const lastResult = document.getElementById('last-result');
    if (data.lastResult) {
      lastResult.textContent = data.lastResult.success ? '✓ Success' : '✗ Failed';
      lastResult.style.color = data.lastResult.success ? 'var(--success)' : 'var(--error)';
    } else {
      lastResult.textContent = '-';
      lastResult.style.color = '';
    }
    
  } catch (err) {
    console.error('Failed to load status:', err);
  }
}

// Trigger manual sync
async function triggerSync() {
  try {
    const response = await fetch('/api/sync', { method: 'POST' });
    if (!response.ok) {
      throw new Error('Failed to trigger sync');
    }
    
    showToast('Sync started', 'success');
    loadStatus();
    
  } catch (err) {
    console.error('Failed to trigger sync:', err);
    showToast('Failed to start sync', 'error');
  }
}

// Load configuration
async function loadConfig() {
  try {
    const response = await fetch('/api/config');
    if (!response.ok) throw new Error('Failed to load config');
    
    const config = await response.json();
    
    // Populate form fields
    // General
    document.getElementById('domain').value = config.general?.domain || '';
    document.getElementById('topology_source').value = config.general?.topology_source || '';
    document.getElementById('resolver_ip').value = config.general?.resolver_ip || '9.9.9.9';
    document.getElementById('loop_interval').value = config.general?.loop_interval || 300;
    document.getElementById('gatus_output_path').value = config.general?.gatus_output_path || '';
    document.getElementById('keep_subdomains').value = (config.general?.keep_subdomains || []).join(', ');
    document.getElementById('ignore_subdomains').value = (config.general?.ignore_subdomains || []).join(', ');
    
    // Name overrides (object to YAML-like format)
    const nameOverrides = config.general?.name_overrides || {};
    document.getElementById('name_overrides').value = Object.entries(nameOverrides)
      .map(([k, v]) => `${k}: ${v}`)
      .join('\n');
    
    // Pangolin
    document.getElementById('pangolin_base_url').value = config.apis?.pangolin?.base_url || '';
    document.getElementById('pangolin_auth_token').value = ''; // Don't show token
    document.getElementById('pangolin_auth_token').placeholder = config.apis?.pangolin?.auth_token ? 'Leave empty to keep current' : 'Enter token';
    document.getElementById('pangolin_org_id').value = config.apis?.pangolin?.org_id || '';
    
    // Hetzner
    document.getElementById('hetzner_auth_token').value = '';
    document.getElementById('hetzner_auth_token').placeholder = config.apis?.hetzner?.auth_token ? 'Leave empty to keep current' : 'Enter token';
    document.getElementById('hetzner_zone_id').value = config.apis?.hetzner?.zone_id || '';
    
    // Gatus defaults
    document.getElementById('gatus_interval').value = config.gatus_defaults?.interval || '5m';
    document.getElementById('gatus_timeout').value = config.gatus_defaults?.client?.timeout || '10s';
    document.getElementById('allowed_http_codes').value = (config.gatus_defaults?.allowed_http_codes || [200]).join(', ');
    document.getElementById('skip_technical_cnames').checked = config.gatus_defaults?.skip_technical_cnames ?? true;
    document.getElementById('aggressive_host_filtering').checked = config.gatus_defaults?.aggressive_host_filtering ?? false;
    
    // Subdomain HTTP codes (object to readable format)
    const subdomainHttpCodes = config.gatus_defaults?.subdomain_http_codes || {};
    document.getElementById('subdomain_http_codes').value = Object.entries(subdomainHttpCodes)
      .filter(([, subs]) => Array.isArray(subs) && subs.length > 0)
      .map(([code, subs]) => `${code}: ${subs.join(', ')}`)
      .join('\n');
    
    // Subdomain port overrides (object to YAML-like format)
    const portOverrides = config.gatus_defaults?.subdomain_port_overrides || {};
    document.getElementById('subdomain_port_overrides').value = Object.entries(portOverrides)
      .map(([k, v]) => `${k}: ${v}`)
      .join('\n');
    
    // Alerts (array to YAML format)
    const alerts = config.gatus_defaults?.alerts || [];
    document.getElementById('alerts_config').value = alerts.length > 0
      ? alerts.map(alert => {
          const lines = ['- type: ' + alert.type];
          // Add all properties except 'type'
          for (const [key, value] of Object.entries(alert)) {
            if (key !== 'type') {
              lines.push('  ' + key + ': ' + value);
            }
          }
          return lines.join('\n');
        }).join('\n')
      : '';
    
  } catch (err) {
    console.error('Failed to load config:', err);
    showToast('Failed to load configuration', 'error');
  }
}

// Save configuration
async function saveConfig(e) {
  e.preventDefault();
  
  // Parse name overrides from YAML-like format
  const nameOverridesText = document.getElementById('name_overrides').value;
  const nameOverrides = {};
  for (const line of nameOverridesText.split('\n')) {
    const trimmed = line.trim();
    if (trimmed && trimmed.includes(':')) {
      const [key, ...valueParts] = trimmed.split(':');
      nameOverrides[key.trim()] = valueParts.join(':').trim();
    }
  }
  
  // Parse subdomain HTTP codes
  const subdomainHttpCodesText = document.getElementById('subdomain_http_codes').value;
  const subdomainHttpCodes = {};
  for (const line of subdomainHttpCodesText.split('\n')) {
    const trimmed = line.trim();
    if (trimmed && trimmed.includes(':')) {
      const [code, ...subsParts] = trimmed.split(':');
      const subs = subsParts.join(':').split(',').map(s => s.trim()).filter(Boolean);
      if (subs.length > 0) {
        subdomainHttpCodes[code.trim()] = subs;
      }
    }
  }
  
  // Parse subdomain port overrides
  const portOverridesText = document.getElementById('subdomain_port_overrides').value;
  const subdomainPortOverrides = {};
  for (const line of portOverridesText.split('\n')) {
    const trimmed = line.trim();
    if (trimmed && trimmed.includes(':')) {
      const [key, value] = trimmed.split(':');
      const port = Number.parseInt(value.trim(), 10);
      if (!Number.isNaN(port)) {
        subdomainPortOverrides[key.trim()] = port;
      }
    }
  }
  
  // Parse alerts from YAML-like format
  const alertsText = document.getElementById('alerts_config').value;
  const alerts = parseAlerts(alertsText);
  
  // Build config object
  const config = {
    general: {
      domain: document.getElementById('domain').value,
      topology_source: document.getElementById('topology_source').value,
      resolver_ip: document.getElementById('resolver_ip').value,
      loop_interval: Number.parseInt(document.getElementById('loop_interval').value, 10) || 300,
      gatus_output_path: document.getElementById('gatus_output_path').value,
      keep_subdomains: parseList(document.getElementById('keep_subdomains').value),
      ignore_subdomains: parseList(document.getElementById('ignore_subdomains').value),
      name_overrides: nameOverrides,
    },
    apis: {
      pangolin: {
        base_url: document.getElementById('pangolin_base_url').value,
        org_id: document.getElementById('pangolin_org_id').value,
      },
      hetzner: {
        zone_id: document.getElementById('hetzner_zone_id').value,
      },
    },
    gatus_defaults: {
      interval: document.getElementById('gatus_interval').value,
      client: {
        timeout: document.getElementById('gatus_timeout').value,
      },
      allowed_http_codes: parseList(document.getElementById('allowed_http_codes').value).map(Number),
      skip_technical_cnames: document.getElementById('skip_technical_cnames').checked,
      aggressive_host_filtering: document.getElementById('aggressive_host_filtering').checked,
      subdomain_http_codes: subdomainHttpCodes,
      subdomain_port_overrides: subdomainPortOverrides,
      alerts: alerts,
    },
  };
  
  // Add tokens only if provided
  const pangolinToken = document.getElementById('pangolin_auth_token').value;
  if (pangolinToken) {
    config.apis.pangolin.auth_token = pangolinToken;
  }
  
  const hetznerToken = document.getElementById('hetzner_auth_token').value;
  if (hetznerToken) {
    config.apis.hetzner.auth_token = hetznerToken;
  }
  
  try {
    const response = await fetch('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    });
    
    if (!response.ok) {
      const data = await response.json();
      throw new Error(data.error || 'Failed to save config');
    }
    
    showToast('Configuration saved successfully', 'success');
    loadConfig(); // Reload to get updated values
    
  } catch (err) {
    console.error('Failed to save config:', err);
    showToast(err.message, 'error');
  }
}

// Restore config from backup
async function restoreConfig() {
  if (!confirm('Are you sure you want to restore the previous configuration?')) {
    return;
  }
  
  try {
    const response = await fetch('/api/config/restore', { method: 'POST' });
    if (!response.ok) {
      const data = await response.json();
      throw new Error(data.error || 'Failed to restore config');
    }
    
    showToast('Configuration restored from backup', 'success');
    loadConfig();
    
  } catch (err) {
    console.error('Failed to restore config:', err);
    showToast(err.message, 'error');
  }
}

// Test Pangolin API
async function testPangolin() {
  const resultSpan = document.getElementById('pangolin-test-result');
  resultSpan.innerHTML = '<span class="spinner"></span>';
  resultSpan.className = 'test-result';
  
  try {
    const response = await fetch('/api/config/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'pangolin' }),
    });
    
    const data = await response.json();
    
    if (data.pangolin?.valid) {
      resultSpan.textContent = '✓ Connected';
      resultSpan.className = 'test-result success';
    } else {
      resultSpan.textContent = '✗ ' + (data.pangolin?.error || 'Failed');
      resultSpan.className = 'test-result error';
    }
  } catch (err) {
    resultSpan.textContent = '✗ Error';
    resultSpan.className = 'test-result error';
  }
}

// Test Hetzner API
async function testHetzner() {
  const resultSpan = document.getElementById('hetzner-test-result');
  resultSpan.innerHTML = '<span class="spinner"></span>';
  resultSpan.className = 'test-result';
  
  try {
    const response = await fetch('/api/config/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: 'hetzner' }),
    });
    
    const data = await response.json();
    
    if (data.hetzner?.valid) {
      resultSpan.textContent = '✓ Connected';
      resultSpan.className = 'test-result success';
    } else {
      resultSpan.textContent = '✗ ' + (data.hetzner?.error || 'Failed');
      resultSpan.className = 'test-result error';
    }
  } catch (err) {
    resultSpan.textContent = '✗ Error';
    resultSpan.className = 'test-result error';
  }
}

// Load sync history
async function loadHistory() {
  try {
    const response = await fetch('/api/history');
    if (!response.ok) throw new Error('Failed to load history');
    
    const data = await response.json();
    const container = document.getElementById('history-list');
    
    if (data.runs.length === 0) {
      container.innerHTML = '<p style="color: var(--text-muted); text-align: center; padding: 2rem;">No sync history yet</p>';
      return;
    }
    
    container.innerHTML = data.runs.slice(0, 20).map(run => `
      <div class="history-item ${run.success ? 'success' : 'error'}" onclick="showRunDetails('${run.id}')">
        <div class="history-info">
          <span class="history-time">${formatTimestamp(run.start)}</span>
          <span class="history-id">${run.id}</span>
        </div>
        <span class="history-status ${run.success ? 'success' : 'error'}">
          ${run.success ? '✓ Success' : '✗ Failed'}
        </span>
      </div>
    `).join('');
    
  } catch (err) {
    console.error('Failed to load history:', err);
  }
}

// Show run details in modal
async function showRunDetails(runId) {
  try {
    const response = await fetch(`/api/history/${runId}`);
    if (!response.ok) throw new Error('Failed to load run details');
    
    const data = await response.json();
    const container = document.getElementById('modal-log-entries');
    
    container.innerHTML = data.logs.map(entry => `
      <div class="log-entry">
        <span class="log-timestamp">${formatTimestamp(entry.ts)}</span>
        <span class="log-level ${entry.level.toLowerCase()}">${entry.level}</span>
        <span class="log-message">${escapeHtml(entry.msg)}</span>
      </div>
    `).join('');
    
    document.getElementById('history-modal').classList.add('visible');
    
  } catch (err) {
    console.error('Failed to load run details:', err);
    showToast('Failed to load run details', 'error');
  }
}

// Close history modal
function closeHistoryModal() {
  document.getElementById('history-modal').classList.remove('visible');
}

// Logout
async function logout() {
  try {
    await fetch('/auth/logout', { method: 'POST' });
    globalThis.location.href = '/login.html';
  } catch (err) {
    console.error('Logout error:', err);
    globalThis.location.href = '/login.html';
  }
}

// Show toast notification
function showToast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.textContent = message;
  
  container.appendChild(toast);
  
  setTimeout(() => {
    toast.style.animation = 'slideIn 0.3s ease reverse';
    setTimeout(() => toast.remove(), 300);
  }, 3000);
}

// Utility functions
function formatTimestamp(ts) {
  const date = new Date(ts);
  return date.toLocaleTimeString('en-US', { 
    hour12: false, 
    hour: '2-digit', 
    minute: '2-digit', 
    second: '2-digit' 
  });
}

function formatRelativeTime(ts) {
  const date = new Date(ts);
  const now = new Date();
  const diff = Math.floor((now - date) / 1000);
  
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return date.toLocaleDateString();
}

function formatDuration(seconds) {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}

function parseList(str) {
  return str.split(',').map(s => s.trim()).filter(s => s);
}

function parseAlerts(text) {
  // Parse YAML-like alerts config
  // Format:
  // - type: discord
  //   discord-webhook-url: https://...
  const alerts = [];
  const lines = text.split('\n');
  let currentAlert = null;
  
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    
    // Check if this is a new alert (starts with "- type:")
    if (trimmed.startsWith('- type:')) {
      if (currentAlert) {
        alerts.push(currentAlert);
      }
      currentAlert = {
        type: trimmed.slice(8).trim()
      };
    } else if (trimmed.startsWith('type:') && !line.startsWith(' ') && !line.startsWith('\t')) {
      // Also support "type:" without dash at start
      if (currentAlert) {
        alerts.push(currentAlert);
      }
      currentAlert = {
        type: trimmed.slice(5).trim()
      };
    } else if (currentAlert && trimmed.includes(':')) {
      // This is a property of the current alert
      const colonIdx = trimmed.indexOf(':');
      const key = trimmed.slice(0, colonIdx).trim();
      let value = trimmed.slice(colonIdx + 1).trim();
      
      if (key) {
        // Convert value to appropriate type
        if (value === 'true') {
          currentAlert[key] = true;
        } else if (value === 'false') {
          currentAlert[key] = false;
        } else if (!Number.isNaN(Number(value)) && value !== '') {
          currentAlert[key] = Number(value);
        } else {
          currentAlert[key] = value;
        }
      }
    }
  }
  
  if (currentAlert) {
    alerts.push(currentAlert);
  }
  
  return alerts;
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// Close modal on overlay click
document.getElementById('history-modal').addEventListener('click', (e) => {
  if (e.target.classList.contains('modal-overlay')) {
    closeHistoryModal();
  }
});

// Close modal on escape key
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    closeHistoryModal();
  }
});
