<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { SecurityDashboard } from '$lib/api/types';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Lock } from 'lucide-svelte';

	let dashboard = $state<SecurityDashboard | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		void (async () => {
			try {
				dashboard = await api.getSecurityDashboard();
			} catch (e) {
				error = e instanceof ApiRequestError ? e.message : 'Failed to load security dashboard';
			} finally {
				loading = false;
			}
		})();
	});
</script>

<section class="mx-auto max-w-7xl space-y-6">
	<PageHeader
		title="Security"
		description="Edge abuse visibility, WAF detections, and active blocks."
		icon={Lock}
	/>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading security metrics…</p>
	{:else if error}
		<p class="text-sm text-destructive">{error}</p>
	{:else if dashboard}
		<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
			<OverviewWidget title="Allowed" description="Requests allowed in range.">
				<StatusRow label="Requests" value={String(dashboard.allowed)} />
			</OverviewWidget>
			<OverviewWidget title="Blocked" description="Requests blocked in range.">
				<StatusRow label="Requests" value={String(dashboard.blocked)} status="error" />
			</OverviewWidget>
			<OverviewWidget title="Challenged" description="Challenged requests in range.">
				<StatusRow label="Requests" value={String(dashboard.challenged)} status="warn" />
			</OverviewWidget>
			<OverviewWidget title="Top blocked IPs" description="Most active blocklist entries.">
				{#if dashboard.topBlockedIps.length === 0}
					<p class="text-xs text-muted-foreground">None</p>
				{:else}
					{#each dashboard.topBlockedIps.slice(0, 5) as ip}
						<StatusRow label={ip} value="blocked" status="error" />
					{/each}
				{/if}
			</OverviewWidget>
		</div>

		<PanelSection title="Extended analytics" description="Country/ASN/resource breakdowns pending richer API fields.">
			<p class="text-sm text-muted-foreground">
				Time-range filters and top-country/ASN widgets will activate when backend expands
				SecurityDashboardResponse.
			</p>
		</PanelSection>
	{/if}
</section>
