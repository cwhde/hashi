import { expect, test, type Page, type Route } from '@playwright/test';

test.use({ viewport: { width: 1280, height: 900 }, serviceWorkers: 'block' });

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

test('creates a manual allow for a selected IP', async ({ page }) => {
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
	let allowBody: Record<string, unknown> | null = null;
	await page.route('**/api/security/manual-entries', async (route) => {
		allowBody = route.request().postDataJSON() as Record<string, unknown>;
		return json(route, { id: 'allow-1' }, 201);
	});

	await page.goto('/security');
	await page.getByLabel('Subject search').fill('203.0.113.7');
	await page.getByRole('button', { name: 'Search' }).click();
	await page.getByLabel('Action reason').fill('Known administrator');
	await page.getByRole('button', { name: 'Allow', exact: true }).click();

	await expect.poll(() => allowBody).not.toBeNull();
	expect(allowBody).toMatchObject({
		subjectType: 'ip',
		subjectValue: '203.0.113.7',
		entryType: 'allow',
		scopeType: 'global',
		reason: 'Known administrator',
		isPermanent: true,
		enabled: true
	});
});

test('extends and shortens an active manual ban', async ({ page }) => {
	await mockAdmin(page);
	const activeBlock = {
		id: 'block-1',
		subjectType: 'ip',
		subjectValue: '203.0.113.7',
		normalizedValue: '203.0.113.7',
		entryType: 'block',
		scopeType: 'global',
		scopeId: null,
		reason: 'Repeated abuse',
		createdAtUtc: new Date().toISOString(),
		expiresAtUtc: new Date(Date.now() + 3_600_000).toISOString(),
		isPermanent: false,
		bypassBlocking: false,
		bypassAdaptiveEscalation: false,
		bypassRateLimit: false,
		bypassChallenge: false,
		bypassSso: false,
		enabled: true,
		lastHitAtUtc: null
	};
	await page.route('**/api/security/subjects/search**', (route) =>
		json(route, { results: [subjectDetail([activeBlock]).subject] })
	);
	await page.route('**/api/security/subjects/subject-1', (route) =>
		json(route, subjectDetail([activeBlock]))
	);
	await page.route('**/api/security/subjects/subject-1/effective-decision', (route) =>
		json(route, { decision: 'block', reasons: ['manual'], matchedManualEntryIds: ['block-1'] })
	);
	await page.route('**/api/security/subjects/subject-1/events', (route) => json(route, []));
	await page.route('**/api/security/subjects/subject-1/buckets**', (route) => json(route, []));
	const actions: { path: string; body: Record<string, unknown> }[] = [];
	await page.route('**/api/security/blocks/block-1/*', async (route) => {
		actions.push({
			path: new URL(route.request().url()).pathname,
			body: route.request().postDataJSON() as Record<string, unknown>
		});
		return json(route, { succeeded: true });
	});

	await page.goto('/security');
	await page.getByLabel('Subject search').fill('203.0.113.7');
	await page.getByRole('button', { name: 'Search' }).click();
	await page.getByLabel('Block duration').fill('6');
	await page.getByRole('button', { name: 'Extend', exact: true }).click();
	await expect.poll(() => actions.length).toBe(1);
	await page.getByRole('button', { name: 'Shorten', exact: true }).click();
	await expect.poll(() => actions.length).toBe(2);

	expect(actions).toEqual([
		{ path: '/api/security/blocks/block-1/extend', body: { durationSeconds: 21600 } },
		{ path: '/api/security/blocks/block-1/shorten', body: { durationSeconds: 21600 } }
	]);
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

test('solves a CAPTCHA challenge and submits the widget token', async ({ page }) => {
	await page.route('**/api/edge-challenge/status**', (route) =>
		json(route, {
			enabled: true,
			capApiEndpoint: 'https://captcha.example.test/api',
			safeReturnUrl: '/dashboard'
		})
	);
	await page.route('https://cdn.jsdelivr.net/npm/cap-widget**', (route) =>
		route.fulfill({
			status: 200,
			contentType: 'text/javascript',
			body: `customElements.define('cap-widget', class extends HTMLElement {
				connectedCallback() {
					this.textContent = 'Solve challenge';
					this.style.display = 'block';
					this.addEventListener('click', () => this.dispatchEvent(new CustomEvent('solve', {
						detail: { token: 'captcha-token-1' }
					})));
				}
			});`
		})
	);
	await page.route('**/api/auth/csrf', (route) => json(route, { token: 'csrf-token' }));
	let verification: Record<string, unknown> | null = null;
	await page.route('**/api/edge-challenge/verify', async (route) => {
		verification = JSON.parse(route.request().postData() ?? '{}') as Record<string, unknown>;
		return json(route, { verified: false, redirectUrl: null, error: 'Test verification received.' });
	});

	await page.goto('/challenge?returnUrl=%2Fdashboard');
	const widget = page.locator('cap-widget');
	await expect(widget).toBeAttached();
	await page.waitForFunction(() => customElements.get('cap-widget') !== undefined);
	await widget.evaluate((element) => {
		element.dispatchEvent(
			new CustomEvent('solve', { detail: { token: 'captcha-token-1' } })
		);
	});

	await expect.poll(() => verification).not.toBeNull();
	expect(verification).toMatchObject({ token: 'captcha-token-1', returnUrl: '/dashboard' });
	await expect(page.getByText('Test verification received.')).toBeVisible();
});

test('updates internal DNS settings for a Pulse agent', async ({ page }) => {
	await mockAdmin(page);
	const agent = {
		id: 'agent-1',
		name: 'edge-node-1',
		installType: 'linux_service',
		allowedScopes: [],
		heartbeatIntervalSeconds: 30,
		status: 'online',
		lastSeenAtUtc: new Date().toISOString(),
		lastPublicIp: '198.51.100.10',
		lastPrivateIp: '10.0.0.10',
		lastPrivateIpv4Candidates: ['10.0.0.10'],
		lastPrivateIpv6Candidates: [],
		lastSelectedIp: '10.0.0.10',
		lastSelectedInterface: 'eth0',
		lastHostname: 'edge-node-1',
		lastAgentVersion: '1.0.0',
		dnsPendingAtUtc: null
	};
	const settings = {
		enabled: true,
		domain: 'hashi.home.arpa',
		keepLastRewriteWhenAgentStale: true,
		adGuardConnectionId: 'adguard-1',
		lastSyncStatus: 'idle',
		lastAppliedHash: null,
		agents: [
			{
				id: 'dns-agent-1',
				pulseAgentId: 'agent-1',
				enabled: false,
				nameOverride: null,
				ipMode: 'selected',
				keepLastRewriteWhenStale: true,
				updatedAtUtc: new Date().toISOString()
			}
		]
	};
	await page.route('**/api/pulse/agents', (route) => json(route, [agent]));
	let dnsBody: Record<string, unknown> | null = null;
	await page.route('**/api/settings/internal-agent-dns/', async (route) => {
		if (route.request().method() === 'PUT') {
			dnsBody = route.request().postDataJSON() as Record<string, unknown>;
			return json(route, { ...settings, agents: [{ ...settings.agents[0], enabled: true }] });
		}
		return json(route, settings);
	});

	await page.goto('/pulse');
	await expect(page.getByRole('cell', { name: 'edge-node-1' }).first()).toBeVisible();
	await page.getByRole('switch', { checked: false }).click();
	await page.getByRole('button', { name: 'Save DNS' }).click();

	await expect.poll(() => dnsBody).not.toBeNull();
	expect(dnsBody).toMatchObject({
		enabled: true,
		domain: 'hashi.home.arpa',
		adGuardConnectionId: 'adguard-1',
		agents: [{ pulseAgentId: 'agent-1', enabled: true, ipMode: 'selected' }]
	});
});

test('creates an AdGuard connection targeting a Pulse agent', async ({ page }) => {
	await mockAdmin(page);
	const agent = {
		id: 'agent-1',
		name: 'edge-node-1',
		installType: 'linux_service',
		allowedScopes: [],
		heartbeatIntervalSeconds: 30,
		status: 'online',
		lastSeenAtUtc: new Date().toISOString(),
		lastPublicIp: '198.51.100.10',
		lastPrivateIp: '10.0.0.10',
		lastPrivateIpv4Candidates: ['10.0.0.10'],
		lastPrivateIpv6Candidates: [],
		lastSelectedIp: '10.0.0.10',
		lastSelectedInterface: 'eth0',
		lastHostname: 'edge-node-1',
		lastAgentVersion: '1.0.0',
		dnsPendingAtUtc: null
	};
	await page.route('**/api/pulse/agents', (route) => json(route, [agent]));
	let connections: Record<string, unknown>[] = [];
	let createBody: Record<string, unknown> | null = null;
	await page.route('**/api/adguard/connections', async (route) => {
		if (route.request().method() === 'POST') {
			createBody = route.request().postDataJSON() as Record<string, unknown>;
			const created = { id: 'adguard-1', name: 'home-adguard', ...createBody };
			connections = [created];
			return json(route, created, 201);
		}
		return json(route, connections);
	});
	await page.route('**/api/adguard/connections/adguard-1/rewrites', (route) => json(route, []));

	await page.goto('/adguard');
	await page.getByLabel('Target').selectOption('pulse_agent');
	await page.getByLabel('Pulse agent').selectOption('agent-1');
	await page.getByLabel('Admin password').fill('test-password');
	await page.getByRole('button', { name: 'Add connection' }).click();

	await expect.poll(() => createBody).not.toBeNull();
	expect(createBody).toMatchObject({
		name: 'home-adguard',
		baseUrl: null,
		target: {
			targetMode: 'pulse_agent',
			pulseAgentId: 'agent-1',
			pulseIpMode: 'selected',
			port: 3000,
			scheme: 'http'
		}
	});
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
