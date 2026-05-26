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
