import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { statePath } from './global-setup';

// S1 gate (11-delivery.md §3): passkey registration and login pass with a virtual authenticator, end to end through
// the real binary: bootstrap → base settings → passkey → recovery key → finish → passkey sign-in → account pages.
const state = JSON.parse(readFileSync(statePath, 'utf8')) as {
	baseURL: string;
	bin: string;
	dir: string;
	keyFile: string;
	username: string;
	password: string;
};

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
			automaticPresenceSimulation: true
		}
	});
	// Detaches this authenticator (and its passkeys) from the browser.
	return () => cdp.send('WebAuthn.removeVirtualAuthenticator', { authenticatorId });
}

test('setup with a passkey, then sign in with it', async ({ page, request }) => {
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

	// Step 7: the Hashi admin service; its deeper name re-issues the certificate first, then the plan is shown.
	await expect(page.getByRole('heading', { name: 'Publish Hashi' })).toBeVisible();
	await page.getByRole('button', { name: 'Prepare' }).click();
	await expect(page.getByText(/Certificate: Applied: issued example.com \(3 names/)).toBeVisible();
	await expect(page.getByText('10-hashi-http-services.yml')).toBeVisible();
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

	// Step 11: finish (steps of later slices count as done only in this dev build).
	await expect(page.getByRole('heading', { name: 'Finish setup' })).toBeVisible();
	await page.getByRole('button', { name: 'Finish setup' }).click();
	await expect(page).toHaveURL(/\/login$/);

	// Passkey sign-in.
	await page.getByRole('button', { name: 'Sign in with a passkey' }).click();
	await expect(page).toHaveURL(/\/activity$/);
	await expect(page.getByRole('cell', { name: 'setup.completed' })).toBeVisible();
	await expect(page.getByRole('cell', { name: 'auth.passkey.login' })).toBeVisible();

	// DNS: the observed zone with the decisions from setup; a changed decision survives a reload.
	await page.getByRole('link', { name: 'DNS' }).first().click();
	await expect(page).toHaveURL(/\/dns$/);
	const records = page.getByRole('table', { name: 'DNS records' });
	await expect(records.getByRole('row', { name: /^svc1 A / })).toContainText('imported');
	await expect(records.getByRole('row', { name: /^@ SOA / })).toContainText('protected');
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
	// Deleting asks for the passkey (reauth), then the service is gone.
	await page.getByRole('button', { name: 'Delete' }).click();
	await page.getByRole('button', { name: 'Delete service' }).click();
	await expect(page).toHaveURL(/\/services$/);
	await expect(page.getByRole('link', { name: /^Search/ })).toHaveCount(0);

	// Account: the passkey and this session; the last passkey cannot be deleted.
	await page.goto(`${state.baseURL}/settings?category=account`);
	await expect(page.getByText('E2E authenticator')).toBeVisible();
	await expect(page.getByText('this session')).toBeVisible();
	await expect(
		page.getByRole('button', { name: 'Delete passkey E2E authenticator' })
	).toBeDisabled();

	// Reauth-protected action: adding a passkey first confirms with the existing one. The same authenticator then
	// refuses a second credential (Hashi sends existing ones as exclusions), so a second authenticator is attached.
	await page.getByLabel('New passkey name').fill('Second key');
	await page.getByRole('button', { name: 'Add passkey' }).click();
	await expect(page.getByRole('alert')).toContainText('already holds a passkey');
	const removeUsb = await virtualAuthenticator(page, 'usb');
	await page.getByRole('button', { name: 'Add passkey' }).click();
	await expect(page.getByText('Passkey added.')).toBeVisible();
	await expect(page.getByText('Second key')).toBeVisible();

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
	await expect(page).toHaveURL(/\/activity$/);

	// Deleting the passkey this session signed in with ends the session: the page goes to sign-in.
	await page.goto(`${state.baseURL}/settings?category=account`);
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
	await expect(page).toHaveURL(/\/activity$/);
	// The link works once.
	const again = await page.request.post(`${state.baseURL}/api/auth/recover`, { data: { token } });
	expect(again.status()).toBe(403);
});
