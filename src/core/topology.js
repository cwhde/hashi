/**
 * Topology resolver - queries DNS TXT records to get host-to-subnet mappings.
 * Direct port from script.py lines 95-167
 */

import { queryTXT } from '../utils/dns.js';

export class TopologyResolver {
  constructor(topologySource, resolverIp, domain) {
    this.topologySource = topologySource;
    this.resolverIp = resolverIp;
    this.domain = domain;
  }

  /**
   * Convert IP to /24 subnet string.
   * @param {string} ip - IP address
   * @returns {string|null} - Subnet in CIDR notation or null
   */
  ipToSubnet(ip) {
    const parts = ip.split('.');
    if (parts.length !== 4) return null;
    
    const valid = parts.every(part => {
      const num = Number.parseInt(part, 10);
      return !Number.isNaN(num) && num >= 0 && num <= 255;
    });
    
    if (!valid) return null;
    
    // Convert to /24 subnet
    return `${parts[0]}.${parts[1]}.${parts[2]}.0/24`;
  }

  /**
   * Check if an IP is within a subnet.
   * @param {string} ip - IP address
   * @param {string} subnet - Subnet in CIDR notation (e.g., "10.0.4.0/24")
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
   * Convert IP to 32-bit number.
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
    return num >>> 0; // Ensure unsigned
  }

  /**
   * Query the topology source and return a mapping of CNAME prefixes to /24 subnets.
   * @param {function} logger - Logger function
   * @returns {Promise<Object>} - Map of "on.<hostname>" to "/24 subnet"
   */
  async getHostSubnetMapping(logger) {
    try {
      logger.info(`Querying TXT record for ${this.topologySource} via ${this.resolverIp}`);
      
      const txtData = await queryTXT(this.topologySource, this.resolverIp);
      logger.info(`Received topology data: ${txtData}`);
      
      // Parse format: "hostname:ip,hostname2:ip2,..."
      const mapping = {};
      const entries = txtData.split(',');
      
      for (const entry of entries) {
        const trimmed = entry.trim();
        if (!trimmed.includes(':')) continue;
        
        const [hostname, ip] = trimmed.split(':', 2);
        const cleanHostname = hostname.trim();
        const cleanIp = ip.trim();
        
        const subnet = this.ipToSubnet(cleanIp);
        if (subnet) {
          const cnamePrefix = `on.${cleanHostname}`;
          mapping[cnamePrefix] = subnet;
          logger.debug(`Mapped ${cnamePrefix} -> ${subnet}`);
        } else {
          logger.warn(`Invalid IP address ${cleanIp} for host ${cleanHostname}`);
        }
      }
      
      logger.info(`Resolved ${Object.keys(mapping).length} host-to-subnet mappings`);
      return mapping;
      
    } catch (error) {
      logger.error(`Error querying topology: ${error.message}`);
      return {};
    }
  }

  /**
   * Get the CNAME prefix for a given IP based on subnet mapping.
   * @param {string} ip - IP address to look up
   * @param {Object} mapping - Dict of CNAME prefixes to subnets
   * @returns {string|null} - CNAME prefix (e.g., "on.kanae") or null
   */
  getCnameForIp(ip, mapping) {
    for (const [cnamePrefix, subnet] of Object.entries(mapping)) {
      if (this.ipInSubnet(ip, subnet)) {
        return cnamePrefix;
      }
    }
    return null;
  }
}
