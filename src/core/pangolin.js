/**
 * Pangolin API client.
 * Direct port from script.py lines 170-278
 */

export class PangolinAPI {
  constructor(baseUrl, authToken) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.authToken = authToken;
  }

  /**
   * Make an authenticated request to the Pangolin API.
   * @param {string} path - API path
   * @param {object} options - Fetch options
   * @returns {Promise<any>}
   */
  async request(path, options = {}) {
    const url = `${this.baseUrl}${path}`;
    const response = await fetch(url, {
      ...options,
      headers: {
        'Authorization': `Bearer ${this.authToken}`,
        'Content-Type': 'application/json',
        ...options.headers,
      },
    });
    
    if (!response.ok) {
      throw new Error(`Pangolin API error: ${response.status} ${response.statusText}`);
    }
    
    return response.json();
  }

  /**
   * List all organizations.
   * @param {function} logger - Logger function
   * @returns {Promise<Array>}
   */
  async listOrgs(logger) {
    logger.info(`Fetching organizations from ${this.baseUrl}/orgs`);
    
    try {
      const data = await this.request('/orgs');
      logger.debug(`Response data: ${JSON.stringify(data)}`);
      
      // Handle various API response formats
      let orgs = [];
      if (Array.isArray(data)) {
        orgs = data;
      } else if (typeof data === 'object') {
        if (data.data?.orgs) {
          // Nested format: data.orgs
          orgs = data.data.orgs;
        } else if (Array.isArray(data.data)) {
          // Simple format: data is array
          orgs = data.data;
        } else if (data.orgs) {
          // Direct format: orgs at top level
          orgs = data.orgs;
        }
      }
      
      logger.info(`Found ${orgs.length} organization(s)`);
      for (const org of orgs) {
        const orgId = org.orgId || org.id || 'unknown';
        const name = org.name || 'unnamed';
        logger.debug(`  Org: ${orgId} - ${name}`);
      }
      
      return orgs;
    } catch (error) {
      logger.error(`Failed to list organizations: ${error.message}`);
      return [];
    }
  }

  /**
   * Get the organization ID, either from config or by listing orgs.
   * @param {string} specifiedOrgId - Org ID from config (optional)
   * @param {function} logger - Logger function
   * @returns {Promise<string|null>}
   */
  async getOrgId(specifiedOrgId, logger) {
    if (specifiedOrgId) {
      logger.info(`Using specified org_id: ${specifiedOrgId}`);
      return specifiedOrgId;
    }
    
    logger.info('No org_id specified, listing organizations...');
    const orgs = await this.listOrgs(logger);
    
    if (orgs.length > 0) {
      const firstOrg = orgs[0];
      logger.debug(`First org data: ${JSON.stringify(firstOrg)}`);
      const orgId = firstOrg.orgId || firstOrg.id || firstOrg.org_id;
      
      if (orgId) {
        logger.info(`Using first organization: ${orgId}`);
        return orgId;
      }
      logger.error(`Could not extract org ID from org data: ${JSON.stringify(firstOrg)}`);
      return null;
    }
    
    logger.error('No organizations found');
    return null;
  }

  /**
   * List all resources for an organization.
   * @param {string} orgId - Organization ID
   * @param {function} logger - Logger function
   * @returns {Promise<Array>}
   */
  async listResources(orgId, logger) {
    const url = `/org/${orgId}/resources`;
    logger.info(`Fetching resources from ${this.baseUrl}${url}`);
    
    try {
      const data = await this.request(url);
      logger.debug(`Response status: OK`);
      
      // Handle various API response formats
      let resources = [];
      if (Array.isArray(data)) {
        resources = data;
      } else if (typeof data === 'object') {
        if (data.data?.resources) {
          resources = data.data.resources;
        } else if (Array.isArray(data.data)) {
          resources = data.data;
        } else if (data.resources) {
          resources = data.resources;
        }
      }
      
      logger.info(`Found ${resources.length} resource(s)`);
      return resources;
    } catch (error) {
      logger.error(`Failed to list resources for org ${orgId}: ${error.message}`);
      return [];
    }
  }

  /**
   * Get targets for a specific resource.
   * @param {string} resourceId - Resource ID
   * @param {function} logger - Logger function
   * @returns {Promise<Array>}
   */
  async getResourceTargets(resourceId, logger) {
    try {
      const data = await this.request(`/resource/${resourceId}/targets`);
      
      // Handle nested format
      if (Array.isArray(data)) {
        return data;
      }
      if (typeof data === 'object') {
        if (data.data?.targets) {
          return data.data.targets;
        }
        if (Array.isArray(data.data)) {
          return data.data;
        }
        if (data.targets) {
          return data.targets;
        }
      }
      
      return [];
    } catch (error) {
      logger.error(`Failed to get targets for resource ${resourceId}: ${error.message}`);
      return [];
    }
  }
}
