/**
 * DNS Sync orchestrator.
 * Direct port from script.py lines 643-850
 */

import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import yaml from 'js-yaml';

import { TopologyResolver } from './topology.js';
import { PangolinAPI } from './pangolin.js';
import { HetznerDNS } from './hetzner.js';
import { GatusConfigGenerator } from './gatus.js';

export class DNSSync {
  constructor(config, logger) {
    this.config = config;
    this.logger = logger;
    
    this.topology = new TopologyResolver(
      config.topologySource,
      config.resolverIp,
      config.domain
    );
    
    this.pangolin = new PangolinAPI(
      config.pangolinBaseUrl,
      config.pangolinAuthToken
    );
    
    this.hetzner = new HetznerDNS(
      config.hetznerAuthToken,
      config.hetznerZoneId
    );
    
    this.gatusGenerator = new GatusConfigGenerator(config);
  }

  /**
   * Reload configuration and reinitialize clients.
   */
  reloadConfig() {
    this.config.reload();
    
    this.topology = new TopologyResolver(
      this.config.topologySource,
      this.config.resolverIp,
      this.config.domain
    );
    
    this.pangolin = new PangolinAPI(
      this.config.pangolinBaseUrl,
      this.config.pangolinAuthToken
    );
    
    this.hetzner = new HetznerDNS(
      this.config.hetznerAuthToken,
      this.config.hetznerZoneId
    );
    
    this.gatusGenerator = new GatusConfigGenerator(this.config);
  }

  /**
   * Get domain-to-CNAME pairs from Pangolin resources.
   * Direct port from script.py get_pangolin_domain_cname_pairs
   * @returns {Promise<{pairs: Array, hostMapping: Object}>}
   */
  async getPangolinDomainCnamePairs() {
    // Get topology mapping
    this.logger.info('Getting host-to-subnet mapping from topology...');
    const hostMapping = await this.topology.getHostSubnetMapping(this.logger);
    
    if (Object.keys(hostMapping).length === 0) {
      this.logger.warn('No host-to-subnet mapping available');
      return { pairs: [], hostMapping: {} };
    }
    
    this.logger.info(`Host mappings: ${JSON.stringify(hostMapping)}`);
    
    // Get Pangolin org ID
    this.logger.info('Getting Pangolin organization ID...');
    const orgId = await this.pangolin.getOrgId(this.config.pangolinOrgId, this.logger);
    
    if (!orgId) {
      this.logger.error('Failed to get Pangolin org ID, cannot continue');
      return { pairs: [], hostMapping };
    }
    
    // Get all resources
    this.logger.info(`Listing resources for org ${orgId}...`);
    const resources = await this.pangolin.listResources(orgId, this.logger);
    this.logger.info(`Found ${resources.length} Pangolin resources`);
    
    if (resources.length === 0) {
      this.logger.warn('No resources found in Pangolin');
      return { pairs: [], hostMapping };
    }
    
    const pairs = [];
    
    for (let i = 0; i < resources.length; i++) {
      const resource = resources[i];
      const resourceId = resource.resourceId || resource.id;
      const resourceName = resource.name || 'Unknown';
      
      this.logger.debug(`Processing resource ${i + 1}/${resources.length}: ${resourceName} (ID: ${resourceId})`);
      
      // Get domain from resource
      let domain = resource.fullDomain || resource.domain || resource.subdomain || '';
      
      if (!domain) {
        this.logger.debug(`Resource ${resourceName} has no domain field, skipping`);
        continue;
      }
      
      // Ensure domain includes the base domain
      if (!domain.endsWith(this.config.domain)) {
        domain = `${domain}.${this.config.domain}`;
      }
      
      // Fetch targets from API
      this.logger.debug(`Fetching targets for resource ${resourceId}...`);
      let targets = await this.pangolin.getResourceTargets(resourceId, this.logger);
      
      if (targets.length === 0) {
        // Fallback to embedded targets
        targets = resource.targets || [];
        this.logger.debug(`No targets from API, using ${targets.length} embedded targets`);
      }
      
      if (targets.length === 0) {
        this.logger.debug(`Resource ${resourceName} has no targets, skipping`);
        continue;
      }
      
      // Use first enabled target
      let target = targets.find(t => t.enabled !== false) || targets[0];
      
      const targetIp = target.ip || '';
      const targetPort = target.port || 443;
      
      // Determine protocol
      let targetProtocol;
      const targetMethod = (target.method || '').toLowerCase();
      
      if (targetMethod === 'http' || targetMethod === 'https') {
        targetProtocol = targetMethod;
      } else {
        const resProto = (resource.protocol || '').toLowerCase();
        if (resProto === 'http' || resProto === 'https') {
          targetProtocol = resProto;
        } else if (targetPort === 443 || targetPort === 8006) {
          targetProtocol = 'https';
        } else if (targetPort === 80) {
          targetProtocol = 'http';
        } else {
          targetProtocol = 'tcp';
        }
      }
      
      if (!targetIp) {
        this.logger.debug('Target has no IP, skipping');
        continue;
      }
      
      // Get CNAME for this IP
      const cname = this.topology.getCnameForIp(targetIp, hostMapping);
      if (!cname) {
        this.logger.warn(`No subnet match for ${resourceName} with IP ${targetIp}`);
        continue;
      }
      
      // Extract subdomain
      const subdomain = domain.replace(`.${this.config.domain}`, '');
      const isRoot = subdomain === this.config.domain || subdomain === '';
      
      pairs.push({
        name: resourceName,
        domain,
        subdomain,
        cname,
        cname_full: `${cname}.${this.config.domain}`,
        is_root: isRoot,
        resource_name: resourceName,
        target: targetIp,
        port: targetPort,
        protocol: targetProtocol,
      });
      
      this.logger.info(`Mapped ${domain} -> ${cname}.${this.config.domain} (IP: ${targetIp}, Proto: ${targetProtocol})`);
    }
    
    this.logger.info(`Total pairs created: ${pairs.length}`);
    return { pairs, hostMapping };
  }

