import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { statePath } from './global-setup';

// S1 gate (11-delivery.md §3): passkey registration and login pass with a virtual authenticator, end to end through
// the real binary: bootstrap → base settings → passkey → recovery key → finish → passkey sign-in → account pages.
const state = JSON.parse(readFileSync(statePath, 'utf8')) as {
	baseURL: string;
	username: string;
	password: string;
};

async function virtualAuthenticator(page: Page, transport: 'internal' | 'usb' = 'internal') {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('WebAuthn.enable');
	await cdp.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport,
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true,
			automaticPresenceSimulation: true
		}
	});
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
	await page.getByLabel('Root domain').fill('example.com');
	await page.getByLabel('Admin domain').fill('hashi.example.com');
	await page.getByRole('button', { name: 'Save and continue' }).click();

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
	await virtualAuthenticator(page, 'usb');
	await page.getByRole('button', { name: 'Add passkey' }).click();
	await expect(page.getByText('Passkey added.')).toBeVisible();
	await expect(page.getByText('Second key')).toBeVisible();

	// Bootstrap login is gone after setup.
	const res = await page.request.post(`${state.baseURL}/api/auth/bootstrap/login`, {
		data: { username: state.username, password: state.password } // hashi:allow-secret-pattern (runtime value)
	});
	expect(res.status()).toBe(403);

	// Sign out ends the session server-side.
	await page.getByRole('button', { name: 'Account', exact: true }).first().click();
	await page.getByRole('menuitem', { name: 'Sign out' }).click();
	await expect(page).toHaveURL(/\/login$/);
	const s = await (await page.request.get(`${state.baseURL}/api/auth/session`)).json();
	expect(s.authenticated).toBe(false);
});
