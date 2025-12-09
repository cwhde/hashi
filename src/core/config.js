/**
 * Configuration handler for the DNS sync application.
 * Direct port from script.py lines 38-92
 */

import fs from 'fs';
import yaml from 'js-yaml';

export class Config {
  constructor(configPath = 'config.yml') {
    this.configPath = configPath;
    this.config = this._loadConfig();
  }

  _loadConfig() {
    try {
      const content = fs.readFileSync(this.configPath, 'utf8');
      return yaml.load(content);
    } catch (error) {
      if (error.code === 'ENOENT') {
        throw new Error(`Configuration file not found: ${this.configPath}`);
      }
      throw new Error(`Error parsing configuration file: ${error.message}`);
    }
  }

  reload() {
    this.config = this._loadConfig();
  }

  save() {
    const content = yaml.dump(this.config, { 
      lineWidth: -1,
      noRefs: true,
      quotingType: '"',
    });
    fs.writeFileSync(this.configPath, content, 'utf8');
  }

  backup() {
    const backupPath = `${this.configPath}.backup`;
    fs.copyFileSync(this.configPath, backupPath);
    return backupPath;
  }

  restore() {
    const backupPath = `${this.configPath}.backup`;
    if (fs.existsSync(backupPath)) {
      fs.copyFileSync(backupPath, this.configPath);
      this.reload();
      return true;
    }
    return false;
  }

  // Auth section (new for web app)
  get auth() {
    return this.config.auth || {};
  }

  set auth(value) {
    this.config.auth = value;
  }

  // General settings
  get domain() {
    return this.config.general?.domain || '';
  }

  get topologySource() {
    return this.config.general?.topology_source || '';
  }

  get resolverIp() {
    return this.config.general?.resolver_ip || '9.9.9.9';
  }

  get gatusOutputPath() {
    return this.config.general?.gatus_output_path || '/gatus/endpoints.yaml';
  }

  get loopInterval() {
    return this.config.general?.loop_interval || 300;
  }

  get nameOverrides() {
    return this.config.general?.name_overrides || {};
  }

  get keepSubdomains() {
    return this.config.general?.keep_subdomains || [];
  }

  get ignoreSubdomains() {
    return this.config.general?.ignore_subdomains || [];
  }

  // Pangolin API settings
  get pangolinBaseUrl() {
    return this.config.apis?.pangolin?.base_url || '';
  }

  get pangolinAuthToken() {
    return this.config.apis?.pangolin?.auth_token || '';
  }

  get pangolinOrgId() {
    return this.config.apis?.pangolin?.org_id || '';
  }

  // Hetzner API settings
  get hetznerAuthToken() {
    return this.config.apis?.hetzner?.auth_token || '';
  }

  get hetznerZoneId() {
    return this.config.apis?.hetzner?.zone_id || '';
  }

  // Gatus defaults
  get gatusDefaults() {
    return this.config.gatus_defaults || {};
  }

  get gatusAllowedHttpCodes() {
    return this.config.gatus_defaults?.allowed_http_codes || [200];
  }

  get gatusSubdomainHttpCodes() {
    const raw = this.config.gatus_defaults?.subdomain_http_codes || {};
    const result = {};
    for (const [code, subdomains] of Object.entries(raw)) {
      result[parseInt(code)] = Array.isArray(subdomains) ? subdomains : [subdomains];
    }
    return result;
  }

  get gatusSubdomainPortOverrides() {
    return this.config.gatus_defaults?.subdomain_port_overrides || {};
  }

  get gatusSkipTechnicalCnames() {
    return this.config.gatus_defaults?.skip_technical_cnames ?? true;
  }

  get gatusAggressiveHostFiltering() {
    return this.config.gatus_defaults?.aggressive_host_filtering ?? false;
  }

  // Get config for API response (with masked sensitive fields)
  toJSON(maskSensitive = true) {
    const config = JSON.parse(JSON.stringify(this.config));
    
    if (maskSensitive) {
      // Mask sensitive fields
      if (config.apis?.pangolin?.auth_token) {
        config.apis.pangolin.auth_token = '••••••••••••';
      }
      if (config.apis?.hetzner?.auth_token) {
        config.apis.hetzner.auth_token = '••••••••••••';
      }
      if (config.auth?.password_hash) {
        delete config.auth.password_hash;
      }
      if (config.auth?.password) {
        delete config.auth.password;
      }
    }
    
    return config;
  }

  // Update config from partial object
  update(partial) {
    const merge = (target, source) => {
      for (const key of Object.keys(source)) {
        if (source[key] === null || source[key] === undefined) {
          continue;
        }
        if (typeof source[key] === 'object' && !Array.isArray(source[key])) {
          if (!target[key]) target[key] = {};
          merge(target[key], source[key]);
        } else {
          // Don't update if the value is the masked placeholder
          if (source[key] !== '••••••••••••') {
            target[key] = source[key];
          }
        }
      }
    };
    
    merge(this.config, partial);
  }
}
