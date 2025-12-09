/**
 * Gatus configuration generator.
 * Direct port from script.py lines 383-640
 */

import net from 'node:net';

export class GatusConfigGenerator {
  // Port to Protocol mapping - exact from script.py lines 391-403
  static PORT_PROTOCOLS = {
    8443: 'https',   // Alt HTTPS
    8006: 'https',   // Proxmox
    8080: 'http',    // Alt HTTP
    587: 'starttls', // SMTP
    465: 'tls',      // SMTPS
    993: 'tls',      // IMAPS
    443: 'https',    // HTTPS
    80: 'http',      // HTTP
    123: 'udp',      // NTP
    53: 'dns',       // DNS
    21: 'tcp',       // FTP
  };

  // Check order - exact from script.py line 420
  static CHECK_ORDER = [8443, 8006, 8080, 21, 443, 80, 587, 465, 993, 123, 53];

  constructor(config) {
    this.config = config;
    this.defaults = config.gatusDefaults;
    this.nameOverrides = config.nameOverrides;
    this.portOverrides = config.gatusSubdomainPortOverrides;
    this.domain = config.domain;
    this.hostMapping = {};
    this.subnetToHostname = {};
    this.hostnames = new Set();
  }

  /**
   * Update host mapping for grouping.
   * @param {Object} hostMapping - Map of CNAME prefixes to subnets
   */
  updateHostMapping(hostMapping) {
    this.hostMapping = hostMapping;
    this.subnetToHostname = {};
    this.hostnames = new Set();
    
    for (const [cname, subnet] of Object.entries(hostMapping)) {
      const hostname = cname.replace('on.', '');
      this.subnetToHostname[subnet] = hostname;
      this.hostnames.add(hostname);
    }
  }

  /**
   * Check if a port is open on a host.
   * @param {string} host - Hostname or IP
   * @param {number} port - Port number
   * @param {number} timeout - Timeout in ms
   * @returns {Promise<boolean>}
   */
  async checkPort(host, port, timeout = 2000) {
    return new Promise((resolve) => {
      const socket = new net.Socket();
      
      socket.setTimeout(timeout);
      
      socket.on('connect', () => {
        socket.destroy();
        resolve(true);
      });
      
      socket.on('timeout', () => {
        socket.destroy();
        resolve(false);
      });
      
      socket.on('error', () => {
        socket.destroy();
        resolve(false);
      });
      
      socket.connect(port, host);
    });
  }

  /**
   * Detect the protocol and port for a target.
   * Direct port from script.py detect_protocol_and_port
   * @param {string} target - Target hostname or IP
   * @param {string} subdomain - Subdomain name for overrides
   * @returns {Promise<{protocol: string, port: number}>}
   */
  async detectProtocolAndPort(target, subdomain = '') {
    // Check for explicit override
    if (subdomain && this.portOverrides[subdomain] !== undefined) {
      const port = this.portOverrides[subdomain];
      const protocol = GatusConfigGenerator.PORT_PROTOCOLS[port] || 'tcp';
      return { protocol, port };
    }
    
    // Scan ports in order
    for (const port of GatusConfigGenerator.CHECK_ORDER) {
      const isOpen = await this.checkPort(target, port);
      if (isOpen) {
        const protocol = GatusConfigGenerator.PORT_PROTOCOLS[port] || 'tcp';
        return { protocol, port };
      }
    }
    
    // Fallback to ICMP
    return { protocol: 'icmp', port: 0 };
  }

  /**
   * Get display name for a subdomain, applying overrides.
   * @param {string} subdomain - Subdomain
   * @param {string} defaultName - Default name
   * @returns {string}
   */
  getDisplayName(subdomain, defaultName) {
    if (this.nameOverrides[subdomain]) {
      return this.nameOverrides[subdomain];
    }
    return defaultName;
  }

  /**
   * Determine the group for a target based on IP or domain.
   * @param {string} targetIp - Target IP address
   * @param {string} domain - Domain name
   * @returns {string}
   */
  getGroupForTarget(targetIp, domain) {
    // Try to match IP to a subnet
    for (const [subnet, hostname] of Object.entries(this.subnetToHostname)) {
      if (this.ipInSubnet(targetIp, subnet)) {
        return hostname;
      }
    }
    
    // Fallback: try to find hostname in domain
    for (const hostname of this.hostnames) {
      if (domain.toLowerCase().includes(hostname.toLowerCase())) {
        return hostname;
      }
    }
    
    return 'other';
  }

