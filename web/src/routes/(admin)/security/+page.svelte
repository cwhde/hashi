<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { SecurityDashboard, SecurityRankItem } from '$lib/api/types';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import { Lock } from 'lucide-svelte';

	let dashboard = $state<SecurityDashboard | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let hours = $state(24);
	const topCountries = $derived((dashboard?.topCountries ?? []) as SecurityRankItem[]);
	const topAsns = $derived((dashboard?.topAsns ?? []) as SecurityRankItem[]);

	async function loadDashboard() {
		loading = true;
		error = null;
		try {
			dashboard = await api.getSecurityDashboard({ hours });
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load security dashboard';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void loadDashboard();
	});
</script>

<section class="mx-auto max-w-7xl space-y-6">
	<PageHeader
		title="Security"
		description="Edge abuse visibility, WAF detections, and active blocks."
		icon={Lock}
	/>

	<div class="flex flex-wrap items-center gap-3">
		<label class="text-sm text-muted-foreground" for="security-hours">Range</label>
		<select
			id="security-hours"
			class="rounded-md border border-border bg-background px-2 py-1 text-sm"
			bind:value={hours}
			onchange={() => void loadDashboard()}
		>
			<option value={1}>Last hour</option>
			<option value={24}>Last 24 hours</option>
			<option value={168}>Last 7 days</option>
		</select>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading security metrics…</p>
	{:else if error}
		<p class="text-sm text-destructive">{error}</p>
	{:else if dashboard}
		<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
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
					{#each dashboard.topBlockedIps.slice(0, 5) as ip (ip)}
						<StatusRow label={ip} value="blocked" status="error" />
					{/each}
				{/if}
			</OverviewWidget>
			<OverviewWidget title="Top countries" description="Traffic by country code.">
				{#if topCountries.length === 0}
					<p class="text-xs text-muted-foreground">None</p>
				{:else}
					{#each topCountries as item (item.label)}
						<StatusRow label={item.label} value={String(item.count)} />
					{/each}
				{/if}
			</OverviewWidget>
			<OverviewWidget title="Top ASNs" description="Traffic by autonomous system.">
				{#if topAsns.length === 0}
					<p class="text-xs text-muted-foreground">None</p>
				{:else}
					{#each topAsns as item (item.label)}
						<StatusRow label={item.label} value={String(item.count)} />
					{/each}
				{/if}
			</OverviewWidget>
		</div>
	{/if}
</section>
