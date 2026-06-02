/** @vitest-environment jsdom */
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import SettingsPage from './+page.svelte';

const apiMock = vi.hoisted(() => ({
	getGeneralSettings: vi.fn(),
	getDashboardSettings: vi.fn(),
	updateDashboardSettings: vi.fn(),
	listNotificationProviders: vi.fn()
}));

vi.mock('$lib/api/client', () => ({
	ApiRequestError: class ApiRequestError extends Error {
		code?: string;
	},
	api: apiMock
}));

vi.mock('$lib/auth/reauth', () => ({
	performPasskeyReauthentication: vi.fn()
}));

function mockSettingsApi() {
	apiMock.getGeneralSettings.mockResolvedValue({
		rootDomain: 'example.test',
		adminDomain: 'admin.example.test',
		internalUrl: 'http://hashi.test',
		defaultSyncIntervalMinutes: 60,
		publicDashboardEnabled: true,
		publicStatusEnabled: true,
		theme: 'dark'
	});
	apiMock.getDashboardSettings.mockResolvedValue({
		overviewWidgetsJson: JSON.stringify({
			enabled: { 'resource-health': true },
			order: ['resource-health']
		})
	});
	apiMock.updateDashboardSettings.mockResolvedValue({});
	apiMock.listNotificationProviders.mockResolvedValue([]);
}

describe('settings widget preferences', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		localStorage.clear();
		mockSettingsApi();
	});

	it('saves overview widget changes to dashboard settings and the local cache', async () => {
		render(SettingsPage);

		const widgetTitle = await screen.findByText('Resource health');
		await waitFor(() =>
			expect(localStorage.getItem('hashi.overview.widgets')).toContain('"resource-health":true')
		);
		const checkbox = widgetTitle.closest('li')?.querySelector('[role="checkbox"]');
		expect(checkbox).toBeTruthy();

		await fireEvent.click(checkbox as Element);

		await waitFor(() => expect(apiMock.updateDashboardSettings).toHaveBeenCalledTimes(1));
		const request = apiMock.updateDashboardSettings.mock.calls[0][0] as {
			overviewWidgetsJson: string;
		};
		const savedPrefs = JSON.parse(request.overviewWidgetsJson) as {
			enabled: Record<string, boolean>;
		};

		expect(savedPrefs.enabled['resource-health']).toBe(false);
		expect(localStorage.getItem('hashi.overview.widgets')).toContain('"resource-health":false');
		expect(screen.getByText('Widget preferences saved.')).toBeTruthy();
	});
});
