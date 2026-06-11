import { expect, test, type Page, type Route } from '@playwright/test';

test.use({ viewport: { width: 1280, height: 720 } });

const connectionId = '11111111-1111-1111-1111-111111111111';

async function json(route: Route, body: unknown, status = 200) {
	await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
}

async function mockAdmin(page: Page) {
	await page.route('**/api/setup/status', (route) =>
		json(route, {
			isComplete: true,
			currentStep: 'complete',
			completedSteps: [],
			httpsDomainVerified: true,
			updatedAtUtc: new Date().toISOString()
		})
	);
	await page.route('**/api/auth/session', (route) =>
		json(route, { isAuthenticated: true, username: 'e2e-admin', vaultUnlocked: true })
	);
	await page.route('**/api/auth/csrf', (route) => json(route, { token: 'e2e-csrf' }));
	await page.route('**/api/**', (route) => route.fallback());
}

test('creates a resource through the real resources page', async ({ page }) => {
	await mockAdmin(page);
	let requestBody: Record<string, unknown> | null = null;
	await page.route(/\/api\/resources(?:\?.*)?$/, async (route) => {
		if (route.request().method() === 'POST') {
			requestBody = route.request().postDataJSON() as Record<string, unknown>;
			return json(route, { id: '22222222-2222-2222-2222-222222222222' }, 201);
		}
		return json(route, []);
	});

	await page.goto('/resources');
	await expect(page).toHaveURL(/\/resources$/);
	await expect(page.getByRole('heading', { name: 'Resources', level: 2 })).toBeVisible();
	await page.getByLabel('Name').fill('Grafana');
	await page.getByRole('textbox', { name: 'Domain', exact: true }).fill('grafana');
	await page.getByLabel('Target host').fill('10.0.0.12');
	await page.getByLabel('Target port').fill('3000');
	await page.getByRole('button', { name: 'Create resource' }).click();

	await expect.poll(() => requestBody).not.toBeNull();
	expect(requestBody).toMatchObject({
		name: 'Grafana',
		domain: 'grafana',
		targetHost: '10.0.0.12',
		targetPort: 3000
	});
});

test('previews and applies a DNS import', async ({ page }) => {
	await mockAdmin(page);
	await page.route('**/api/dns/connections', (route) =>
		json(route, [
			{
				id: connectionId,
				name: 'Hetzner',
				type: 'dns_provider',
				enabled: true,
				healthState: 'healthy',
				lastValidationMessage: null,
				lastValidatedAtUtc: null
			}
		])
	);
	await page.route('**/api/dns/zones', (route) => json(route, []));
	await page.route('**/api/dns/records', (route) => json(route, []));
	await page.route(/\/api\/dns\/connections\/[^/]+\/import\/preview(?:\?.*)?$/, (route) =>
		json(route, [
			{
				id: '33333333-3333-3333-3333-333333333333',
				providerRecordId: 'provider-1',
				name: 'grafana.example.test',
				type: 'A',
				value: '203.0.113.10',
				selectedForImport: true
			}
		])
	);
	let importedIds: string[] = [];
	await page.route(/\/api\/dns\/connections\/[^/]+\/import\/apply(?:\?.*)?$/, async (route) => {
		importedIds = (route.request().postDataJSON() as { selectedDecisionIds: string[] })
			.selectedDecisionIds;
		return json(route, { succeeded: true });
	});

	await page.goto('/dns');
	await expect(page.getByRole('heading', { name: 'DNS', level: 2 })).toBeVisible();
	await page.getByRole('button', { name: 'Import', exact: true }).click();
	await expect(page.getByText('grafana.example.test')).toBeVisible();
	await page.getByRole('button', { name: 'Import 1 records' }).click();
	await expect(page.getByText('Imported 1 DNS records into Hashi.')).toBeVisible();
	expect(importedIds).toEqual(['33333333-3333-3333-3333-333333333333']);
});


for (const [path, heading] of [
	['/traefik', 'Traefik'],
	['/scripts', 'Scripts'],
	['/firewall-hosts', 'Firewall Hosts'],
	['/connections', 'Connections']
] as const) {
	test(`${heading} administrative route renders`, async ({ page }) => {
		await mockAdmin(page);
		await page.goto(path);
		await expect(page).toHaveURL(new RegExp(`${path}$`));
		await expect(page.getByRole('heading', { name: heading, level: 2 })).toBeVisible();
	});
}

test('public dashboard renders API data and search filters it', async ({ page }) => {
	await page.route('**/api/public/apps', (route) =>
		json(route, {
			totalHosts: 1,
			hostsOnline: 1,
			totalLinuxFirewallHosts: 1,
			linuxFirewallHostsAvailable: 1,
			items: [
				{
					id: 'app-1',
					displayName: 'Grafana',
					domain: 'grafana.example.test',
					publicUrl: 'https://grafana.example.test',
					status: 'Up'
				}
			]
		})
	);
	await page.goto('/dashboard');
	await expect(page.getByRole('heading', { name: 'Homelab Dashboard' })).toBeVisible();
	await expect(page.getByRole('link', { name: /Grafana/ })).toBeVisible();
	await page.getByRole('button', { name: 'Search' }).click();
	await page.getByPlaceholder('Search services...').fill('missing');
	await expect(page.getByText('No public dashboard tiles configured.')).toBeVisible();
});

test('public status page renders monitor data', async ({ page }) => {
	await page.route('**/api/public/status', (route) =>
		json(route, [
			{
				name: 'Grafana',
				status: 'Up',
				lastLatencyMs: 12,
				recentStrip: [{ up: true }]
			}
		])
	);
	await page.goto('/status-page');
	await expect(page.getByRole('heading', { name: 'Status' })).toBeVisible();
	await expect(page.getByText('Grafana')).toBeVisible();
	await expect(page.getByText('12 ms')).toBeVisible();
});
