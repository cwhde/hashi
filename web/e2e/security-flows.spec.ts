import { expect, test, type Page, type Route } from '@playwright/test';

test.use({ viewport: { width: 1280, height: 900 } });

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
	await page.route('**/api/security/dashboard**', (route) =>
		json(route, {
			allowed: 0,
			blocked: 0,
			challenged: 0,
			wafDetections: 0,
			wafBlocks: 0,
			hours: 24,
			resourceFilter: null,
			traefikHostFilter: null,
			firewallHostIdFilter: null,
			resourceOptions: [],
			traefikHostOptions: [],
			firewallHostOptions: [],
			topBlockedIps: [],
			topChallengedIps: [],
			topResources: [],
			topCountries: [],
			topAsns: [],
			recentManualActions: [],
			blocklistMatchesOverTime: [],
			captchaOutcomes: { solved: 0, failed: 0, ignored: 0 },
			activeSoftBlocks: [],
			activeFirewallBlocks: [],
			staleBlocklistSources: [],
			geoIpStatus: {
				enabled: false,
				databaseAvailable: false,
				isStale: false,
				lastUpdateStatus: 'disabled',
				lastUpdateMessage: null,
				lastUpdateAtUtc: null,
				nextUpdateAtUtc: null,
				missingDatabases: [],
				staleDatabases: []
			},
			blocklistCount: 0,
			firewallActiveIpBlocks: 0
		})
	);
	await page.route('**/api/auth/csrf', (route) => json(route, { token: 'e2e-csrf' }));
	await page.route('**/api/**', (route) => route.fallback());
}

function subjectDetail(manualEntries: unknown[] = []) {
	return {
		subject: {
			id: 'subject-1',
			subjectType: 'ip',
			subjectValue: '203.0.113.7',
			normalizedValue: '203.0.113.7',
			currentState: 'observed',
			firstSeenAtUtc: new Date().toISOString(),
			lastSeenAtUtc: new Date().toISOString(),
			lastCountry: null,
			lastRegion: null,
			lastAsn: null,
			lastAsOrg: null
		},
		state: null,
		manualEntries,
		blocklistEntries: [],
		resourceRules: [],
		firewallApplications: []
	};
}

test('searches an IP and displays its security timeline', async ({ page }) => {
	await mockAdmin(page);
	await page.route('**/api/security/subjects/search**', (route) =>
		json(route, { results: [subjectDetail().subject] })
	);
	await page.route('**/api/security/subjects/subject-1', (route) => json(route, subjectDetail()));
	await page.route('**/api/security/subjects/subject-1/effective-decision', (route) =>
		json(route, { decision: 'allow', reasons: [], matchedManualEntryIds: [] })
	);
	await page.route('**/api/security/subjects/subject-1/events', (route) =>
		json(route, [
			{
				id: 'event-1',
				occurredAtUtc: new Date().toISOString(),
				resourceId: null,
				eventType: 'request',
				severity: 'info',
				decision: 'allow',
				source: 'forward-auth',
				reason: 'observed',
				requestMethod: 'GET',
				requestPath: '/',
				statusCode: 200
			}
		])
	);
	await page.route('**/api/security/subjects/subject-1/buckets**', (route) => json(route, []));

	await page.goto('/security');
	await expect(page.getByRole('heading', { name: 'Security', level: 2 })).toBeVisible();
	await page.getByLabel('Subject search').fill('203.0.113.7');
	await page.getByRole('button', { name: 'Search' }).click();
	await expect(page.getByText('203.0.113.7').first()).toBeVisible();
	await expect(page.getByText('Timeline')).toBeVisible();
	await expect(page.getByText('request / observed')).toBeVisible();
});

test('creates a manual soft block for a selected IP', async ({ page }) => {
	await mockAdmin(page);
	await page.route('**/api/security/subjects/search**', (route) =>
		json(route, { results: [subjectDetail().subject] })
	);
	await page.route('**/api/security/subjects/subject-1', (route) => json(route, subjectDetail()));
	await page.route('**/api/security/subjects/subject-1/effective-decision', (route) =>
		json(route, { decision: 'allow', reasons: [], matchedManualEntryIds: [] })
	);
	await page.route('**/api/security/subjects/subject-1/events', (route) => json(route, []));
	await page.route('**/api/security/subjects/subject-1/buckets**', (route) => json(route, []));
	let blockBody: Record<string, unknown> | null = null;
	await page.route('**/api/security/blocks', async (route) => {
		blockBody = route.request().postDataJSON() as Record<string, unknown>;
		return json(route, { id: 'block-1' }, 201);
	});

	await page.goto('/security');
	await page.getByLabel('Subject search').fill('203.0.113.7');
	await page.getByRole('button', { name: 'Search' }).click();
	await page.getByLabel('Action reason').fill('E2E verification');
	await page.getByRole('button', { name: 'Soft block' }).click();

	await expect.poll(() => blockBody).not.toBeNull();
	expect(blockBody).toMatchObject({
		subjectType: 'ip',
		subjectValue: '203.0.113.7',
		blockType: 'soft',
		reason: 'E2E verification',
		firewallEnforced: false
	});
});

test('adds a custom HTTPS blocklist source', async ({ page }) => {
	await mockAdmin(page);
	let createdSource: Record<string, unknown> | null = null;
	await page.route('**/api/security/blocklists', async (route) => {
		if (route.request().method() === 'POST') {
			createdSource = {
				id: 'source-1',
				...(route.request().postDataJSON() as object),
				enabled: true,
				lastFetchStatus: 'never',
				lastFetchError: null,
				lastFetchedAtUtc: null,
				entryCount: 0,
				isStale: false,
				metadataJson: null
			};
			return json(route, createdSource, 201);
		}
		return json(route, createdSource ? [createdSource] : []);
	});

	await page.goto('/security');
	await page.getByPlaceholder('Name').fill('Threat feed');
	await page.getByPlaceholder('https://example.test/feed.txt').fill('https://example.test/feed.txt');
	await page.getByRole('button', { name: 'Add source' }).click();
	await expect(page.getByText('Threat feed', { exact: true })).toBeVisible();
});

test('challenge and connection-target routes are real and render', async ({ page }) => {
	await page.route('**/api/**', (route) => json(route, []));
	await page.goto('/challenge');
	await expect(page.locator('main')).toBeVisible();

	await mockAdmin(page);
	await page.goto('/connections');
	await expect(page.getByRole('heading', { name: 'Connections', level: 2 })).toBeVisible();
	await expect(page.getByText('Registered connections')).toBeVisible();
});
