import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { openSync, readFileSync, writeFileSync } from 'node:fs';
import { execFileSync, spawn } from 'node:child_process';
import { logPath, statePath } from './global-setup';

// S1 gate (11-delivery.md §3): passkey registration and login pass with a virtual authenticator, end to end through
// the real binary: bootstrap → base settings → passkey → recovery key → finish → passkey sign-in → account pages.
const state = JSON.parse(readFileSync(statePath, 'utf8')) as {
	baseURL: string;
	appsURL: string;
	statusURL: string;
	bin: string;
	dir: string;
	keyFile: string;
	username: string;
	password: string;
};

// CI keeps no traces: on a failure, print where the page was and what it showed.
test.afterEach(async ({ page }, info) => {
	if (info.status === info.expectedStatus) return;
	const text = await page
		.locator('body')
		.innerText()
		.catch(() => '');
	console.log(`failed at ${page.url()}\n${text.slice(0, 2000)}`);
});

// Stops the Hashi process and starts it again with the same flags and data (strict mode starts locked).
async function restartHashi() {
	const s = JSON.parse(readFileSync(statePath, 'utf8')) as {
		bin: string;
		args: string[];
		env: Record<string, string>;
		pid: number;
		baseURL: string;
	};
	process.kill(s.pid, 'SIGTERM');
	for (let i = 0; i < 100; i++) {
		try {
			process.kill(s.pid, 0);
		} catch {
			break; // exited
		}
		await new Promise((r) => setTimeout(r, 100));
	}
	const log = openSync(logPath, 'a');
	const proc = spawn(s.bin, s.args, {
		env: { ...process.env, ...s.env },
		stdio: ['ignore', log, log]
	});
	writeFileSync(
		statePath,
		JSON.stringify({ ...JSON.parse(readFileSync(statePath, 'utf8')), pid: proc.pid })
	);
	for (let i = 0; i < 100; i++) {
		if ((await fetch(`${s.baseURL}/readyz`).catch(() => null))?.ok) return;
		await new Promise((r) => setTimeout(r, 100));
	}
	throw new Error('Hashi did not come back after the restart');
}

async function virtualAuthenticator(page: Page, transport: 'internal' | 'usb' = 'internal') {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('WebAuthn.enable');
	const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport,
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true,
			automaticPresenceSimulation: true,
			hasPrf: true // strict vault mode unlocks with the PRF extension (02 §5.4)
		}
	});
	// Detaches this authenticator (and its passkeys) from the browser.
	return () => cdp.send('WebAuthn.removeVirtualAuthenticator', { authenticatorId });
}

