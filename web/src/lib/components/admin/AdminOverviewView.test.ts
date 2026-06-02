/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import AdminOverviewView from './AdminOverviewView.svelte';

const apiMock = vi.hoisted(() => ({
	getDashboardSettings: vi.fn(),
	getAuditEvents: vi.fn(),
	getHealth: vi.fn(),
	getVaultStatus: vi.fn(),
	listResources: vi.fn(),
	listStatusEndpoints: vi.fn(),
	getSecurityDashboard: vi.fn(),
	listDnsConnections: vi.fn(),
	listPulseAgents: vi.fn(),
	listSyncRuns: vi.fn()
}));

vi.mock('$lib/api/client', () => ({
	api: apiMock
}));

function mockOverviewApi() {
	apiMock.getDashboardSettings.mockResolvedValue({
		overviewWidgetsJson: JSON.stringify({
			enabled: { 'dns-sync': false, audit: false },
			order: ['dns-sync', 'resource-health']
		})
	});
	apiMock.getAuditEvents.mockResolvedValue([]);
	apiMock.getHealth.mockResolvedValue({ version: 'test-version' });
	apiMock.getVaultStatus.mockResolvedValue({ lockState: 'Unlocked', hasPasskey: true });
	apiMock.listResources.mockResolvedValue([{ enabled: true }, { enabled: false }]);
	apiMock.listStatusEndpoints.mockResolvedValue([{ status: 'Up' }, { status: 'Down' }]);
	apiMock.getSecurityDashboard.mockResolvedValue({ allowed: 9, blocked: 1, challenged: 2 });
	apiMock.listDnsConnections.mockResolvedValue([{ id: 'dns-1' }]);
	apiMock.listPulseAgents.mockResolvedValue([{ id: 'pulse-1' }]);
	apiMock.listSyncRuns.mockResolvedValue([
		{ status: 'awaiting_confirmation' },
		{ status: 'completed' }
	]);
}

describe('AdminOverviewView', () => {
	afterEach(() => {
		cleanup();
	});

	beforeEach(() => {
		vi.clearAllMocks();
		localStorage.clear();
		mockOverviewApi();
	});

	it('loads overview widget preferences from dashboard settings and updates the cache', async () => {
		localStorage.setItem(
			'hashi.overview.widgets',
			JSON.stringify({ enabled: { 'resource-health': false, 'dns-sync': true } })
		);

		render(AdminOverviewView);

		await waitFor(() => expect(apiMock.getDashboardSettings).toHaveBeenCalledTimes(1));
		await waitFor(() => expect(screen.queryByText('DNS sync')).toBeNull());

		expect(screen.getByText('Resource health')).toBeTruthy();
		expect(screen.queryByText('Recent audit')).toBeNull();
		expect(localStorage.getItem('hashi.overview.widgets')).toContain('"dns-sync":false');
		expect(screen.getByText('Hashi test-version')).toBeTruthy();
		expect(screen.queryByText(/stored locally until settings API ships/)).toBeNull();
	});

	it('uses the local widget cache when dashboard settings cannot be loaded', async () => {
		apiMock.getDashboardSettings.mockRejectedValue(new Error('offline'));
		localStorage.setItem(
			'hashi.overview.widgets',
			JSON.stringify({ enabled: { 'resource-health': false, 'security-events': true } })
		);

		render(AdminOverviewView);

		await waitFor(() => expect(apiMock.getDashboardSettings).toHaveBeenCalledTimes(1));
		await waitFor(() => expect(screen.queryByText('Resource health')).toBeNull());

		expect(screen.getByText('Security events')).toBeTruthy();
	});
});
