/** @vitest-environment jsdom */
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import StatusPage from './+page.svelte';

const apiMock = vi.hoisted(() => ({
	listStatusEndpoints: vi.fn(),
	listStatusRollups: vi.fn(),
	listStatusEvents: vi.fn(),
	updateStatusEndpoint: vi.fn()
}));

vi.mock('$lib/api/client', () => ({
	ApiRequestError: class ApiRequestError extends Error {
		status = 500;
	},
	api: apiMock
}));

vi.mock('$lib/components/monitoring/MonitorLatencyChart.svelte', () => ({
	default: vi.fn()
}));

const endpoints = [
	{
		id: 'endpoint-a',
		name: 'Alpha app',
		url: 'https://alpha.example.test/',
		checkType: 'https',
		enabled: true,
		publicStatusEnabled: true,
		status: 'Up',
		lastCheckedAtUtc: '2026-06-04T08:00:00Z',
		lastLatencyMs: 42,
		resourceId: 'resource-a',
		resourceType: 'web',
		host: 'alpha.example.test',
		firewallHostId: 'firewall-a',
		firewallHostName: 'edge-a',
		provisioned: true
	},
	{
		id: 'endpoint-b',
		name: 'Beta api',
		url: 'https://beta.example.test/',
		checkType: 'https',
		enabled: true,
		publicStatusEnabled: false,
		status: 'Down',
		lastCheckedAtUtc: '2026-06-04T08:01:00Z',
		lastLatencyMs: 700,
		resourceId: 'resource-b',
		resourceType: 'api',
		host: 'beta.example.test',
		firewallHostId: null,
		firewallHostName: null,
		provisioned: true
	}
];

const rollups = [
	{
		monitorEndpointId: 'endpoint-a',
		bucketStartUtc: '2026-06-04T07:00:00Z',
		intervalMinutes: 1,
		sampleCount: 2,
		upCount: 2,
		downCount: 0,
		averageLatencyMs: 40
	},
	{
		monitorEndpointId: 'endpoint-b',
		bucketStartUtc: '2026-06-04T07:00:00Z',
		intervalMinutes: 1,
		sampleCount: 2,
		upCount: 0,
		downCount: 2,
		averageLatencyMs: 700
	}
];

const events = [
	{
		id: 'event-a',
		monitorEndpointId: 'endpoint-a',
		previousStatus: 'Down',
		newStatus: 'Up',
		latencyMs: 42,
		occurredAtUtc: '2026-06-04T07:59:00Z'
	}
];

describe('status page operations view', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		apiMock.listStatusEndpoints.mockResolvedValue(endpoints);
		apiMock.listStatusRollups.mockResolvedValue(rollups);
		apiMock.listStatusEvents.mockResolvedValue(events);
		apiMock.updateStatusEndpoint.mockResolvedValue({ ...endpoints[0], publicStatusEnabled: false });
	});

	it('loads 30-day status data, groups, sorts, renders timeline, and updates endpoint settings', async () => {
		render(StatusPage);

		expect(await screen.findByText('Alpha app')).toBeTruthy();
		expect(screen.getByText('Last 30 days')).toBeTruthy();
		expect(screen.getByText('Event timeline')).toBeTruthy();
		expect(screen.getByText('Down -> Up')).toBeTruthy();
		expect(screen.getByText('Endpoint settings')).toBeTruthy();
		expect(screen.getAllByText('edge-a').length).toBeGreaterThan(0);

		await fireEvent.change(screen.getByLabelText('Range'), { target: { value: '720' } });
		await waitFor(() =>
			expect(apiMock.listStatusRollups).toHaveBeenLastCalledWith({
				intervalMinutes: 1,
				hours: 1
			})
		);
		expect(apiMock.listStatusRollups).toHaveBeenCalledWith({
			intervalMinutes: 60,
			hours: 720
		});
		expect(apiMock.listStatusEvents).toHaveBeenCalledWith({ hours: 720 });

		await fireEvent.change(screen.getByLabelText('Group'), { target: { value: 'firewallHost' } });
		expect(await screen.findByText('No Linux firewall host')).toBeTruthy();

		await fireEvent.change(screen.getByLabelText('Sort'), { target: { value: 'lastEvent' } });
		expect(screen.getByText('Last event')).toBeTruthy();

		const publicToggles = screen.getAllByRole('switch');
		await fireEvent.click(publicToggles[publicToggles.length - 1]);
		await waitFor(() =>
			expect(apiMock.updateStatusEndpoint).toHaveBeenCalledWith('endpoint-a', {
				publicStatusEnabled: false
			})
		);
	});
});
