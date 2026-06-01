/** @vitest-environment jsdom */
import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import PublicDashboardView from './PublicDashboardView.svelte';

vi.mock('$lib/api/client', () => ({
	api: {
		getPublicApps: vi.fn(async () => ({
			items: [
				{
					id: '00000000-0000-0000-0000-000000000001',
					source: 'resource',
					displayName: 'Public App',
					publicUrl: 'https://public.example.com',
					domain: 'public.example.com',
					status: 'Online',
					lastLatencyMs: 25
				},
				{
					id: '00000000-0000-0000-0000-000000000002',
					source: 'manual_dns',
					displayName: 'Manual DNS',
					publicUrl: 'https://manual.example.com',
					domain: 'manual.example.com',
					status: 'Online',
					lastLatencyMs: null
				}
			],
			hostsOnline: 2,
			totalHosts: 2,
			linuxFirewallHostsAvailable: 1,
			totalLinuxFirewallHosts: 1
		}))
	}
}));

describe('PublicDashboardView', () => {
	it('renders safe public dashboard DTO cards and collapsed search', async () => {
		const { container } = render(PublicDashboardView);

		await waitFor(() => expect(screen.getByText('Public App')).toBeTruthy());

		expect(screen.getByText('Manual DNS')).toBeTruthy();
		expect(screen.getByText('2 / 2 hosts online')).toBeTruthy();
		expect(screen.getByText(/1\s*\/\s*1 Linux firewall hosts available/)).toBeTruthy();
		expect(screen.queryByPlaceholderText('Search services...')).toBeNull();
		expect(container.querySelector('a[href="https://public.example.com"]')).toBeTruthy();
		expect(container.querySelector('a[href="https://manual.example.com"]')).toBeTruthy();
		expect(container.textContent).not.toContain('10.0.0.10:8080');
		expect(container.querySelector('a[href="#"]')).toBeNull();
	});
});
