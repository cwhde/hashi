import { expect, test } from '@playwright/test';

test.use({ viewport: { width: 1280, height: 720 }, serviceWorkers: 'block' });

test('setup wizard loads on fresh install', async ({ page }) => {
	const response = await page.goto('/setup');
	expect(response?.status()).toBeLessThan(500);

	await expect(page.locator('main')).toBeVisible();
	await expect(
		page
			.getByRole('heading', { name: /bootstrap access/i })
			.or(page.getByText('Setup error'))
			.or(page.getByText('Loading setup state'))
	).toBeVisible({ timeout: 15_000 });
});

test('login page renders', async ({ page }) => {
	const response = await page.goto('/login');
	expect(response?.status()).toBeLessThan(500);
	await expect(page.getByText('Hashi Admin')).toBeVisible();
});

test('public status page loads', async ({ page }) => {
	const response = await page.goto('/status-page');
	expect(response?.status()).toBeLessThan(500);
});

test('registers a passkey with a virtual WebAuthn authenticator', async ({ page }) => {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('WebAuthn.enable');
	await cdp.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport: 'internal',
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true
		}
	});

	await page.route('**/api/setup/status', (route) =>
		route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({
				isComplete: false,
				currentStep: 'passkey-and-vault',
				completedSteps: [
					'bootstrap-access',
					'base-settings',
					'dns-provider',
					'certificate-provider',
					'traefik-connection',
					'firewall-host',
					'system-resource'
				],
				httpsDomainVerified: true,
				updatedAtUtc: new Date().toISOString()
			})
		})
	);
	await page.route('**/api/auth/csrf', (route) =>
		route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ token: 'e2e-csrf-token' })
		})
	);
	await page.route('**/api/vault/recovery-key/generate', (route) =>
		route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ recoveryKey: 'test-recovery-key' })
		})
	);
	await page.route('**/api/vault/status', (route) =>
		route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ serviceSyncVaultReady: false })
		})
	);
	let beginCalled = false;
	await page.route(/\/api\/auth\/passkeys\/register\/begin(?:\?.*)?$/, async (route) => {
		beginCalled = true;
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({
				challengeSessionId: 'challenge-session-1',
				options: {
					challenge: 'AQIDBAUGBwgJCgsMDQ4PEA',
					rp: { id: 'localhost', name: 'Hashi' },
					user: { id: 'AQIDBA', name: 'admin', displayName: 'Admin' },
					pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
					timeout: 60_000,
					attestation: 'none',
					authenticatorSelection: {
						residentKey: 'preferred',
						userVerification: 'preferred'
					}
				}
			})
		});
	});
	let registration: Record<string, unknown> | null = null;
	await page.route('**/api/auth/passkeys/register/complete', async (route) => {
		registration = route.request().postDataJSON() as Record<string, unknown>;
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ credentialId: 'credential-1', prfSupported: false })
		});
	});

	await page.goto('http://localhost:8080/setup');
	await expect(page.getByRole('heading', { name: /passkey & vault/i })).toBeVisible();
	await page.getByRole('button', { name: 'Register passkey' }).click();

	await expect.poll(() => beginCalled).toBe(true);
	await expect(page.getByText('Passkey registered. Vault will use recovery key wrap.')).toBeVisible();
	await expect.poll(() => registration).not.toBeNull();
	expect(registration).toMatchObject({
		challengeSessionId: 'challenge-session-1',
		nickname: 'Primary passkey'
	});
	expect((registration as { attestation: { response: Record<string, unknown> } }).attestation.response)
		.toMatchObject({ clientDataJSON: expect.any(String), attestationObject: expect.any(String) });
});

test('optional setup step skip advances to complete', async ({ page }) => {
	let currentStep = 'optional';
	const completedBeforeOptional = [
		'bootstrap-access',
		'base-settings',
		'dns-provider',
		'certificate-provider',
		'traefik-connection',
		'firewall-host',
		'system-resource',
		'passkey-and-vault'
	];

	await page.route('**/api/auth/csrf', async (route) => {
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({ token: 'e2e-csrf-token' })
		});
	});

	await page.route('**/api/setup/status', async (route) => {
		const completedSteps =
			currentStep === 'optional'
				? completedBeforeOptional
				: [...completedBeforeOptional, 'optional'];
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({
				isComplete: false,
				currentStep,
				completedSteps,
				httpsDomainVerified: true,
				updatedAtUtc: new Date().toISOString()
			})
		});
	});

	await page.route('**/api/setup/steps/optional/complete', async (route) => {
		currentStep = 'complete';
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: '{}'
		});
	});

	const response = await page.goto('/setup');
	expect(response?.status()).toBeLessThan(500);

	await expect(page.getByRole('heading', { name: /optional setup/i })).toBeVisible();
	await expect(page.getByRole('button', { name: /skip optional/i })).toBeEnabled();
	await page.getByRole('button', { name: /skip optional/i }).click();
	await expect(page.getByRole('heading', { name: /^complete$/i, level: 2 })).toBeVisible({
		timeout: 15_000
	});
});