  /**
   * Synchronize Hetzner DNS with Pangolin CNAME mappings.
   * Direct port from script.py sync_hetzner_dns
   * @param {Array} pangolinPairs - Domain-to-CNAME pairs from Pangolin
   * @returns {Promise<Array>} - All Hetzner DNS records
   */
  async syncHetznerDns(pangolinPairs) {
    // Get Hetzner zone ID
    const zoneId = await this.hetzner.getZoneId(this.config.domain, this.logger);
    if (!zoneId) {
      return [];
    }
    
    // Get current DNS records
    const currentRecords = await this.hetzner.listRecords(zoneId, this.logger);
    this.logger.info(`Found ${currentRecords.length} existing DNS records`);
    
    // Build lookup of current CNAME records pointing to on.*
    const currentCnames = new Map();
    
    for (const record of currentRecords) {
      if (record.type !== 'CNAME') continue;
      
      const name = record.name || '';
      const value = (record.value || '').replace(/\.$/, '');
      
      if (value.startsWith('on.') || value.includes('.on.')) {
        currentCnames.set(name, { value, id: record.id });
      }
    }
    
    this.logger.info(`Found ${currentCnames.size} existing on.* CNAME records`);
    
    // Build expected records from Pangolin
    const expectedCnames = new Map();
    
    for (const pair of pangolinPairs) {
      const subdomain = pair.subdomain;
      const cnameTarget = pair.cname_full;
      const isRoot = pair.is_root;
      
      // Check ignore list
      if (this.config.ignoreSubdomains.includes(subdomain)) {
        this.logger.info(`Skipping ignored subdomain: ${subdomain}`);
        continue;
      }
      
      // Skip root domain if in ignore list
      if (isRoot && (
        this.config.ignoreSubdomains.includes('@') ||
        this.config.ignoreSubdomains.includes('') ||
        this.config.ignoreSubdomains.includes(this.config.domain)
      )) {
        this.logger.info('Skipping root domain (ignored)');
        continue;
      }
      
      const recordName = isRoot ? '@' : subdomain;
      expectedCnames.set(recordName, cnameTarget);
    }
    
    const keptSubdomains = this.config.keepSubdomains;
    
    // Delete orphaned records
    for (const [name, { value, id }] of currentCnames) {
      if (!expectedCnames.has(name)) {
        if (keptSubdomains.includes(name)) {
          this.logger.info(`Skipping deletion of explicitly kept subdomain: ${name}`);
          continue;
        }
        
        this.logger.info(`Deleting orphaned CNAME: ${name} -> ${value}`);
        await this.hetzner.deleteRecord(id, this.logger, zoneId);
      }
    }
    
    // Update or create records
    for (const [name, target] of expectedCnames) {
      const current = currentCnames.get(name);
      const targetClean = target.replace(/\.$/, '');
      
      if (current) {
        if (current.value !== targetClean) {
          this.logger.info(`Updating CNAME: ${name} (${current.value} -> ${targetClean})`);
          await this.hetzner.updateRecord(current.id, zoneId, name, 'CNAME', target, 3600, this.logger);
        } else {
          this.logger.debug(`CNAME unchanged: ${name} -> ${targetClean}`);
        }
      } else {
        this.logger.info(`Creating new CNAME: ${name} -> ${targetClean}`);
        await this.hetzner.createRecord(zoneId, name, 'CNAME', target, 3600, this.logger);
      }
    }
    
    // Return refreshed records
    return this.hetzner.listRecords(zoneId, this.logger);
  }

