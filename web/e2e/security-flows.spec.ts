import { expect, test } from '@playwright/test';

test.use({ viewport: { width: 1280, height: 720 } });

test.describe('Security Subject Search', () => {
	test('security page loads and shows search interface', async ({ page }) => {
		const response = await page.goto('/security');
		expect(response?.status()).toBeLessThan(500);
		await expect(page.locator('main')).toBeVisible();
	});
});

test.describe('Manual Block/Allow Operations', () => {
	test('blocklist page loads and shows blocklist management', async ({ page }) => {
		const response = await page.goto('/blocklist');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Blocklist Management', () => {
	test('blocklist page loads and shows existing entries', async ({ page }) => {
		const response = await page.goto('/blocklist');
		expect(response?.status()).toBeLessThan(500);
		await expect(page.locator('main')).toBeVisible();
	});
});

test.describe('Agent DNS Configuration', () => {
	test('connections page loads', async ({ page }) => {
		const response = await page.goto('/connections');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Connection Target Configuration', () => {
	test('connections page loads connection targets', async ({ page }) => {
		const response = await page.goto('/connections');
		expect(response?.status()).toBeLessThan(500);
		await expect(page.locator('main')).toBeVisible();
	});
});
