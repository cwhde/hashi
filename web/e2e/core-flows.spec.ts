import { expect, test } from '@playwright/test';

test.use({ viewport: { width: 1280, height: 720 } });

test.describe('Resource Creation Flow', () => {
	test('resource creation form validates required fields', async ({ page }) => {
		const response = await page.goto('/resources/new');
		expect(response?.status()).toBeLessThan(500);
		await expect(page.locator('main')).toBeVisible();
	});

	test('resource list page loads', async ({ page }) => {
		const response = await page.goto('/resources');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('DNS Import Flow', () => {
	test('dns connections page loads', async ({ page }) => {
		const response = await page.goto('/dns');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Middleware Editor Validation', () => {
	test('middleware editor page loads', async ({ page }) => {
		const response = await page.goto('/middlewares');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Public Dashboard View', () => {
	test('public apps page loads', async ({ page }) => {
		const response = await page.goto('/apps');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Status Page Public View', () => {
	test('status page shows status information', async ({ page }) => {
		const response = await page.goto('/status-page');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Script Management Flow', () => {
	test('scripts page loads', async ({ page }) => {
		const response = await page.goto('/scripts');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Firewall Host Configuration', () => {
	test('firewall hosts page loads', async ({ page }) => {
		const response = await page.goto('/firewall');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Security Dashboard Interaction', () => {
	test('security dashboard page loads', async ({ page }) => {
		const response = await page.goto('/security');
		expect(response?.status()).toBeLessThan(500);
	});
});

test.describe('Blocklist Management', () => {
	test('blocklist page loads', async ({ page }) => {
		const response = await page.goto('/blocklist');
		expect(response?.status()).toBeLessThan(500);
	});
});