  /**
   * Generate Gatus configuration.
   * Direct port from script.py generate_gatus_config
   * @param {Array} pangolinPairs - Pangolin pairs
   * @param {Array} hetznerRecords - Hetzner records
   * @returns {Promise<Object>}
   */
  async generateGatusConfig(pangolinPairs, hetznerRecords) {
    // Prepare Pangolin entries
    const pangolinEntries = pangolinPairs.map(pair => ({
      name: pair.resource_name,
      domain: pair.domain,
      target: pair.target,
      port: pair.port,
      protocol: pair.protocol,
    }));
    
    // Prepare Hetzner entries
    const pangolinSubdomains = new Set(pangolinPairs.map(p => p.subdomain));
    
    const hetznerEntries = [];
    for (const record of hetznerRecords) {
      const recordType = record.type || '';
      const name = record.name || '';
      const value = (record.value || '').replace(/\.$/, '');
      
      if (recordType !== 'A' && recordType !== 'CNAME') continue;
      if (pangolinSubdomains.has(name)) continue;
      
      if (name.startsWith('on.') && !this.config.keepSubdomains.includes(name)) {
        continue;
      }
      
      hetznerEntries.push({
        name,
        type: recordType,
        value,
      });
    }
    
    return this.gatusGenerator.generateConfig(pangolinEntries, hetznerEntries);
  }

  /**
   * Write Gatus configuration to file only if changed.
   * Direct port from script.py write_gatus_config
   * @param {Object} config - Gatus configuration
   * @returns {boolean} - True if file was written
   */
  writeGatusConfig(config) {
    const outputPath = this.config.gatusOutputPath;
    const newContent = yaml.dump(config, { lineWidth: -1, noRefs: true });
    
    // Check if file exists and compare
    try {
      const existingContent = fs.readFileSync(outputPath, 'utf8');
      if (existingContent === newContent) {
        this.logger.info('Gatus configuration unchanged, skipping write');
        return false;
      }
    } catch {
      // File doesn't exist
    }
    
    // Write to temp file first, then move (use same directory to avoid EXDEV cross-device error)
    let tmpPath = null;
    try {
      const dir = path.dirname(outputPath) || '.';
      if (dir && dir !== '.') {
        fs.mkdirSync(dir, { recursive: true });
      }
      
      // Create temp file in the SAME directory as output to avoid cross-device rename issues
      tmpPath = path.join(dir, `gatus-${Date.now()}.yaml.tmp`);
      fs.writeFileSync(tmpPath, newContent, 'utf8');
      
      // Use fs.renameSync for atomic move (works when on same filesystem)
      fs.renameSync(tmpPath, outputPath);
      
      this.logger.info(`Wrote Gatus configuration to ${outputPath}`);
      return true;
    } catch (error) {
      this.logger.error(`Failed to write Gatus configuration: ${error.message}`);
      // Clean up temp file if it exists
      if (tmpPath) {
        try {
          fs.unlinkSync(tmpPath);
        } catch {
          // Ignore cleanup errors
        }
      }
      return false;
    }
  }

  /**
   * Run a single synchronization cycle.
   * Direct port from script.py run_once
   * @returns {Promise<{success: boolean, summary: Object}>}
   */
  async runOnce() {
    const summary = {
      pangolinMappings: 0,
      dnsRecords: 0,
      gatusEndpoints: 0,
      errors: [],
    };
    
    try {
      this.logger.info('Starting DNS sync cycle');
      
      // Step 1: Get Pangolin domain-to-CNAME pairs
      this.logger.info('Step 1: Fetching Pangolin domain-to-CNAME pairs...');
      const { pairs: pangolinPairs, hostMapping } = await this.getPangolinDomainCnamePairs();
      summary.pangolinMappings = pangolinPairs.length;
      this.logger.info(`Found ${pangolinPairs.length} Pangolin domain mappings`);
      
      // Update Gatus generator with host mapping
      this.gatusGenerator.updateHostMapping(hostMapping);
      
      if (pangolinPairs.length > 0) {
        for (const pair of pangolinPairs) {
          this.logger.debug(`  - ${pair.domain} -> ${pair.cname_full} (resource: ${pair.resource_name})`);
        }
      }
      
      // Step 2: Sync Hetzner DNS
      this.logger.info('Step 2: Synchronizing Hetzner DNS...');
      const hetznerRecords = await this.syncHetznerDns(pangolinPairs);
      summary.dnsRecords = hetznerRecords.length;
      this.logger.info(`DNS sync complete, ${hetznerRecords.length} total records`);
      
      // Step 3: Generate and write Gatus config
      this.logger.info('Step 3: Generating Gatus configuration...');
      const gatusConfig = await this.generateGatusConfig(pangolinPairs, hetznerRecords);
      summary.gatusEndpoints = gatusConfig.endpoints?.length || 0;
      this.logger.info(`Generated ${summary.gatusEndpoints} Gatus endpoints`);
      this.writeGatusConfig(gatusConfig);
      
      this.logger.success('DNS sync cycle completed successfully');
      return { success: true, summary };
      
    } catch (error) {
      this.logger.error(`DNS sync cycle failed: ${error.message}`);
      this.logger.error(`Stack trace: ${error.stack}`);
      summary.errors.push(error.message);
      return { success: false, summary };
    }
  }
}