  /**
   * Check if IP is in subnet.
   * @param {string} ip - IP address
   * @param {string} subnet - Subnet in CIDR notation
   * @returns {boolean}
   */
  ipInSubnet(ip, subnet) {
    const [subnetIp, prefixStr] = subnet.split('/');
    const prefix = Number.parseInt(prefixStr, 10);
    
    const ipNum = this.ipToNumber(ip);
    const subnetNum = this.ipToNumber(subnetIp);
    
    if (ipNum === null || subnetNum === null) return false;
    
    const mask = ~((1 << (32 - prefix)) - 1) >>> 0;
    return (ipNum & mask) === (subnetNum & mask);
  }

  /**
   * Convert IP to number.
   * @param {string} ip - IP address
   * @returns {number|null}
   */
  ipToNumber(ip) {
    const parts = ip.split('.');
    if (parts.length !== 4) return null;
    
    let num = 0;
    for (const part of parts) {
      const val = Number.parseInt(part, 10);
      if (Number.isNaN(val) || val < 0 || val > 255) return null;
      num = (num << 8) + val;
    }
    return num >>> 0;
  }

  /**
   * Check if endpoint should be skipped based on filtering rules.
   * Direct port from script.py should_skip_endpoint
   * @param {string} name - Endpoint name
   * @returns {boolean}
   */
  shouldSkipEndpoint(name) {
    // Skip ignored subdomains
    for (const ignored of this.config.ignoreSubdomains) {
      if (ignored === name || (ignored && name.includes(ignored))) {
        return true;
      }
    }
    
    // Skip technical CNAMEs (on.*, via.*)
    if (this.config.gatusSkipTechnicalCnames) {
      for (const hostname of this.hostnames) {
        if (name.includes(`on.${hostname}`) || name.includes(`via.${hostname}`)) {
          return true;
        }
      }
    }
    
    // Aggressive host filtering
    if (this.config.gatusAggressiveHostFiltering) {
      for (const hostname of this.hostnames) {
        if (name.includes(hostname)) {
          return true;
        }
      }
    }
    
    return false;
  }

  /**
   * Get allowed HTTP codes for a subdomain.
   * Direct port from script.py get_allowed_codes_for_subdomain
   * @param {string} subdomain - Subdomain
   * @param {string} name - Name
   * @returns {number[]}
   */
  getAllowedCodesForSubdomain(subdomain, name) {
    const baseCodes = [...this.config.gatusAllowedHttpCodes];
    const subdomainCodes = this.config.gatusSubdomainHttpCodes;
    
    for (const [code, allowedSubdomains] of Object.entries(subdomainCodes)) {
      const codeNum = Number.parseInt(code, 10);
      if (baseCodes.includes(codeNum)) continue;
      
      for (const pattern of allowedSubdomains) {
        const patternLower = pattern.toLowerCase();
        if (
          subdomain.toLowerCase().includes(patternLower) ||
          name.toLowerCase().includes(patternLower) ||
          subdomain.toLowerCase() === patternLower ||
          name.toLowerCase() === patternLower
        ) {
          baseCodes.push(codeNum);
          break;
        }
      }
    }
    
    return [...new Set(baseCodes)].sort((a, b) => a - b);
  }

  /**
   * Generate a single Gatus endpoint configuration.
   * Direct port from script.py generate_endpoint
   * @param {Object} params - Endpoint parameters
   * @returns {Object}
   */
  generateEndpoint({ name, host, port, protocol, group = '', conditions = null, useInsecureTls = false, subdomain = '' }) {
    const endpoint = { name };
    
    // Construct URL based on protocol
    if (protocol === 'icmp') {
      endpoint.url = `icmp://${host}`;
    } else if (protocol === 'http' || protocol === 'https') {
      endpoint.url = `${protocol}://${host}:${port}`;
    } else if (protocol === 'dns') {
      endpoint.url = `dns://${host}`;
    } else {
      endpoint.url = `${protocol}://${host}:${port}`;
    }
    
    if (group) {
      endpoint.group = group;
    }
    
    // Add conditions
    if (conditions) {
      endpoint.conditions = conditions;
    } else if (protocol === 'http' || protocol === 'https') {
      const allowedCodes = this.getAllowedCodesForSubdomain(subdomain || name, name);
      if (allowedCodes.length === 1) {
        endpoint.conditions = [`[STATUS] == ${allowedCodes[0]}`];
      } else {
        const codesStr = allowedCodes.join(', ');
        endpoint.conditions = [`[STATUS] == any(${codesStr})`];
      }
    } else {
      endpoint.conditions = ['[CONNECTED] == true'];
    }
    
    // Add defaults
    if (this.defaults.interval) {
      endpoint.interval = this.defaults.interval;
    }
    
    // Handle client configuration
    if (this.defaults.client || useInsecureTls) {
      const clientConfig = { ...this.defaults.client };
      if (useInsecureTls) {
        clientConfig.insecure = true;
      }
      endpoint.client = clientConfig;
    }
    
    if (this.defaults.alerts) {
      endpoint.alerts = this.defaults.alerts;
    }
    
    return endpoint;
  }

