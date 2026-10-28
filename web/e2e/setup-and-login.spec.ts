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

test('setup with a passkey, then sign in with it', async ({ page }) => {
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
	await page.getByRole('button', { name: 'Save decisions and continue' }).click();

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
