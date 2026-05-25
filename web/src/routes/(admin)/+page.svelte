<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { AuditEvent } from '$lib/api/types';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import { DEFAULT_WIDGETS, loadWidgetPrefs } from '$lib/overview/widgets';
	import { LayoutDashboard } from 'lucide-svelte';

	let prefs = $state(loadWidgetPrefs());
	let audit = $state<AuditEvent[]>([]);
	let healthVersion = $state('—');

	onMount(async () => {
		try {
			const [events, health] = await Promise.all([
				api.getAuditEvents().catch(() => []),
				api.getHealth().catch(() => null)
			]);
			audit = events.slice(0, 5);
			healthVersion = health?.version ?? '—';
		} catch {
			// offline dev
		}
	});

	const orderedWidgets = $derived(
		DEFAULT_WIDGETS.filter((w) => prefs.enabled[w.id]).sort(
			(a, b) => prefs.order.indexOf(a.id) - prefs.order.indexOf(b.id)
		)
	);
</script>

<section class="mx-auto max-w-7xl space-y-6">
	<PageHeader
		title="Overview"
		description="Homelab edge orchestration at a glance."
		icon={LayoutDashboard}
	/>

	<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
		{#each orderedWidgets as widget (widget.id)}
			<OverviewWidget title={widget.title} description={widget.description}>
				{#if widget.id === 'resource-health'}
					<StatusRow label="Healthy" value="0" />
					<StatusRow label="Degraded" value="0" status="warn" />
					<StatusRow label="Down" value="0" status="error" />
				{:else if widget.id === 'firewall-hosts'}
					<StatusRow label="Hosts online" value="0 / 0" />
					<StatusRow label="Last SSH check" value="—" />
				{:else if widget.id === 'traefik-sync'}
					<StatusRow label="Last sync" value="—" />
					<StatusRow label="Pending changes" value="0" status="neutral" />
				{:else if widget.id === 'dns-sync'}
					<StatusRow label="Last sync" value="—" />
					<StatusRow label="Drift records" value="0" />
				{:else if widget.id === 'incidents'}
					<StatusRow label="Open incidents" value="0" status="ok" />
					<StatusRow label="Last 24h" value="0" />
				{:else if widget.id === 'security-events'}
					<StatusRow label="Active events" value="0" />
					<StatusRow label="Blocked IPs (24h)" value="0" />
				{:else if widget.id === 'pending-sync'}
					<StatusRow label="Queued plans" value="0" />
					<StatusRow label="Awaiting approval" value="0" status="warn" />
				{:else if widget.id === 'cert-expiry'}
					<StatusRow label="Expiring < 14d" value="0" status="ok" />
					<StatusRow label="Expiring < 7d" value="0" />
				{:else if widget.id === 'vault-lock'}
					<StatusRow label="Vault" value="locked" status="warn" />
					<StatusRow label="Passkey" value="—" />
				{:else if widget.id === 'audit'}
					{#if audit.length === 0}
						<StatusRow label="Recent entries" value="None yet" />
					{:else}
						{#each audit as event}
							<StatusRow
								label="{event.category}/{event.action}"
								value={new Date(event.createdAtUtc).toLocaleString()}
							/>
						{/each}
					{/if}
				{/if}
			</OverviewWidget>
		{/each}
	</div>

	<p class="text-[11px] text-muted-foreground">
		Hashi {healthVersion} · widget layout stored locally until settings API ships.
	</p>
</section>
