/**
 * Validation utilities for configuration and API inputs.
 */

/**
 * Validate a URL string.
 * @param {string} url - URL to validate
 * @returns {boolean}
 */
export function isValidUrl(url) {
  try {
    new URL(url);
    return true;
  } catch {
    return false;
  }
}

/**
 * Validate an IP address (IPv4).
 * @param {string} ip - IP address to validate
 * @returns {boolean}
 */
export function isValidIp(ip) {
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
  if (!ipv4Regex.test(ip)) return false;
  
  const parts = ip.split('.');
  return parts.every(part => {
    const num = Number.parseInt(part, 10);
    return num >= 0 && num <= 255;
  });
}

/**
 * Validate a domain name.
 * @param {string} domain - Domain to validate
 * @returns {boolean}
 */
export function isValidDomain(domain) {
  const domainRegex = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
  return domainRegex.test(domain) && domain.length <= 253;
}

/**
 * Validate configuration object.
 * @param {object} config - Configuration to validate
 * @returns {{valid: boolean, errors: string[]}}
 */
export function validateConfig(config) {
  const errors = [];
  
  // Validate general section
  if (config.general) {
    if (config.general.domain && !isValidDomain(config.general.domain)) {
      errors.push('Invalid domain format');
    }
    if (config.general.resolver_ip && !isValidIp(config.general.resolver_ip)) {
      errors.push('Invalid resolver IP address');
    }
    if (config.general.loop_interval !== undefined) {
      const interval = Number.parseInt(config.general.loop_interval, 10);
      if (Number.isNaN(interval) || interval < 30) {
        errors.push('Loop interval must be at least 30 seconds');
      }
    }
  }
  
  // Validate API section
  if (config.apis) {
    if (config.apis.pangolin?.base_url && !isValidUrl(config.apis.pangolin.base_url)) {
      errors.push('Invalid Pangolin base URL');
    }
  }
  
  return {
    valid: errors.length === 0,
    errors,
  };
}

/**
 * Test Pangolin API token.
 * @param {string} baseUrl - Pangolin API base URL
 * @param {string} token - Auth token
 * @returns {Promise<{valid: boolean, error?: string}>}
 */
export async function testPangolinToken(baseUrl, token) {
  try {
    const response = await fetch(`${baseUrl.replace(/\/$/, '')}/orgs`, {
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    });
    
    if (response.ok) {
      return { valid: true };
    }
    return { valid: false, error: `HTTP ${response.status}: ${response.statusText}` };
  } catch (error) {
    return { valid: false, error: error.message };
  }
}

/**
 * Test Hetzner DNS API token.
 * @param {string} token - Auth token
 * @returns {Promise<{valid: boolean, error?: string}>}
 */
export async function testHetznerToken(token) {
  try {
    const response = await fetch('https://dns.hetzner.com/api/v1/zones', {
      headers: {
        'Auth-API-Token': token,
      },
    });
    
    if (response.ok) {
      return { valid: true };
    }
    return { valid: false, error: `HTTP ${response.status}: ${response.statusText}` };
  } catch (error) {
    return { valid: false, error: error.message };
  }
}