  /**
   * Generate complete Gatus configuration.
   * Direct port from script.py generate_config
   * @param {Array} pangolinEntries - Pangolin entries
   * @param {Array} hetznerEntries - Hetzner entries
   * @returns {Promise<Object>}
   */
  async generateConfig(pangolinEntries, hetznerEntries) {
    const endpoints = [];
    const pangolinSubdomains = new Set();
    
    // Process Pangolin entries
    for (const entry of pangolinEntries) {
      const name = entry.name || entry.domain || 'Unknown';
      const domain = entry.domain || '';
      const targetIp = entry.target || '';
      const port = entry.port || 443;
      const protocol = entry.protocol || 'https';
      
      const subdomain = domain.replace(`.${this.domain}`, '');
      pangolinSubdomains.add(subdomain);
      pangolinSubdomains.add(domain);
      
      if (this.shouldSkipEndpoint(name) || this.shouldSkipEndpoint(domain)) {
        continue;
      }
      
      if (!targetIp) {
        continue;
      }
      
      const group = this.getGroupForTarget(targetIp, domain);
      const useInsecure = protocol === 'https';
      
      const endpoint = this.generateEndpoint({
        name,
        host: targetIp,
        port,
        protocol,
        group,
        useInsecureTls: useInsecure,
        subdomain,
      });
      
      endpoints.push(endpoint);
    }
    
    // Process Hetzner entries
    const keptSubdomains = this.config.keepSubdomains;
    
    for (const entry of hetznerEntries) {
      const recordName = entry.name || '';
      const recordType = entry.type || '';
      const target = entry.value || '';
      
      const isKept = keptSubdomains.includes(recordName);
      if (!isKept) {
        if (recordName.startsWith('on.') || target.startsWith('on.')) {
          continue;
        }
      }
      
      if (pangolinSubdomains.has(recordName) && !isKept) {
        continue;
      }
      
      let fullDomain;
      if (recordName === '@') {
        fullDomain = this.domain;
      } else {
        fullDomain = `${recordName}.${this.domain}`;
      }
      
      if (pangolinSubdomains.has(fullDomain)) {
        continue;
      }
      
      if (recordType !== 'A' && recordType !== 'CNAME') {
        continue;
      }
      
      if (this.shouldSkipEndpoint(recordName) || this.shouldSkipEndpoint(fullDomain)) {
        continue;
      }
      
      const subdomain = recordName !== '@' ? recordName : '';
      const displayName = this.getDisplayName(subdomain, fullDomain);
      
      const detectTarget = recordType === 'CNAME' ? target.replace(/\.$/, '') : fullDomain;
      const { protocol, port } = await this.detectProtocolAndPort(detectTarget, recordName);
      
      let group = 'other';
      const targetVal = target.replace(/\.$/, '');
      let foundGroupViaTarget = false;
      
      for (const hostname of this.hostnames) {
        if (targetVal.includes(`on.${hostname}`)) {
          group = hostname;
          foundGroupViaTarget = true;
          break;
        }
      }
      
      if (!foundGroupViaTarget) {
        for (const hostname of this.hostnames) {
          if (fullDomain.toLowerCase().includes(hostname.toLowerCase())) {
            group = hostname;
            break;
          }
        }
      }
      
      const endpoint = this.generateEndpoint({
        name: displayName,
        host: fullDomain,
        port,
        protocol,
        group,
        subdomain: recordName,
      });
      
      endpoints.push(endpoint);
    }
    
    return { endpoints };
  }
}
