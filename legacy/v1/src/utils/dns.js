/**
 * DNS utility functions using dns-packet over UDP to Quad9.
 * Avoids split-horizon issues with system resolver.
 */

import dgram from 'node:dgram';
import dnsPacket from 'dns-packet';

const DEFAULT_RESOLVER = '9.9.9.9';
const DNS_PORT = 53;
const TIMEOUT_MS = 10000;  // Increased from 5000 to handle slower networks
const MAX_RETRIES = 3;
const RETRY_DELAY_MS = 2000;

/**
 * Sleep for a given number of milliseconds.
 */
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Single attempt to query TXT records from a DNS server.
 */
async function queryTXTOnce(domain, resolverIp) {
  return new Promise((resolve, reject) => {
    const socket = dgram.createSocket('udp4');

    const timeout = setTimeout(() => {
      socket.close();
      reject(new Error(`DNS query timeout for ${domain}`));
    }, TIMEOUT_MS);

    const query = dnsPacket.encode({
      type: 'query',
      id: Math.floor(Math.random() * 65535),
      flags: dnsPacket.RECURSION_DESIRED,
      questions: [{ type: 'TXT', name: domain }],
    });

    socket.on('error', (err) => {
      clearTimeout(timeout);
      socket.close();
      reject(err);
    });

    socket.on('message', (msg) => {
      clearTimeout(timeout);
      socket.close();

      try {
        const response = dnsPacket.decode(msg);
        const txtRecords = response.answers
          .filter(a => a.type === 'TXT')
          .flatMap(a => a.data)
          .map(d => (typeof d === 'string' ? d : d.toString('utf8')))
          .join('');

        resolve(txtRecords);
      } catch (err) {
        reject(new Error(`Failed to parse DNS response: ${err.message}`));
      }
    });

    socket.send(query, DNS_PORT, resolverIp, (err) => {
      if (err) {
        clearTimeout(timeout);
        socket.close();
        reject(err);
      }
    });
  });
}

/**
 * Query TXT records from a DNS server with retries.
 * @param {string} domain - Domain to query
 * @param {string} resolverIp - DNS resolver IP (default: Quad9)
 * @returns {Promise<string>} - Combined TXT record content
 */
export async function queryTXT(domain, resolverIp = DEFAULT_RESOLVER) {
  let lastError = null;

  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    try {
      return await queryTXTOnce(domain, resolverIp);
    } catch (err) {
      lastError = err;
      if (attempt < MAX_RETRIES) {
        // Wait before retrying
        await sleep(RETRY_DELAY_MS);
      }
    }
  }

  throw lastError;
}

/**
 * Resolve A records for a domain.
 * @param {string} domain - Domain to query
 * @param {string} resolverIp - DNS resolver IP (default: Quad9)
 * @returns {Promise<string[]>} - Array of IP addresses
 */
export async function resolveA(domain, resolverIp = DEFAULT_RESOLVER) {
  return new Promise((resolve, reject) => {
    const socket = dgram.createSocket('udp4');

    const timeout = setTimeout(() => {
      socket.close();
      reject(new Error(`DNS query timeout for ${domain}`));
    }, TIMEOUT_MS);

    const query = dnsPacket.encode({
      type: 'query',
      id: Math.floor(Math.random() * 65535),
      flags: dnsPacket.RECURSION_DESIRED,
      questions: [{ type: 'A', name: domain }],
    });

    socket.on('error', (err) => {
      clearTimeout(timeout);
      socket.close();
      reject(err);
    });

    socket.on('message', (msg) => {
      clearTimeout(timeout);
      socket.close();

      try {
        const response = dnsPacket.decode(msg);
        const ips = response.answers
          .filter(a => a.type === 'A')
          .map(a => a.data);

        resolve(ips);
      } catch (err) {
        reject(new Error(`Failed to parse DNS response: ${err.message}`));
      }
    });

    socket.send(query, DNS_PORT, resolverIp, (err) => {
      if (err) {
        clearTimeout(timeout);
        socket.close();
        reject(err);
      }
    });
  });
}
