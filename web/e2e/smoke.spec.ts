import { expect, test } from '@playwright/test';

test('login page renders', async ({ page }) => {
	await page.goto('/login');
	await expect(page.getByRole('heading', { name: /sign in|login|hashi/i })).toBeVisible();
});

test('public status page loads', async ({ page }) => {
	const response = await page.goto('/status-page');
	expect(response?.status()).toBeLessThan(500);
});
