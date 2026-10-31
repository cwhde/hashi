import { spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { execFileSync } from 'node:child_process';

export const statePath = resolve(import.meta.dirname, '.state.json');

function freePort(): Promise<number> {
	return new Promise((ok, fail) => {
		const s = createServer();
		s.listen(0, '127.0.0.1', () => {
			const a = s.address();
			s.close(() => (a && typeof a === 'object' ? ok(a.port) : fail(new Error('no port'))));
		});
	});
}

export default async function globalSetup() {
	const bin = resolve(import.meta.dirname, '../../bin/hashi-e2e');
	const dir = mkdtempSync(join(tmpdir(), 'hashi-e2e-'));
	const keyFile = join(dir, 'master.key');
	writeFileSync(keyFile, execFileSync(bin, ['keygen']), { mode: 0o600 });
	const port = await freePort();
	const baseURL = `http://localhost:${port}`;
	const appsURL = `http://localhost:${await freePort()}`;
	const statusURL = `http://localhost:${await freePort()}`;
	const proc = spawn(
		bin,
		[
			'-data-dir',
			dir,
			'-admin-addr',
			`127.0.0.1:${port}`,
			'-public-apps-addr',
			appsURL.replace('http://localhost', '127.0.0.1'),
			'-public-status-addr',
			statusURL.replace('http://localhost', '127.0.0.1')
		],
		{
			env: {
				...process.env,
				HASHI_MASTER_KEY_FILE: keyFile,
				HASHI_DEV_WEBAUTHN_ORIGIN: baseURL,
				// The Hetzner adapter talks to the recorded fixtures (token "replay-token", zone example.com).
				HASHI_DEV_DNS_FIXTURES: resolve(import.meta.dirname, '../../testdata/contracts/hetzner')
			}
		}
	);
	let out = '';
	const creds = await new Promise<{ username: string; password: string }>((ok, fail) => {
		const timer = setTimeout(
			() => fail(new Error(`no bootstrap block within 15 s:\n${out}`)),
			15_000
		);
		proc.stdout.on('data', (b: Buffer) => {
			out += b.toString();
			const m = /username: (\S+)\n\s+password: (\S+)/.exec(out); // hashi:allow-secret-pattern (parses the log block)
			if (m) {
				clearTimeout(timer);
				ok({ username: m[1], password: m[2] });
			}
		});
		proc.on('exit', (code) => fail(new Error(`hashi exited with ${code}:\n${out}`)));
	});
	// Wait until the admin listener answers.
	for (let i = 0; i < 50; i++) {
		if ((await fetch(`${baseURL}/readyz`).catch(() => null))?.ok) break;
		await new Promise((r) => setTimeout(r, 100));
	}
	writeFileSync(
		statePath,
		JSON.stringify({ baseURL, appsURL, statusURL, bin, dir, keyFile, ...creds })
	);
	return async () => {
		proc.kill('SIGTERM');
	};
}