test('setup with a passkey, then sign in with it', async ({ page, request }) => {
	// One long journey through every built page; the default 30 s per test is too short on the CI runners.
	test.setTimeout(420_000); // one long walk; it took 3.6 of 4 min on a quiet CI runner (2026-11-19)
	await virtualAuthenticator(page);
	await page.goto(state.baseURL);
	await expect(page).toHaveURL(/\/setup$/);

	// Step 1: bootstrap login.
	await page.getByLabel('Username').fill(state.username);
	await page.getByLabel('Password').fill(state.password);
	await page.getByRole('button', { name: 'Sign in' }).click();

	// Step 2: base settings.
	await expect(page.getByRole('heading', { name: 'Base settings' })).toBeVisible();
	// A root domain outside the fixture zones, so step 3 has to offer the zone choice (auto-pick: Go integration tests).
	await page.getByLabel('Root domain').fill('example.net');
	await page.getByLabel('Admin domain').fill('hashi.example.net');
	await page.getByRole('button', { name: 'Save and continue' }).click();

	// Step 3: DNS provider against the recorded Hetzner fixtures. A bad token stores nothing and says why.
	await expect(page.getByRole('heading', { name: 'Connect your DNS provider' })).toBeVisible();
	await page.getByLabel('API token').fill('not-a-token');
	await page.getByRole('button', { name: 'Check read-only' }).click();
	await expect(page.getByRole('alert')).toContainText('token');
	await page.getByLabel('API token').fill('replay-token');
	await page.getByRole('button', { name: 'Check read-only' }).click();
	// No zone contains the root domain: nothing is stored yet, the admin picks one of the token's zones.
	await expect(page.getByText('Choose the zone Hashi manages')).toBeVisible();
	await page.getByRole('radio', { name: 'example.com' }).check();
	await page.getByRole('button', { name: 'Use this zone' }).click();
	await expect(page.getByText('58 records', { exact: false }).first()).toBeVisible();
	await expect(page.getByRole('alert')).toHaveCount(0);
	// NS and SOA are leave-only; everything else starts as "leave".
	await expect(page.getByRole('radiogroup', { name: 'Decision for @ NS' })).toHaveCount(0);
	await expect(page.getByRole('radiogroup', { name: 'Decision for @ SOA' })).toHaveCount(0);
	const svc1 = page.getByRole('radiogroup', { name: 'Decision for svc1 A' });
	await expect(svc1.getByRole('radio', { name: 'Leave' })).toHaveAttribute('aria-checked', 'true');
	await svc1.getByRole('radio', { name: 'Import' }).click();
	await page
		.getByRole('radiogroup', { name: 'Decision for svc2 CNAME' })
		.getByRole('radio', { name: 'Remove' })
		.click();
	await expect(page.getByText('1 to import · 1 to remove')).toBeVisible();
	// Optional write test (invariant 4): the plan is shown first; only "Approve and run" writes.
	await page.getByRole('button', { name: 'Plan the write test' }).click();
	await expect(page.getByText('Test write access to example.com')).toBeVisible();
	await expect(page.getByText('· 2 changes', { exact: false })).toBeVisible();
	await page.getByRole('button', { name: 'Approve and run' }).click();
	await expect(page.getByText('Write access confirmed', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Save decisions and continue' }).click();

	// Certificates can only be proven for names inside the managed zone (03 §5.3), so the root domain moves into it.
	await expect(page.getByRole('heading', { name: 'Certificates' })).toBeVisible();
	await page.getByRole('button', { name: 'Base settings' }).click();
	await page.getByLabel('Root domain').fill('example.com');
	// A name a level deeper, so step 7 needs the extra wildcard *.hashi.example.com (03 §5.3).
	await page.getByLabel('Admin domain').fill('dev.hashi.example.com');
	await page.getByRole('button', { name: 'Save and continue' }).click();

	// Step 4: certificate authority (dev build: a local CA; the DNS-01 values go through the fake zone).
	await expect(page.getByRole('heading', { name: 'Certificates' })).toBeVisible();
	await page.getByLabel('Google Trust Services').check();
	await page.getByLabel('Contact email').fill('admin@example.com');
	await page.getByLabel('EAB key ID').fill('kid-e2e');
	await expect(page.getByRole('button', { name: 'Register account' })).toBeDisabled(); // HMAC missing
	await page.getByLabel('EAB HMAC key').fill('ZTJlLWhtYWMta2V5');
	await page.getByRole('button', { name: 'Register account' }).click();
	await expect(page.getByText('Account registered with Google Trust Services')).toBeVisible();
	await page.getByRole('button', { name: 'Issue the certificate now' }).click();
	await expect(page.getByText('Issued', { exact: true })).toBeVisible();
	await expect(page.getByText('*.example.com', { exact: false }).first()).toBeVisible();
	await page.getByRole('button', { name: 'Continue' }).click();

	// Step 5: Traefik host (dev build: an in-memory Debian 13 host). Key shown, compared, pinned; checks; takeover; install.
	await expect(page.getByRole('heading', { name: 'Connect a Traefik host' })).toBeVisible();
	await page.getByRole('radio', { name: "I'll do it myself" }).check();
	await expect(page.getByTestId('hashi-public-key')).toContainText('ssh-ed25519 ');
	await page.getByLabel('Host or IP').fill('192.0.2.10');
	await page.getByRole('button', { name: 'Read the host key' }).click();
	await expect(page.getByTestId('host-key')).toContainText('SHA256:');
	await expect(page.getByText('ssh_host_ed25519_key.pub')).toBeVisible(); // the file for the shown key type
	await page.getByRole('button', { name: "It doesn't match" }).click();
	await expect(page.getByRole('alert')).toContainText('Connection stopped');
	await page.getByRole('button', { name: 'Read the host key' }).click();
	await page.getByRole('button', { name: 'It matches, pin it' }).click();
	const checks = page.getByRole('list', { name: 'Checks' });
	await expect(checks).toContainText('Debian GNU/Linux 13');
	await expect(checks).toContainText('No Traefik found on the host');
	await expect(page.getByRole('button', { name: 'Continue' })).toBeDisabled();
	await page.getByLabel(/Hashi may run its own Traefik service/).check();
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await page.getByRole('button', { name: 'Plan the install' }).click();
	await expect(page.getByRole('heading', { name: 'Traefik on edge' })).toBeVisible();
	await expect(page.getByText('traefik binary')).toBeVisible();
	await page.getByRole('button', { name: 'Approve and run' }).click();
	await expect(page.getByText('Installed and verified')).toBeVisible();
	await expect(
		page.getByText(/certificate\(s\) served with the expected fingerprint/)
	).toBeVisible();
	await page.getByRole('button', { name: 'Continue' }).click();

	// Step 6: the firewall host (fake answering the captured probe): key compared and pinned, validation only reads, the
	// first apply is a high-risk plan under the rescue timer.
	await expect(page.getByRole('heading', { name: 'Connect a firewall host' })).toBeVisible();
	await page.getByLabel('Name').fill('edge-a');
	await page.getByLabel('SSH address').fill('198.18.3.1');
	await page.getByLabel(/^Password for/).fill('once');
	await page.getByRole('button', { name: 'Read the host key' }).click();
	await page.getByRole('button', { name: 'It matches, pin it' }).click();
	// Start over works before the host is confirmed; the second try keeps going.
	await page.getByRole('button', { name: /Start over/ }).click();
	await page.getByLabel('Name').fill('edge-a');
	await page.getByLabel('SSH address').fill('198.18.3.1');
	await page.getByRole('radio', { name: "I'll do it myself" }).check();
	await page.getByRole('button', { name: 'Read the host key' }).click();
	await page.getByRole('button', { name: 'It matches, pin it' }).click();
	const fwChecks = page.getByRole('list', { name: 'Checks' });
	await expect(fwChecks).toContainText('nftables 1.1.3');
	await expect(fwChecks).toContainText('NetBird 0.81.3');
	await page.getByLabel('NetBird', { exact: true }).click();
	await page
		.getByRole('option', { name: 'This host also routes NetBird traffic to its networks' })
		.click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	// Saved: nothing differs any more, so Save is off again (bug 104), and the choice stays.
	await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	await expect(page.getByLabel('NetBird', { exact: true })).toContainText(
		'This host also routes NetBird traffic to its networks'
	);
	await page.getByLabel(/Hashi may manage its own firewall table/).check();
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await page.getByRole('button', { name: 'Plan the first apply' }).click();
	await expect(page.getByRole('heading', { name: 'Firewall on edge-a' })).toBeVisible();
	await page.getByRole('button', { name: 'Approve and run' }).click();
	await expect(page.getByText(/rescue timer disarmed/)).toBeVisible();
	await page.getByRole('button', { name: 'Continue' }).click();

	// Step 7: the Hashi admin service; its deeper name re-issues the certificate first, then the plan is shown.
	await expect(page.getByRole('heading', { name: 'Publish Hashi' })).toBeVisible();
	await page.getByRole('button', { name: 'Prepare' }).click();
	await expect(page.getByText(/Certificate: Applied: issued example.com \(3 names/)).toBeVisible();
	await expect(page.getByText('10-hashi-http-services.yml')).toBeVisible();
	await page.getByRole('button', { name: 'Approve and run' }).click();
	// Opening ports 80/443 is the step's last part: it stays open until approved (step 8 needs them).
	await expect(page.getByRole('heading', { name: 'Firewall on edge-a' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Continue' })).toBeDisabled();
	await page.getByRole('button', { name: 'Approve and run' }).click();
	await expect(page.getByText('The Traefik host serves dev.hashi.example.com')).toBeVisible();
	await page.getByRole('button', { name: 'Continue' }).click();

	// Step 8: the button opens https://dev.hashi.example.com/setup#https=<nonce>. There is no Traefik in front of this
	// dev server, so the test catches the link and sends the proof the way Hashi's Traefik would (dev edge token).
	await expect(page.getByRole('heading', { name: 'Open Hashi over HTTPS' })).toBeVisible();
	await page.route('https://dev.hashi.example.com/**', (route) => route.fulfill({ status: 204 }));
	const started = page.waitForResponse((r) => r.url().endsWith('/api/setup/https/start'));
	await page.getByRole('button', { name: 'Open https://dev.hashi.example.com' }).click();
	const link = ((await (await started).json()) as { url: string }).url;
	expect(link).toMatch(/^https:\/\/dev\.hashi\.example\.com\/setup#https=/);
	const nonce = link.split('#https=')[1];
	const edge = {
		Host: 'dev.hashi.example.com',
		'X-Forwarded-Proto': 'https',
		'X-Forwarded-For': '127.0.0.1'
	};
	const proof = `${state.baseURL}/api/setup/https/proof`;
	expect((await request.post(proof, { headers: edge, data: { nonce } })).status()).toBe(403); // no edge token
	const ok = await request.post(proof, {
		headers: { ...edge, 'X-Hashi-Edge': 'dev-edge-token' },
		data: { nonce }
	});
	expect(ok.status()).toBe(200);
	// Before setup finishes, the setup session can already use the services pages (02 §3.3), and nothing else.
	await page.goto(`${state.baseURL}/services`);
	await expect(page.getByRole('link', { name: /Hashi\s*system/ })).toBeVisible();
	await expect(page.getByRole('link', { name: 'DNS' })).toHaveCount(0);
	await page.goto(`${state.baseURL}/setup`);

	// Step 9: passkey, then recovery key with its checksum group.
	await expect(page.getByRole('heading', { name: 'Passkey and recovery key' })).toBeVisible();
	await page.getByLabel('Name').fill('E2E authenticator');
	await page.getByRole('button', { name: 'Create passkey' }).click();
	await expect(page.getByText('1 passkey registered')).toBeVisible();
	await page.getByRole('button', { name: 'Show recovery key' }).click();
	const key = (await page.getByTestId('recovery-key').textContent())?.trim() ?? '';
	expect(key.split('-')).toHaveLength(14);
	await page.getByLabel('Last group').fill('ZZZZ');
	await page.getByRole('button', { name: 'Confirm and test' }).click();
	await expect(page.getByRole('alert')).toContainText('last group');
	await page.getByLabel('Last group').fill(key.split('-').at(-1) ?? '');
	await page.getByRole('button', { name: 'Confirm and test' }).click();

	// Step 10 can wait, then step 11 finishes.
	await expect(page.getByRole('heading', { name: 'Optional integrations' })).toBeVisible();
	await page.getByRole('button', { name: 'Continue' }).click();
	await expect(page.getByRole('heading', { name: 'Finish setup' })).toBeVisible();
	await page.getByRole('button', { name: 'Finish setup' }).click();
	await expect(page).toHaveURL(/\/login$/);

	// Passkey sign-in.
	await page.getByRole('button', { name: 'Sign in with a passkey' }).click();
	await expect(page).toHaveURL(/\/overview$/); // the start page (bug 31)
	await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
	await page.getByRole('link', { name: 'Activity', exact: true }).first().click();
	await expect(page.getByRole('cell', { name: 'Setup finished' })).toBeVisible();
	await expect(page.getByRole('cell', { name: 'Signed in with a passkey' })).toBeVisible();

	// DNS: the observed zone with the decisions from setup; a changed decision survives a reload.
	await page.getByRole('link', { name: 'DNS' }).first().click();
	await expect(page).toHaveURL(/\/dns$/);
	const records = page.getByRole('table', { name: 'DNS records' });
	await expect(records.getByRole('row', { name: /^svc1 A / })).toContainText('imported');
	await expect(records.getByRole('row', { name: /^@ SOA / })).toContainText('protected');
	// A record Hashi doesn't own has a way into the editor: it is marked for import on the Import tab (bug 116).
	const take = records.getByRole('button', { name: /^Import .+ to edit it$/ }).first();
	const [, takeName, takeType] = /^Import (.+) (\S+) to edit it$/.exec(
		(await take.getAttribute('aria-label'))!
	)!;
	await take.click();
	await expect(page).toHaveURL(/\?tab=import$/);
	const marked = page.getByRole('radiogroup', { name: `Decision for ${takeName} ${takeType}` });
	await expect(marked.getByRole('radio', { name: 'Import' })).toHaveAttribute(
		'aria-checked',
		'true'
	);
	await marked.getByRole('radio', { name: 'Leave' }).click();
	// "Import all shown" sets every importable row the filter shows; nothing is sent until Save decisions.
	const importFilter = page.getByRole('tabpanel').getByLabel('Filter records');
	await importFilter.fill('svc29');
	await page.getByRole('button', { name: 'Import all shown (1)' }).click();
	const svc29 = page.getByRole('radiogroup', { name: 'Decision for svc29 CNAME' });
	await expect(svc29.getByRole('radio', { name: 'Import' })).toHaveAttribute(
		'aria-checked',
		'true'
	);
	await expect(page.getByRole('button', { name: 'Import all shown (0)' })).toBeDisabled();
	await svc29.getByRole('radio', { name: 'Leave' }).click();
	await importFilter.fill('');
	await page.getByRole('tab', { name: 'Import' }).click();
	await expect(page).toHaveURL(/\?tab=import$/);
	const svc2 = page.getByRole('radiogroup', { name: 'Decision for svc2 CNAME' });
	await expect(svc2.getByRole('radio', { name: 'Remove' })).toHaveAttribute('aria-checked', 'true');
	await svc2.getByRole('radio', { name: 'Leave' }).click();
	await page.getByRole('button', { name: 'Save decisions' }).click();
	await expect(page.getByText('Decisions saved.')).toBeVisible();
	await page.reload();
	await expect(
		page
			.getByRole('radiogroup', { name: 'Decision for svc2 CNAME' })
			.getByRole('radio', { name: 'Leave' })
	).toHaveAttribute('aria-checked', 'true');

	// A manual entry: saving creates a plan that applies right away (low risk) and is verified.
	await page.getByRole('tab', { name: 'Records' }).click();
	await page.getByRole('button', { name: 'Add record' }).click();
	await page.getByLabel('Name', { exact: true }).fill('_hashi-test');
	await page.getByLabel('Values', { exact: true }).fill('192.0.2.10');
	// A TTL that is not whole seconds is refused in the form instead of becoming "zone default".
	await page.getByLabel('TTL (seconds)').fill('300s');
	await expect(page.getByRole('alert')).toContainText('Whole seconds');
	await expect(page.getByRole('button', { name: 'Save · creates a plan' })).toBeDisabled();
	await page.getByLabel('TTL (seconds)').fill('');
	await page.getByRole('button', { name: 'Save · creates a plan' }).click();
	await expect(page.getByRole('status')).toContainText('1 created, verified');
	const entry = records.getByRole('row', { name: /^_hashi-test A / });
	await expect(entry).toContainText('user');
	await expect(entry).toContainText('192.0.2.10');
	// NS and SOA cannot be edited at all.
	await expect(page.getByRole('button', { name: 'Edit @ NS' })).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Edit @ SOA' })).toHaveCount(0);

	// Deleting it is high risk: it waits on Pending changes until approved with the passkey.
	await page.getByRole('button', { name: 'Edit _hashi-test A' }).click();
	await page.getByRole('button', { name: 'Delete record' }).click();
	await expect(page.getByRole('status')).toContainText('needs your approval');
	await expect(page.getByRole('link', { name: /1 pending change/ })).toBeVisible();
	// The count is there on a fresh load too, without navigating anywhere first.
	await page.reload();
	await expect(page.getByRole('link', { name: /1 pending change/ })).toBeVisible();
	// Another save while that delete waits reports its own result, not the waiting delete.
	await page.getByRole('button', { name: 'Add record' }).click();
	await page.getByLabel('Name', { exact: true }).fill('b._hashi-test');
	await page.getByLabel('Values', { exact: true }).fill('192.0.2.11');
	await page.getByRole('button', { name: 'Save · creates a plan' }).click();
	await expect(page.getByRole('status')).toContainText(
		'b._hashi-test A saved. Applied: 1 created, verified'
	);
	await page.getByRole('link', { name: /1 pending change/ }).click();
	await expect(page).toHaveURL(/\/changes$/);
	await expect(page.getByText('Delete DNS records in example.com')).toBeVisible();
	await expect(page.getByText('192.0.2.10  TTL zone')).toBeVisible();
	await page.getByRole('button', { name: 'Approve and apply' }).click();
	await expect(page.getByRole('status')).toContainText('1 deleted, verified');
	await expect(page.getByText('Nothing waiting')).toBeVisible();
	await page.getByRole('link', { name: 'DNS' }).first().click();
	await expect(records.getByRole('row', { name: /^_hashi-test A / })).toHaveCount(0);

	// Traefik: the host from setup with the files Hashi applied; the certificate is served by it; "Renew now" plans.
	await page.getByRole('link', { name: 'Traefik' }).first().click();
	await expect(page).toHaveURL(/\/traefik$/);
	await expect(page.getByText('v3.7.16')).toBeVisible();
	const files = page.getByRole('list', { name: 'Files on edge' });
	await expect(files).toContainText('40-hashi-security.yml');
	await expect(files).toContainText('example.com.crt');
	await expect(files).not.toContainText('example.com.key'); // key hashes stay out of the database
	await page.getByRole('button', { name: 'Check and sync' }).click();
	await expect(page.getByRole('status')).toContainText('edge already matches');
	await page.getByRole('tab', { name: 'Certificates' }).click();
	await expect(page).toHaveURL(/\?tab=certificates$/);
	const certs = page.getByRole('list', { name: 'Certificates' });
	await expect(certs).toContainText('example.com, *.example.com');
	await expect(certs).toContainText('served by 1 of 1 host');
	await expect(certs).toContainText('Hashi dev');
	await page.getByRole('button', { name: 'Renew now' }).click();
	await expect(page.getByRole('status')).toContainText('example.com renewed');
	await expect(certs).toContainText('served by 0 of 1 host'); // the new one is not on the host yet
	await page.getByRole('tab', { name: 'Hosts' }).click();
	await page.getByRole('button', { name: 'Check and sync' }).click();
	await expect(page.getByRole('status')).toContainText(
		'certificate(s) served with the expected fingerprint'
	);
	await page.getByRole('tab', { name: 'Certificates' }).click();
	await expect(certs).toContainText('served by 1 of 1 host');

	// Services: the admin service from setup is read-only; a user service with a route and rewrite is published on the
	// edge, shows its rendered excerpt, and turning it off takes it off the host again.
	await page.getByRole('link', { name: 'Services' }).first().click();
	await expect(page).toHaveURL(/\/services$/);
	await expect(page.getByRole('link', { name: /Hashi\s*system/ })).toBeVisible();
	await page.getByRole('button', { name: 'Add service' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Search');
	await page.getByLabel('Public domain').fill('search');
	await page.getByLabel('Address').fill('192.0.2.20');
	await page.getByLabel('Port').fill('8080');
	await page.getByRole('button', { name: 'Save and publish' }).click();
	await expect(page.getByRole('status')).toContainText('applied on 1 Traefik host');
	await page.getByRole('link', { name: /^Search/ }).click();
	await expect(page).toHaveURL(/\/services\/[0-9A-Z]+$/);
	await page.getByLabel('Advanced routes').click();
	await page.getByRole('button', { name: 'Add route' }).click();
	await page.getByLabel('Path', { exact: true }).fill('/api');
	await page.getByLabel('Rewrite', { exact: true }).click();
	await page.getByRole('option', { name: 'strip prefix' }).click();
	await page.getByRole('button', { name: 'Save and publish' }).click();
	await expect(page.getByRole('status')).toContainText('applied on 1 Traefik host');
	await page.getByRole('tab', { name: 'Rendered config' }).click();
	await expect(page.getByText(/stripPrefix/)).toBeVisible();
	await page.getByRole('button', { name: 'Turn off' }).click();
	await expect(page.getByRole('status')).toContainText('applied on 1 Traefik host');
	await expect(page.getByText('Off', { exact: true })).toBeVisible();
	// Bug 78: the rendered config of an Off service says it isn't on any host.
	await expect(page.getByText('Off: not on any Traefik host')).toBeVisible();
	// Deleting asks for the passkey (reauth), then the service is gone.
	await page.getByRole('button', { name: 'Delete' }).click();
	await page.getByRole('button', { name: 'Delete service' }).click();
	await expect(page).toHaveURL(/\/services$/);
	await expect(page.getByRole('link', { name: /^Search/ })).toHaveCount(0);

	// IP sets (05a): system sets are read-only; a user set with a first item, a nested set, a refused loop, the
	// address page's path through nesting, and delete refused while another set still includes it.
	await page.getByRole('link', { name: 'IP sets' }).first().click();
	await expect(page).toHaveURL(/\/ip-sets$/);
	await expect(page.getByRole('link', { name: /^private-networks/ })).toBeVisible();
	await page.getByRole('button', { name: 'Add set' }).click();
	await page.getByLabel('Name', { exact: true }).fill('home');
	await page.getByLabel('Address (IPv4 or IPv6)').fill('192.0.2.44');
	await page.getByRole('button', { name: 'Create set' }).click();
	await expect(page).toHaveURL(/\/ip-sets\/[0-9A-Z]+$/);
	await expect(page.getByRole('heading', { name: 'home' })).toBeVisible();
	await page.getByRole('button', { name: 'Add item' }).first().click();
	await page.getByRole('radio', { name: 'CIDR' }).click();
	await page.getByLabel('Prefix').fill('198.51.100.9/24');
	await page.getByRole('button', { name: 'Add item' }).last().click();
	await expect(page.getByRole('status')).toContainText('Item added.');
	await expect(
		page.getByRole('tabpanel', { name: 'Items' }).getByText('198.51.100.0/24')
	).toBeVisible(); // host bits cleared
	const homeURL = page.url();
	await page.getByRole('link', { name: 'IP sets' }).first().click();
	await page.getByRole('button', { name: 'Add set' }).click();
	await page.getByLabel('Name', { exact: true }).fill('office');
	await page.getByRole('radio', { name: 'Other set' }).click();
	await page.getByLabel('Set to include').click();
	await page.getByRole('option', { name: /^home/ }).click();
	await page.getByRole('button', { name: 'Create set' }).click();
	await expect(page.getByRole('heading', { name: 'office' })).toBeVisible();
	await page.goto(homeURL);
	await page.getByRole('button', { name: 'Add item' }).first().click();
	await page.getByRole('radio', { name: 'Other set' }).click();
	await page.getByLabel('Set to include').click();
	await page.getByRole('option', { name: /^office/ }).click();
	await page.getByRole('button', { name: 'Add item' }).last().click();
	await expect(page.getByRole('alert')).toContainText('home → office → home');
	await page.getByRole('button', { name: 'Cancel' }).click();
	await page.goto(`${state.baseURL}/ip-sets/address?ip=192.0.2.44`);
	await expect(page.getByRole('heading', { name: '192.0.2.44' })).toBeVisible();
	await expect(page.getByText('office › home › 192.0.2.44 (ip)')).toBeVisible();
	await page.goto(homeURL);
	await page.getByRole('tab', { name: 'Used by' }).click();
	await expect(page.getByRole('link', { name: 'office' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Delete set' })).toBeDisabled();

	// Account: the passkey and this session; the last passkey cannot be deleted.
	await page.goto(`${state.baseURL}/settings?category=security`);
	await expect(page.getByText('E2E authenticator')).toBeVisible();
	await expect(page.getByText('this session')).toBeVisible();
	await expect(
		page.getByRole('button', { name: 'Delete passkey E2E authenticator' })
	).toBeDisabled();

	// Adding a passkey is a sequence of steps, each naming the device it wants (bug 37): confirm with the existing
	// passkey, create on the new device, then link it to the vault. The same authenticator refuses a second
	// credential (Hashi sends existing ones as exclusions), so a second, USB one is attached.
	await page.getByRole('button', { name: 'Add passkey' }).click();
	const add = page.getByRole('dialog', { name: 'Add a passkey' });
	await expect(add.getByText('Confirm it’s you')).toBeVisible();
	await add.getByLabel('Name').fill('Second key');
	const confirmStep = add.getByRole('button', { name: 'Confirm with a known passkey' });
	if (await confirmStep.isVisible()) await confirmStep.click();
	await add.getByRole('button', { name: 'Create passkey' }).click();
	await expect(add.getByRole('alert')).toContainText('already holds a passkey');
	const removeUsb = await virtualAuthenticator(page, 'usb');
	await add.getByRole('button', { name: 'Create passkey' }).click();
	await expect(add.getByText('“Second key” created')).toBeVisible();
	await page.screenshot({ path: 'test-results/visual/passkey-add-steps.png' });
	await page.setViewportSize({ width: 390, height: 844 });
	await page.screenshot({ path: 'test-results/visual/passkey-add-steps-phone.png' });
	await page.setViewportSize({ width: 1280, height: 800 });
	await add.getByRole('button', { name: 'Link Second key' }).click();
	await expect(add.getByRole('status')).toHaveText('Second key can unlock the vault now.');
	await add.getByRole('button', { name: 'Done' }).click();
	await expect(page.getByText('Second key').first()).toBeVisible();

	// Firewall pages (05): the host from step 6 with its rendered table; no old script, no mention;
	// a UDP service opens its port, which waits for approval.
	await page.getByRole('link', { name: 'Firewall hosts' }).first().click();
	await page.getByRole('link', { name: 'Open edge-a' }).click();
	await expect(page.getByRole('heading', { name: 'edge-a' })).toBeVisible();
	await page.getByRole('tab', { name: 'Generated' }).click();
	await expect(page.getByText('"name": "hashi"').first()).toBeVisible();
	await expect(page.getByRole('tab', { name: 'Old script' })).toHaveCount(0); // no script: no mention
	await page.getByRole('link', { name: 'Services' }).first().click();
	await page
		.getByRole('button', { name: /Add service/ })
		.first()
		.click();
	await page.getByLabel('Name', { exact: true }).fill('Game');
	await page.getByRole('combobox', { name: 'Protocol' }).click();
	await page.getByRole('option', { name: 'UDP on its own public port' }).click();
	await page.getByLabel('Address').fill('10.32.0.40');
	await page.getByLabel('Port', { exact: true }).fill('19132');
	await page.getByLabel('Public port').fill('19132');
	await page.getByRole('button', { name: 'Save and publish' }).click();
	await expect(page.getByRole('status')).toContainText('need your approval');
	// A home server behind a router: no SSH, public traffic enters through edge-a, its own network.
	// Every outside system is added on Connections (bug 24).
	await page.goto(`${state.baseURL}/connections`);
	await page.getByRole('button', { name: 'Add connection' }).click();
	await page.getByRole('menuitem', { name: 'Firewall host' }).click();
	await page.getByLabel('Name').fill('home-a');
	await page.getByLabel('How it is reached').click();
	await page.getByRole('option', { name: 'Behind a router I configure myself' }).click();
	await page.getByLabel('Public address').fill('198.51.100.40');
	await page.getByLabel('Public traffic enters through').click();
	await page.getByRole('option', { name: 'edge-a' }).click();
	await page.getByLabel("Traefik's address on this network").fill('10.33.0.10');
	await page.getByLabel('Networks behind this host').fill('10.33.0.0/24');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.locator('#entry')).toContainText('edge-a');
	await expect(page.getByText('via.edge-a')).toBeVisible();
	await page.goto(`${state.baseURL}/firewall`);
	await page.screenshot({ path: 'test-results/firewall-hosts.png', fullPage: true });

	// Bootstrap login is gone after setup.
	const res = await page.request.post(`${state.baseURL}/api/auth/bootstrap/login`, {
		data: { username: state.username, password: state.password } // hashi:allow-secret-pattern (runtime value)
	});
	expect(res.status()).toBe(403);

	// Sign out ends the session server-side; signing in again works.
	await page.getByRole('button', { name: 'Account', exact: true }).first().click();
	await page.getByRole('menuitem', { name: 'Sign out' }).click();
	await expect(page).toHaveURL(/\/login$/);
	const s = await (await page.request.get(`${state.baseURL}/api/auth/session`)).json();
	expect(s.authenticated).toBe(false);
	await removeUsb(); // sign in with the first passkey for sure
	await page.getByRole('button', { name: 'Sign in with a passkey' }).click();
	await expect(page).toHaveURL(/\/overview$/);

	// Status: Hashi's own endpoint is checked without a browser; a user check, its page and settings; the public view.
	await page.getByRole('link', { name: 'Status', exact: true }).first().click();
	await expect(page).toHaveURL(/\/status$/);
	await expect(page.getByRole('link', { name: /Hashi/ }).first()).toBeVisible({ timeout: 30_000 });
	await page.getByRole('button', { name: 'Add check' }).click();
	await page.getByLabel('Name', { exact: true }).fill('E2E port');
	await page.getByLabel('Target').fill('127.0.0.1:1');
	await expect(page.getByRole('button', { name: 'Add check' }).last()).toBeEnabled();
	await page.getByRole('combobox', { name: 'Type' }).click();
	await page.getByRole('option', { name: 'TCP connect' }).click();
	await page.getByRole('button', { name: 'Add check' }).last().click();
	await expect(page).toHaveURL(/\/status\/[0-9A-Z]+$/);
	await expect(page.getByRole('heading', { name: 'E2E port' })).toBeVisible();
	await page.getByRole('switch', { name: 'Show on the public status page' }).click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Saved.');
	// The public status page on its own port shows the endpoint, no target, and offers nothing of the admin app.
	await page.waitForTimeout(16_000); // past the 15 s public cache
	const pub = await (await request.get(`${state.statusURL}/api/public/status`)).json();
	expect(JSON.stringify(pub)).toContain('E2E port');
	expect(JSON.stringify(pub)).not.toContain('127.0.0.1');
	expect((await request.get(`${state.statusURL}/api/auth/session`)).status()).toBe(404);
	expect((await request.get(`${state.statusURL}/settings`)).status()).toBe(404);
	const visitor = await page.context().newPage();
	await visitor.goto(state.statusURL);
	await expect(visitor.getByRole('heading', { name: /status/ })).toBeVisible();
	await expect(visitor.getByText('E2E port')).toBeVisible();
	await visitor.goto(state.appsURL);
	await expect(visitor.getByText(/hosts online/)).toBeVisible();
	await visitor.close();
	// Notifications: no provider yet; the page explains it. Sent alerts are history, so they live under Activity (bug 39).
	await page.goto(`${state.baseURL}/activity`);
	await page.getByRole('tab', { name: 'Alerts sent' }).click();
	await expect(page.getByText('No alerts are sent until you choose a provider.')).toBeVisible();
	await page.getByRole('link', { name: 'Notification settings' }).click();
	await expect(page.getByText('Add Telegram or email to receive alerts.')).toBeVisible();

	// A provider can be renamed without entering its secret again.
	await page.getByRole('button', { name: 'Add provider' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Bot');
	await page.getByLabel('Bot token').fill('123:e2e-token');
	await page.getByLabel('Chat').fill('123');
	await page.getByRole('button', { name: 'Add provider' }).last().click();
	await expect(page.getByText('Telegram · chat 123')).toBeVisible();
	await page.getByRole('button', { name: 'Edit Bot' }).click();
	await page.getByLabel('Name', { exact: true }).fill('My Telegram');
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect(page.getByRole('button', { name: 'Edit My Telegram' })).toBeVisible();
	await expect(page.getByText('Telegram · chat 123')).toBeVisible();
	await page.getByRole('button', { name: 'Remove My Telegram' }).click();
	await expect(page.getByText('Add Telegram or email to receive alerts.')).toBeVisible();

	// AdGuard: adding one is safe, and the page says so.
	await page.goto(`${state.baseURL}/connections`);
	await expect(page.getByText('A new one starts read-only')).toBeVisible();
	await page.getByRole('button', { name: 'Add connection' }).click();
	await page.getByRole('menuitem', { name: 'AdGuard Home' }).click();
	await expect(page.getByLabel('Name on its certificate')).toBeVisible();
	// Bug 90: over HTTP there is no certificate to check (a Pulse agent target only allows HTTPS).
	await page.getByLabel('Reached through').click();
	await page.getByRole('option', { name: 'Address set' }).click();
	await page.locator('#ag-scheme').click();
	await page.getByRole('option', { name: 'HTTP', exact: true }).click();
	await expect(page.getByLabel('Name on its certificate')).toHaveCount(0);
	await expect(page.locator('#ag-port')).toHaveValue('80');
	await page.getByRole('button', { name: 'Cancel' }).click();

	// Pulse: an agent's token is shown once, and an agent can go into an IP set. Agents are added on Connections.
	await page.goto(`${state.baseURL}/pulse`);
	await page.getByRole('link', { name: 'Add agent' }).click();
	await expect(page).toHaveURL(/\/connections$/);
	await page.getByLabel('Name').fill('e2e-box');
	await page.getByRole('button', { name: 'Create agent' }).click();
	const command = page.getByText(/--token hp_/);
	await expect(command).toBeVisible();
	const line = (await command.textContent()) ?? '';
	const agentId = /--id (\S+)/.exec(line)?.[1] ?? '';
	const agentToken = /--token (\S+)/.exec(line)?.[1] ?? '';
	await page.getByRole('button', { name: 'Done' }).click();
	// A real heartbeat with that token shows up on the page.
	const beat = await page.request.post(`${state.baseURL}/api/pulse/${agentId}/heartbeat`, {
		headers: { Authorization: `Bearer ${agentToken}` },
		data: {
			version: 'e2e',
			hostname: 'e2e-host',
			candidates: [
				{ interface: 'eth0', address: '10.9.8.7' },
				{ interface: 'wg0', address: '10.9.8.8' }
			]
		}
	});
	expect(beat.status()).toBe(200);
	await page.goto(`${state.baseURL}/pulse`);
	await expect(page.getByRole('cell', { name: '10.9.8.7' })).toBeVisible();
	// Bug 22: the agent's page lists its addresses per interface; choosing one makes it the main address.
	await page.getByRole('link', { name: 'e2e-box' }).click();
	await expect(page.getByText('wg0')).toBeVisible();
	await page.getByRole('radio', { name: '10.9.8.8' }).click();
	await expect(page.getByRole('status')).toContainText('10.9.8.8 is the main address now.');
	await page.goto(`${state.baseURL}/pulse`);
	await expect(page.getByRole('cell', { name: /10\.9\.8\.8/ })).toBeVisible();
	// The DNS page lists its name and AdGuard state.
	await page.goto(`${state.baseURL}/dns?tab=agents`);
	await expect(page.getByText('e2e-box.hashi.home.arpa')).toBeVisible();
	await page.goto(`${state.baseURL}/dns?tab=adguard`);
	await expect(page.getByText('No AdGuard instance')).toBeVisible();
	await page.goto(`${state.baseURL}/connections`);
	await page.getByRole('button', { name: 'Actions for e2e-box' }).click();
	await page.getByRole('menuitem', { name: 'Remove' }).click();
	// A removal asks first and names what goes (10c §5).
	await expect(page.getByRole('alertdialog')).toContainText('Remove e2e-box?');
	await page.getByRole('button', { name: 'Remove agent' }).click();
	await expect(page.getByRole('button', { name: 'Actions for e2e-box' })).toHaveCount(0);

	// Rule sets (06 §13): a shared set with one rule, attached to a service; the trace shows who decides; a closed fallback
	// says what happens while Hashi is away. The admin service stays read-only.
	await page.goto(`${state.baseURL}/rule-sets`);
	await page.getByRole('button', { name: 'Add rule set' }).click();
	await page.getByLabel('Slug').fill('e2e-shared');
	await page.getByRole('button', { name: 'Add rule set' }).last().click();
	await expect(page).toHaveURL(/\/rule-sets\/[0-9A-Z]+$/);
	await page.getByRole('button', { name: 'Add rule', exact: true }).click();
	await page.getByRole('combobox', { name: 'Path match' }).click();
	await page.getByRole('option', { name: 'prefix' }).click();
	await page.locator('#pp').fill('/admin');
	// Bug 61: the editor offers every action the edge engine runs, Pass CAPTCHA included.
	await expect(page.getByRole('radio', { name: /Pass CAPTCHA/ })).toBeVisible();
	await page.getByRole('radio', { name: /Deny/ }).click();
	await page.getByRole('button', { name: 'Save rule' }).click();
	await expect(page.getByText('/admin')).toBeVisible();
	await page.goto(`${state.baseURL}/services`);
	await page.getByRole('button', { name: 'Add service' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Guarded');
	await page.getByLabel('Public domain').fill('guarded');
	await page.getByLabel('Address').fill('192.0.2.21');
	await page.getByLabel('Port').fill('8080');
	await page.getByRole('button', { name: 'Save and publish' }).click();
	await expect(page.getByRole('status')).toContainText('Saved');
	await page.getByRole('link', { name: /^Guarded/ }).click();
	await page.getByRole('tab', { name: 'Access' }).click();
	await page.getByRole('button', { name: 'Attach rule set' }).click();
	await page.getByRole('combobox').last().click();
	await page.getByRole('option', { name: 'e2e-shared' }).click();
	await page.getByRole('button', { name: 'Attach', exact: true }).click();
	await expect(page.getByRole('link', { name: 'e2e-shared' })).toBeVisible();
	await expect(page.getByText('this service refuses requests')).toBeVisible();
	await page.getByLabel('Address to try').fill('203.0.113.7');
	await page.getByLabel('Path to try').fill('/admin/x');
	await page.getByRole('button', { name: 'Try a request' }).click();
	await expect(page.getByLabel('Decides')).toBeVisible();
	// S10: the WAF tab (headers only by default, a path with uploads). Bug 65: it saves with the header's one Save.
	await page.getByRole('tab', { name: 'WAF' }).click();
	await expect(page.getByText('Mode per path')).toBeVisible();
	await expect(page.getByLabel('WAF mode for the whole service')).toContainText(
		'default (headers only)'
	);
	await page.getByRole('button', { name: 'Add path' }).click();
	await page.getByLabel('Path', { exact: true }).fill('/upload');
	await expect(page.getByRole('button', { name: 'Save WAF' })).toHaveCount(0);
	await page.getByRole('button', { name: 'Save and publish' }).click();
	await expect(page.getByRole('status')).toContainText('Saved');
	await expect(page.getByRole('button', { name: 'Save and publish' })).toBeDisabled();
	await page.reload();
	await page.getByRole('tab', { name: 'WAF' }).click();
	await expect(page.getByLabel('Path', { exact: true })).toHaveValue('/upload');
	await page.setViewportSize({ width: 390, height: 844 });
	await page.screenshot({ path: 'test-results/s10-waf-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });

	// Bug 25: the user middleware file (04 §4). A file that fails the strict check shows its line and cannot be saved;
	// a valid one lists what it defines, one goes on every service, and saving applies it on the edge.
	const guardedURL = page.url();
	await page.goto(`${state.baseURL}/traefik?tab=middlewares`);
	const editor = page.getByRole('textbox', { name: 'User middleware file' });
	await editor.click();
	await page.keyboard.insertText('http:\n  routers: {}\n');
	await expect(page.getByText(/Line 2: under http: only middlewares/)).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save and plan' })).toBeDisabled();
	await page.keyboard.press('ControlOrMeta+a');
	await page.keyboard.insertText(
		'# office and frames\nhttp:\n  middlewares:\n    office-only:\n      ipAllowList:\n        sourceRange: [192.0.2.0/24]\n' +
			'    no-frames:\n      headers:\n        customResponseHeaders:\n          X-Frame-Options: DENY\n'
	);
	await expect(page.getByText('Found 2 middlewares')).toBeVisible();
	await expect(page.getByText('Valid. Not saved yet.')).toBeVisible();
	await page.getByRole('button', { name: 'Save and plan' }).click();
	await expect(page.getByRole('status').first()).toContainText('Saved');
	await expect(page.getByText('Valid and saved.')).toBeVisible();
	// Which middlewares run on every service is a setting (10c §1.2); the Traefik page keeps the file.
	await page.goto(`${state.baseURL}/settings?category=traefik`);
	await page.getByRole('switch', { name: 'Use no-frames on every service' }).click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Saved');
	await page.goto(`${state.baseURL}/traefik?tab=middlewares`);
	await expect(page.getByText('Valid and saved.')).toBeVisible();
	// Break the file, then take the break back: the old problem goes away and the switch stays on.
	await editor.click();
	await page.keyboard.press('ControlOrMeta+End');
	await page.keyboard.insertText('bad');
	await expect(page.getByText(/^Line \d+:/)).toBeVisible();
	for (let i = 0; i < 3; i++) await page.keyboard.press('Backspace');
	await expect(page.getByText('Valid and saved.')).toBeVisible();
	await expect(page.getByRole('list', { name: 'Middlewares in the file' })).toContainText(
		'on every service'
	);
	await page.setViewportSize({ width: 390, height: 844 });
	await page.screenshot({ path: 'test-results/bug25-middlewares-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });

	// Traffic options (04 §1.1a) on the service: a Host name of its own, no X-Real-IP, an extra header, a timeout and
	// one middleware; the rendered config shows them.
	await page.goto(guardedURL);
	// Bug 61: the summary lists the checks Status runs for the service instead of a Monitoring tab.
	await expect(page.getByRole('tab', { name: 'Monitoring' })).toHaveCount(0);
	await expect(page.getByText('Checks', { exact: true })).toBeVisible();
	await page.getByRole('tab', { name: 'Traffic' }).click();
	await page.getByRole('radio', { name: /A name you choose/ }).click();
	await page.getByLabel('Name sent to the app').fill('guarded.internal.example');
	await page.getByRole('switch', { name: /X-Real-IP/ }).click();
	await page.getByRole('button', { name: 'Add header' }).first().click();
	await page.getByLabel('Header name').first().fill('X-Extra');
	await page.getByLabel('Header value').first().fill('one');
	await page.getByLabel('Wait for an answer').fill('120');
	await page
		.getByRole('list', { name: 'Extra middlewares' })
		.getByRole('button', { name: 'office-only' })
		.click();
	await page.getByRole('button', { name: 'Save and publish' }).click();
	await expect(page.getByRole('status').first()).toContainText('Saved');
	await page.setViewportSize({ width: 390, height: 844 });
	await page.screenshot({ path: 'test-results/bug25-traffic-phone.png', fullPage: true });
	// The tabs scroll in their own row; the page itself never scrolls sideways.
	expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.getByRole('tab', { name: 'Rendered config' }).click();
	for (const want of [
		'Host: guarded.internal.example',
		'X-Real-Ip: ""',
		'X-Extra: one',
		'responseHeaderTimeout: 120s',
		'- office-only',
		'- no-frames'
	]) {
		await expect(page.locator('pre')).toContainText(want);
	}
	await page.goto(`${state.baseURL}/traefik?tab=middlewares`);
	await expect(page.getByRole('list', { name: 'Middlewares in the file' })).toContainText(
		'used by Guarded'
	);

	await page.goto(`${state.baseURL}/security`);
	// Dashboard first: live tiles and the period switch.
	await expect(page.getByText('Most refused addresses')).toBeVisible();
	await expect(page.getByText('WAF matches', { exact: true }).first()).toBeVisible();
	await page.getByRole('radio', { name: '7d' }).click();
	await expect(page.getByText('Kernel meters')).toBeVisible();
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
		'the dashboard is wider than a phone'
	).toBe(true);
	await page.screenshot({ path: 'test-results/s10-dashboard-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });
	// A ban by hand shows in Counters with its state; ending it from the row menu leaves the counter.
	// Banning by hand needs a fresh passkey (06 §5.1); the screen asks, the virtual authenticator answers.
	await page.goto(`${state.baseURL}/ip-sets/sys-dynamic-blocked`);
	await page.getByLabel('Address', { exact: true }).fill('198.51.100.77');
	await page.getByLabel('Minutes').fill('10');
	await page.getByRole('button', { name: 'Ban now' }).click();
	await expect(page.getByText('1 banned address')).toBeVisible();
	await page.goto(`${state.baseURL}/security`);
	await page.getByRole('tab', { name: 'Counters' }).click();
	await expect(page.getByRole('row', { name: /198\.51\.100\.77/ })).toContainText('banned');
	// Bug 93: a concept is explained behind its help icon, not in a paragraph on the page (10c §4.7).
	await expect(page.getByText('Solving a CAPTCHA resets neither.')).toHaveCount(0);
	await page.getByRole('button', { name: 'About counters' }).click();
	await expect(page.getByText('Solving a CAPTCHA resets neither.')).toBeVisible();
	await page.keyboard.press('Escape');
	await page.getByRole('button', { name: 'Actions for 198.51.100.77' }).click();
	await page.getByRole('menuitem', { name: 'Unban' }).click();
	await expect(page.getByRole('row', { name: /198\.51\.100\.77/ })).not.toContainText('banned');
	await page.setViewportSize({ width: 390, height: 844 });
	// The row menu must be reachable without scrolling sideways.
	await expect(page.getByRole('button', { name: 'Actions for 198.51.100.77' })).toBeInViewport();
	await page.screenshot({ path: 'test-results/s10-counters-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });
	// Editing a counter keeps the old value in the audit log, shown on Activity (06 §16).
	await page.getByRole('button', { name: 'Actions for 198.51.100.77' }).click();
	await page.getByRole('menuitem', { name: 'Edit counters' }).click();
	await page.getByLabel('Ban counter').fill('4');
	await page.getByRole('button', { name: 'Confirm with passkey and save' }).click();
	await expect(page.getByRole('row', { name: /198\.51\.100\.77/ })).toContainText('4');
	await page.goto(`${state.baseURL}/activity`);
	await expect(page.getByRole('row', { name: /Security counters edited/ }).first()).toContainText(
		'ban 1 → 4'
	);
	// The address page shows what the engine knows (bug 135): counters, its events, and challenge and lift from there.
	await page.goto(`${state.baseURL}/ip-sets/address?ip=198.51.100.77`);
	const engine = page.locator('section', {
		has: page.getByRole('heading', { name: 'Abuse engine' })
	});
	await expect(engine).toContainText('Not banned or challenged now.');
	await expect(
		engine.getByText('Ban counter').locator('xpath=following-sibling::dd[1]')
	).toContainText('4');
	await expect(page.getByRole('list', { name: 'Recent events' })).toContainText('Banned');
	await page.getByRole('button', { name: 'Challenge now' }).click();
	await expect(page.getByRole('status')).toContainText('gets the challenge page now');
	await expect(engine).toContainText('challenged');
	await page.getByRole('button', { name: 'End the challenge' }).click();
	await expect(page.getByRole('status')).toContainText(
		'The challenge for 198.51.100.77 has ended.'
	);
	await expect(engine).toContainText('Not banned or challenged now.');
	// The dynamic sets on the IP sets page: challenge by hand with a duration, then end it.
	await page.goto(`${state.baseURL}/ip-sets/sys-dynamic-challenge`);
	await expect(page.getByText('Nobody is challenged right now.')).toBeVisible();
	await page.getByLabel('Address', { exact: true }).fill('198.51.100.78');
	await page.getByLabel('Minutes').fill('10');
	await page.getByRole('button', { name: 'Challenge now' }).click();
	await expect(page.getByText('1 challenged address')).toBeVisible();
	await page.setViewportSize({ width: 390, height: 844 });
	await expect(page.getByRole('button', { name: 'Actions for 198.51.100.78' })).toBeInViewport();
	await page.screenshot({ path: 'test-results/s10-dynamic-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.getByRole('button', { name: 'Actions for 198.51.100.78' }).click();
	await page.getByRole('menuitem', { name: 'End challenge' }).click();
	await expect(page.getByText('Nobody is challenged right now.')).toBeVisible();
	// S11 scripts (09 §1): save asks for the passkey, the change waits with its diff, apply puts it on the host, a run
	// by hand shows its output in the history.
	await page.goto(`${state.baseURL}/scripts`);
	await expect(page.getByText('No scripts yet.')).toBeVisible();
	await page.getByRole('button', { name: 'New script' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Disk report');
	await page.getByLabel('Schedule (cron, host time)').fill('30 3 * * 0');
	await expect(page.getByText(/^Next: Sun/)).toBeVisible();
	await page.getByRole('button', { name: 'Add variable' }).click();
	await page.getByLabel('Variable name').fill('KEEP');
	await page.getByLabel('Value of KEEP').fill('14');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('status')).toContainText('waits for you below');
	await expect(page.getByLabel('Changes to the file').last()).toContainText('set -eu');
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
		'the scripts page is wider than a phone'
	).toBe(true);
	await page.screenshot({ path: 'test-results/s11-scripts-plan-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.getByRole('button', { name: 'Approve and apply' }).first().click();
	await expect(page.getByRole('status')).toContainText('Applied');
	await page.getByRole('button', { name: 'Run now' }).click();
	await expect(page.getByLabel('Output')).toContainText('fake run of disk-report');
	await expect(page.getByRole('button', { name: /succeeded/ })).toBeVisible();
	await expect(page.getByText(/^Next: Sun/)).toBeVisible();
	await page.screenshot({ path: 'test-results/s11-scripts.png', fullPage: true });
	// Edge protection settings live in Settings; the Security page only shows what happened (10c §1, bug 36).
	await page.goto(`${state.baseURL}/security`);
	await page.getByRole('link', { name: 'Edge protection settings' }).click();
	await expect(page).toHaveURL(/category=edge/);
	await expect(page.getByText('Cap challenge')).toBeVisible();
	await expect(page.getByLabel('Challenge page')).toHaveValue(/^captcha\.hashi\./);
	await page.setViewportSize({ width: 390, height: 844 });
	const fits = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
	expect(fits, 'the page is wider than a phone').toBe(true);
	await page.screenshot({ path: 'test-results/s10-challenge-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });
	await expect(page.getByText('300 requests in 10 s')).toBeVisible();
	await expect(page.getByText('600 new per min, 1000 open per address')).toBeVisible();
	await page.getByRole('button', { name: 'Edit' }).click();
	await page.getByLabel('New connections (per min)').fill('700');
	await page.getByRole('dialog').getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByText('700 new per min, 1000 open per address')).toBeVisible();
	// Sign-in providers are connections (10c §1.2).
	await page.goto(`${state.baseURL}/connections`);
	await expect(page.getByText(/^No provider yet/)).toBeVisible();

	await page.goto(`${state.baseURL}/ip-sets`);
	await page.getByRole('button', { name: 'Add blocklist' }).click();
	await expect(page.getByText('Spamhaus DROP', { exact: true })).toBeVisible();
	await page.setViewportSize({ width: 390, height: 844 });
	await page.screenshot({ path: 'test-results/s10-blocklists-phone.png' });
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.keyboard.press('Escape');

	// The theme is saved, not only remembered by this browser.
	await page.getByRole('button', { name: 'Theme', exact: true }).click();
	await page.getByRole('menuitemradio', { name: 'Violet Night' }).click();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'violet');
	await expect
		.poll(async () => (await page.request.get(`${state.baseURL}/api/settings/appearance`)).json())
		.toMatchObject({ theme: 'violet' });
	await page.evaluate(() => localStorage.removeItem('hashi.theme'));
	await page.reload();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'violet');
	await page.goto(`${state.baseURL}/app-display`);
	await expect(page.getByRole('switch', { name: 'Show E2E port publicly' })).toBeChecked();

	// S11 settings (09 §2, 10-ui §2). A custom theme: unreadable text cannot be saved; a readable one applies here,
	// survives a reload before first paint and reaches the public pages.
	await page.goto(`${state.baseURL}/settings?category=appearance`);
	await page.getByLabel('Muted text colour', { exact: true }).fill('#2a2f36');
	await expect(page.getByRole('button', { name: 'Text too faint to save' })).toBeDisabled();
	await expect(page.getByText(/Muted text on the surface is too faint/)).toBeVisible();
	await page.getByLabel('Muted text colour', { exact: true }).fill('#9da6b2');
	await page.getByLabel('Accent colour', { exact: true }).fill('#14b8a6');
	await page.getByRole('radio', { name: 'Round' }).click();
	await page.getByRole('button', { name: 'Save and use theme' }).click();
	await expect(page.getByText('Your theme is in use here')).toBeVisible();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'custom');
	await page.reload();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'custom');
	expect(
		await page.evaluate(() =>
			getComputedStyle(document.documentElement).getPropertyValue('--h-accent')
		)
	).toBe('#14b8a6');
	const pubLook = await (await page.request.get(`${state.appsURL}/api/public/apps`)).json();
	expect(pubLook).toMatchObject({ theme: 'custom', custom_theme: { accent: '#14b8a6' } });
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
		'the appearance settings are wider than a phone'
	).toBe(true);
	await page.screenshot({ path: 'test-results/s11-custom-theme-phone.png', fullPage: true });
	await page.setViewportSize({ width: 1280, height: 800 });
	// Reduce transparency is applied and saved at once, with no Save of its own.
	await page.getByRole('switch', { name: /Reduce transparency/ }).click();
	await expect(page.locator('html')).toHaveAttribute('data-solid', '');
	await expect
		.poll(async () => (await page.request.get(`${state.baseURL}/api/settings/appearance`)).json())
		.toMatchObject({ reduce_transparency: true });
	await page.getByRole('button', { name: 'Switch back to Graphite' }).click();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'graphite');
	// With a built-in theme in use the editor offers no switch back (bug 130).
	await expect(page.getByRole('button', { name: /Switch back to/ })).toHaveCount(0);
	// Overview widgets (bug 31): hiding one and moving another changes the start page; both are saved at once.
	const widgetList = page.getByRole('list', { name: 'Overview widgets' }).getByRole('listitem');
	await page.getByRole('switch', { name: 'Show Recent activity' }).click();
	await expect(page.getByRole('switch', { name: 'Show Recent activity' })).not.toBeChecked();
	await page.getByRole('button', { name: 'Move Edge hosts up' }).click();
	await expect(widgetList.nth(2)).toContainText('Edge hosts');
	await page.goto(`${state.baseURL}/overview`);
	await expect(page.getByRole('main').getByRole('heading', { level: 2 })).toHaveText([
		'Changes waiting',
		'Needs attention',
		'Edge hosts',
		'Service health',
		'Security, last 24 h',
		'Certificates'
	]);
	await page.goto(`${state.baseURL}/settings?category=appearance`);
	await page.getByRole('button', { name: 'Move Edge hosts down' }).click();
	await expect(widgetList.nth(3)).toContainText('Edge hosts');
	await page.getByRole('switch', { name: 'Show Recent activity' }).click();
	await expect(page.getByRole('switch', { name: 'Show Recent activity' })).toBeChecked();
	// Times in UTC: Activity shows them with the zone.
	await page.goto(`${state.baseURL}/settings?category=general`);
	await page.getByRole('radio', { name: 'UTC' }).click();
	// Leaving with an unsaved change asks in the app's own dialog (bug 108); Stay keeps the change.
	await page.getByRole('link', { name: 'Activity' }).first().click();
	await page
		.getByRole('alertdialog', { name: 'Leave without saving?' })
		.getByRole('button', { name: 'Stay' })
		.click();
	await expect(page).toHaveURL(/category=general/);
	await expect(page.getByRole('radio', { name: 'UTC' })).toBeChecked();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Saved');
	await page.goto(`${state.baseURL}/activity`);
	await expect(page.getByRole('cell', { name: /UTC/ }).first()).toBeVisible();
	await page.goto(`${state.baseURL}/settings?category=general`);
	await page.getByRole('radio', { name: 'Local time' }).click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Saved');
	// Public pages: the dashboard starts sorted by name; new services start hidden from it. The old address still works.
	await page.goto(`${state.baseURL}/settings?category=dashboard`);
	await expect(page).toHaveURL(/category=public/);
	await page.getByRole('radio', { name: 'Name' }).click();
	await page.getByRole('switch', { name: /New services show/ }).click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Saved');
	expect(await (await page.request.get(`${state.appsURL}/api/public/apps`)).json()).toMatchObject({
		default_sort: 'name'
	});
	// The public pages draw in the browser: a crash there shows "500" while the server answers 200 (bug 139).
	for (const url of [state.appsURL, state.statusURL]) {
		const crashes: string[] = [];
		page.on('pageerror', (e) => crashes.push(e.message));
		await page.goto(url);
		await expect(page.getByText('Internal Error')).toHaveCount(0);
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
		expect(crashes, `${url} threw in the browser`).toEqual([]);
		page.removeAllListeners('pageerror');
	}
	await page.goto(`${state.baseURL}/services`);
	await page.getByRole('button', { name: 'Add service' }).click();
	// The switch differs from no default here, so More options stays plain (10c §3.1).
	await expect(page.getByRole('button', { name: 'More options', exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'More options' }).click();
	await expect(page.getByRole('switch', { name: 'Show on the app dashboard' })).not.toBeChecked();
	await page.keyboard.press('Escape');
	// Monitoring defaults and the firewall's MSS switch save (their effect is tested in Go).
	await page.goto(`${state.baseURL}/settings?category=monitoring`);
	await page.getByLabel('Check every (seconds)').fill('120');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Checks that exist keep their own values');
	await page.goto(`${state.baseURL}/settings?category=firewall`);
	await expect(page.getByLabel('When Hashi finds NetBird on a new host')).toContainText(
		'The host is a NetBird peer'
	);
	await page.screenshot({ path: 'test-results/s11-settings-firewall.png', fullPage: true });

	// S11 visual tests (10-ui §2): every main page in every built-in theme keeps readable contrast (axe), fits a phone
	// without sideways scrolling, and leaves a screenshot to look at.
	// Every top-level page (13.2 gate).
	const pages = [
		'/changes',
		'/services',
		'/dns',
		'/traefik',
		'/firewall',
		'/pulse',
		'/overview',
		'/security',
		'/ip-sets',
		'/ip-sets/address',
		'/rule-sets',
		'/status',
		'/app-display',
		'/scripts',
		'/activity',
		'/connections',
		'/settings?category=appearance'
	];
	for (const [id, name] of [
		['graphite', 'Graphite'],
		['linen', 'Warm Linen'],
		['verdant', 'Verdant'],
		['violet', 'Violet Night']
	]) {
		await page.goto(`${state.baseURL}/services`);
		await page.getByRole('button', { name: 'Theme', exact: true }).click();
		await page.getByRole('menuitemradio', { name }).click();
		await expect(page.locator('html')).toHaveAttribute('data-theme', id!);
		// The theme is saved in the background; every page load applies the saved one, so wait for it.
		await expect
			.poll(async () => (await page.request.get(`${state.baseURL}/api/settings/appearance`)).json())
			.toMatchObject({ theme: id });
		for (const path of pages) {
			await page.goto(`${state.baseURL}${path}`);
			await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
			await expect(page.locator('html')).toHaveAttribute('data-theme', id!);
			await expect(page, `title of ${path}`).toHaveTitle(/^[^·]+ · Hashi$/); // bug 60
			await page.waitForLoadState('networkidle');
			const axe = await new AxeBuilder({ page }).withRules(['color-contrast']).analyze();
			expect(
				axe.violations.flatMap((v) =>
					v.nodes.map((n) => `${n.target.join(' ')}: ${n.failureSummary}`)
				),
				`contrast in ${id} on ${path}`
			).toEqual([]);
			await page.setViewportSize({ width: 390, height: 844 });
			expect(
				await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
				`${path} is wider than a phone in ${id}`
			).toBe(true);
			const slug = path.replace(/[^a-z]+/g, '-').replace(/^-|-$/g, '');
			await page.screenshot({
				path: `test-results/visual/${id}-${slug}-phone.png`,
				fullPage: true
			});
			await page.setViewportSize({ width: 1280, height: 800 });
			await page.screenshot({ path: `test-results/visual/${id}-${slug}.png`, fullPage: true });
		}
	}

	// Bug 41: the open tab lives in the address and survives a reload.
	await page.goto(`${state.baseURL}/security`);
	await page.getByRole('tab', { name: 'Counters' }).click();
	await expect(page).toHaveURL(/[?&]tab=counters/);
	await page.reload();
	await expect(page.getByRole('tab', { name: 'Counters' })).toHaveAttribute(
		'aria-selected',
		'true'
	);

	// Bug 91: an unsaved change in a Settings category survives switching to another category and back.
	await page.goto(`${state.baseURL}/settings?category=pulse`);
	const heartbeat = page.getByLabel('Heartbeat every (seconds)');
	const before = await heartbeat.inputValue();
	await heartbeat.fill(String(Number(before) + 7));
	const cats = page.getByRole('navigation', { name: 'Settings categories' });
	await cats.getByRole('button', { name: 'General' }).click();
	await cats.getByRole('button', { name: 'Pulse' }).click();
	await expect(heartbeat).toHaveValue(String(Number(before) + 7));
	// Bug 40: leaving with an unsaved change asks first; dismissing keeps the page and the draft.
	page.once('dialog', (d) => void d.dismiss());
	await page.getByRole('link', { name: 'Services' }).first().click();
	await expect(page).toHaveURL(/\/settings/);
	await expect(heartbeat).toHaveValue(String(Number(before) + 7));
	await heartbeat.fill(before);

	// S12: strict vault mode (02 §5.4). A PRF passkey becomes an unlocker, strict mode is switched on, and after each
	// restart Hashi is locked until the recovery key or the passkey unlocks it; the session survives the restarts.
	await page.goto(`${state.baseURL}/settings?category=security`);
	// The vault's own button links a passkey that already signs in (one prompt, that passkey only).
	await page.getByRole('button', { name: 'Add a passkey' }).click();
	const vaultAdd = page.getByRole('dialog', { name: 'Add a passkey that unlocks the vault' });
	await vaultAdd.getByLabel(/E2E authenticator/).check();
	await page.screenshot({ path: 'test-results/visual/passkey-vault-choose.png' });
	await vaultAdd.getByRole('button', { name: 'Continue' }).click();
	await vaultAdd.getByRole('button', { name: 'Link E2E authenticator' }).click();
	await expect(vaultAdd.getByRole('status')).toHaveText(
		'E2E authenticator can unlock the vault now.'
	);
	await vaultAdd.getByRole('button', { name: 'Done' }).click();
	await page.getByRole('button', { name: 'Switch to strict mode…' }).click();
	await page.getByRole('button', { name: 'Switch to strict mode', exact: true }).click();
	await expect(page.getByText(/Strict mode is on/)).toBeVisible();
	await restartHashi();
	await page.reload();
	await expect(page.getByText('Vault locked.')).toBeVisible();
	await page.getByRole('main').getByRole('button', { name: 'Unlock', exact: true }).click();
	await page.getByLabel('Or the recovery key').fill(key);
	await page.getByRole('button', { name: 'Unlock with the recovery key' }).click();
	await expect(page.getByText('Vault locked.')).toHaveCount(0);
	await restartHashi();
	await page.reload();
	await expect(page.getByText('Vault locked.')).toBeVisible();
	await page.getByRole('main').getByRole('button', { name: 'Unlock', exact: true }).click();
	await page.getByRole('button', { name: 'Unlock with a passkey' }).click();
	await expect(page.getByText('Vault locked.')).toHaveCount(0);
	await page.getByRole('button', { name: 'Switch to service mode…' }).click();
	await page.getByRole('button', { name: 'Switch to service mode', exact: true }).click();
	await expect(page.getByText(/Service mode is on/)).toBeVisible();

	// Deleting the passkey this session signed in with ends the session: the page goes to sign-in.
	await page.goto(`${state.baseURL}/settings?category=security`);
	await page.getByRole('button', { name: 'Delete passkey E2E authenticator' }).click();
	await page.getByRole('button', { name: 'Delete passkey', exact: true }).click();
	await expect(page).toHaveURL(/\/login/);
	await expect(page.getByText('Your session ended')).toBeVisible();
});

test('console recovery link registers a replacement passkey', async ({ browser }) => {
	const out = execFileSync(state.bin, ['admin', 'recover'], {
		env: { ...process.env, HASHI_DATA_DIR: state.dir, HASHI_MASTER_KEY_FILE: state.keyFile }
	}).toString();
	const token = /#(\S+)/.exec(out)?.[1];
	expect(token).toBeTruthy();
	const page = await (await browser.newContext()).newPage();
	await virtualAuthenticator(page);
	await page.goto(`${state.baseURL}/login/recover#${token}`);
	await expect(page).toHaveURL(/\/login\/recover$/); // token removed from the address bar
	await page.getByLabel('Passkey name').fill('Recovered');
	await page.getByRole('button', { name: 'Create passkey' }).click();
	await expect(page.getByText('The new passkey is saved')).toBeVisible();
	await page.getByRole('button', { name: 'Go to sign-in' }).click();
	await page.getByRole('button', { name: 'Sign in with a passkey' }).click();
	await expect(page).toHaveURL(/\/overview$/);
	// The link works once.
	const again = await page.request.post(`${state.baseURL}/api/auth/recover`, { data: { token } });
	expect(again.status()).toBe(403);
});
