import { expect, test } from '@playwright/test';

test.use({ viewport: { width: 1280, height: 720 } });

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
