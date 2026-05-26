<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { MonitorEndpoint } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { HeartPulse } from 'lucide-svelte';

	let endpoints = $state<MonitorEndpoint[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		void (async () => {
			try {
				endpoints = await api.listStatusEndpoints();
			} catch (e) {
				error = e instanceof ApiRequestError ? e.message : 'Failed to load monitors';
			} finally {
				loading = false;
			}
		})();
	});

	const counts = $derived({
		up: endpoints.filter((e) => e.status === 'Up').length,
		degraded: endpoints.filter((e) => e.status === 'Degraded').length,
		down: endpoints.filter((e) => e.status === 'Down').length
	});
</script>

<AdminSectionPage
	title="Status"
	description="Monitors, incidents, latency charts, and public status page config."
	icon={HeartPulse}
>
	<div class="grid gap-4 sm:grid-cols-3">
		<PanelSection title="Up" description="Healthy endpoints.">
			<StatusRow label="Count" value={String(counts.up)} status="ok" />
		</PanelSection>
		<PanelSection title="Degraded" description="Partial failures.">
			<StatusRow label="Count" value={String(counts.degraded)} status="warn" />
		</PanelSection>
		<PanelSection title="Down" description="Failed checks.">
			<StatusRow label="Count" value={String(counts.down)} status="error" />
		</PanelSection>
	</div>

	<PanelSection title="Monitored endpoints" description="Latency charts require time-series API (pending).">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if endpoints.length === 0}
			<p class="text-sm text-muted-foreground">No monitor endpoints configured.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>URL</TableHead>
						<TableHead>Type</TableHead>
						<TableHead>Status</TableHead>
						<TableHead>Latency</TableHead>
						<TableHead>Last check</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each endpoints as endpoint}
						<TableRow>
							<TableCell>{endpoint.name}</TableCell>
							<TableCell class="max-w-[12rem] truncate font-mono text-xs">{endpoint.url}</TableCell>
							<TableCell>{endpoint.checkType}</TableCell>
							<TableCell>{endpoint.status}</TableCell>
							<TableCell>{endpoint.lastLatencyMs ?? '—'} ms</TableCell>
							<TableCell class="text-xs">
								{endpoint.lastCheckedAtUtc
									? new Date(endpoint.lastCheckedAtUtc).toLocaleString()
									: '—'}
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>
</AdminSectionPage>
