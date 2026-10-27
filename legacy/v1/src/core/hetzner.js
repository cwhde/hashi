/**
 * Hetzner DNS API client.
 * Direct port from script.py - uses Hetzner Cloud API for DNS management
 */

export class HetznerDNS {
  // Using the Hetzner Cloud API base URL (modern DNS management)
  static BASE_URL = 'https://api.hetzner.cloud/v1';

  constructor(authToken, zoneId = '') {
    this.authToken = authToken;
    this.zoneId = zoneId;
  }

  /**
   * Make an authenticated request to the Hetzner Cloud API.
   * @param {string} path - API path
   * @param {object} options - Fetch options
   * @returns {Promise<any>}
   */
  async request(path, options = {}) {
    const url = `${HetznerDNS.BASE_URL}${path}`;
    const response = await fetch(url, {
      ...options,
      headers: {
        'Authorization': `Bearer ${this.authToken}`,
        'Content-Type': 'application/json',
        ...options.headers,
      },
    });
    
    if (!response.ok) {
      const text = await response.text();
      throw new Error(`Hetzner DNS API error: ${response.status} ${response.statusText} - ${text}`);
    }
    
    // Handle DELETE responses which may have no content
    const contentType = response.headers.get('content-type');
    if (contentType && contentType.includes('application/json')) {
      return response.json();
    }
    return {};
  }

  /**
   * Get zone ID by domain name.
   * @param {string} domain - Domain name
   * @param {function} logger - Logger function
   * @returns {Promise<string|null>}
   */
  async getZoneId(domain, logger) {
    if (this.zoneId) {
      return this.zoneId;
    }
    
    try {
      const data = await this.request(`/zones?name=${encodeURIComponent(domain)}`);
      const zones = data.zones || [];
      
      if (zones.length > 0) {
        const zoneId = String(zones[0].id);
        logger.info(`Found zone ID ${zoneId} for domain ${domain}`);
        return zoneId;
      }
      
      logger.error(`No zone found for domain ${domain}`);
      return null;
    } catch (error) {
      logger.error(`Failed to get zone ID: ${error.message}`);
      return null;
    }
  }

  /**
   * List all RRSets for a zone.
   * @param {string} zoneId - Zone ID
   * @param {function} logger - Logger function
   * @returns {Promise<Array>}
   */
  async listRecords(zoneId, logger) {
    try {
      const allRecords = [];
      let page = 1;
      const perPage = 100;
      
      while (true) {
        const data = await this.request(`/zones/${zoneId}/rrsets?page=${page}&per_page=${perPage}`);
        const rrsets = data.rrsets || [];
        
        // Convert RRSets to flat record format for compatibility
        for (const rrset of rrsets) {
          const records = rrset.records || [];
          for (const record of records) {
            allRecords.push({
              id: `${rrset.name}:${rrset.type}`,
              name: rrset.name,
              type: rrset.type,
              value: record.value,
              ttl: rrset.ttl,
              zone_id: zoneId,
            });
          }
        }
        
        // Check if there are more pages
        const meta = data.meta || {};
        const pagination = meta.pagination || {};
        if (page >= (pagination.last_page || 1)) {
          break;
        }
        page++;
      }
      
      return allRecords;
    } catch (error) {
      logger.error(`Failed to list records: ${error.message}`);
      return [];
    }
  }

  /**
   * Create a new DNS record (RRSet).
   * @param {string} zoneId - Zone ID
   * @param {string} name - Record name
   * @param {string} recordType - Record type (A, CNAME, etc.)
   * @param {string} value - Record value
   * @param {number} ttl - TTL in seconds
   * @param {function} logger - Logger function
   * @returns {Promise<boolean>}
   */
  async createRecord(zoneId, name, recordType, value, ttl, logger) {
    try {
      // For CNAME, ensure the value ends with a dot
      let finalValue = value;
      if (recordType === 'CNAME' && !value.endsWith('.')) {
        finalValue = `${value}.`;
      }
      
      const payload = {
        name: name,
        type: recordType,
        ttl: ttl,
        records: [{ value: finalValue }],
      };
      
      await this.request(`/zones/${zoneId}/rrsets`, {
        method: 'POST',
        body: JSON.stringify(payload),
      });
      
      logger.info(`Created ${recordType} record: ${name} -> ${value}`);
      return true;
    } catch (error) {
      logger.error(`Failed to create record ${name}: ${error.message}`);
      return false;
    }
  }

  /**
   * Update an existing DNS record (RRSet) by setting new records.
   * @param {string} recordId - Record ID (format: "name:type")
   * @param {string} zoneId - Zone ID
   * @param {string} name - Record name
   * @param {string} recordType - Record type
   * @param {string} value - New value
   * @param {number} ttl - TTL in seconds
   * @param {function} logger - Logger function
   * @returns {Promise<boolean>}
   */
  async updateRecord(recordId, zoneId, name, recordType, value, ttl, logger) {
    try {
      // For CNAME, ensure the value ends with a dot
      let finalValue = value;
      if (recordType === 'CNAME' && !value.endsWith('.')) {
        finalValue = `${value}.`;
      }
      
      const payload = {
        records: [{ value: finalValue }],
      };
      
      await this.request(`/zones/${zoneId}/rrsets/${name}/${recordType}/actions/set_records`, {
        method: 'POST',
        body: JSON.stringify(payload),
      });
      
      logger.info(`Updated ${recordType} record: ${name} -> ${value}`);
      return true;
    } catch (error) {
      logger.error(`Failed to update record ${name}: ${error.message}`);
      return false;
    }
  }

  /**
   * Delete a DNS record (RRSet).
   * @param {string} recordId - Record ID (format: "name:type")
   * @param {function} logger - Logger function
   * @param {string} zoneId - Zone ID (optional, uses stored zoneId if not provided)
   * @returns {Promise<boolean>}
   */
  async deleteRecord(recordId, logger, zoneId = null) {
    try {
      // recordId is in format "name:type"
      const [name, recordType] = recordId.split(':');
      const zone = zoneId || this.zoneId;
      
      await this.request(`/zones/${zone}/rrsets/${name}/${recordType}`, {
        method: 'DELETE',
      });
      
      logger.info(`Deleted ${recordType} record: ${name}`);
      return true;
    } catch (error) {
      logger.error(`Failed to delete record ${recordId}: ${error.message}`);
      return false;
    }
  }

  /**
   * Find a record by name and type.
   * @param {Array} records - Array of records
   * @param {string} name - Record name to find
   * @param {string} recordType - Record type
   * @returns {object|null}
   */
  findRecord(records, name, recordType) {
    return records.find(r => r.name === name && r.type === recordType) || null;
  }
}
