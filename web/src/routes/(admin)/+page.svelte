<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { AuditEvent, VaultStatus } from '$lib/api/types';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import { DEFAULT_WIDGETS, loadWidgetPrefs } from '$lib/overview/widgets';
	import { LayoutDashboard } from 'lucide-svelte';

	let prefs = $state(loadWidgetPrefs());
	let audit = $state<AuditEvent[]>([]);
	let healthVersion = $state('—');
	let vaultStatus = $state<VaultStatus | null>(null);
	let resourceCounts = $state({ total: 0, enabled: 0 });
	let statusCounts = $state({ up: 0, degraded: 0, down: 0 });
	let securityBlocked = $state('—');
	let dnsConnections = $state(0);
	let pulseAgents = $state(0);

	onMount(async () => {
		try {
			const [events, health, vault, resources, monitors, security, dns, pulse] =
				await Promise.all([
					api.getAuditEvents().catch(() => []),
					api.getHealth().catch(() => null),
					api.getVaultStatus().catch(() => null),
					api.listResources().catch(() => []),
					api.listStatusEndpoints().catch(() => []),
					api.getSecurityDashboard().catch(() => null),
					api.listDnsConnections().catch(() => []),
					api.listPulseAgents().catch(() => [])
				]);
			audit = events.slice(0, 5);
			healthVersion = health?.version ?? '—';
			vaultStatus = vault;
			resourceCounts = {
				total: resources.length,
				enabled: resources.filter((r) => r.enabled).length
			};
			statusCounts = {
				up: monitors.filter((m) => m.status === 'Up').length,
				degraded: monitors.filter((m) => m.status === 'Degraded').length,
				down: monitors.filter((m) => m.status === 'Down').length
			};
			securityBlocked = security ? String(security.blocked) : '—';
			dnsConnections = dns.length;
			pulseAgents = pulse.length;
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
					<StatusRow label="Total" value={String(resourceCounts.total)} />
					<StatusRow label="Enabled" value={String(resourceCounts.enabled)} status="ok" />
					<StatusRow
						label="Disabled"
						value={String(resourceCounts.total - resourceCounts.enabled)}
						status="neutral"
					/>
				{:else if widget.id === 'firewall-hosts'}
					<StatusRow label="Pulse agents" value={String(pulseAgents)} />
					<StatusRow label="Firewall render" value="via /firewall-hosts" />
				{:else if widget.id === 'traefik-sync'}
					<StatusRow label="Config render" value="via /traefik" />
					<StatusRow label="Pending changes" value="—" status="neutral" />
				{:else if widget.id === 'dns-sync'}
					<StatusRow label="Connections" value={String(dnsConnections)} />
					<StatusRow label="Drift records" value="—" />
				{:else if widget.id === 'incidents'}
					<StatusRow label="Monitors up" value={String(statusCounts.up)} status="ok" />
					<StatusRow label="Monitors down" value={String(statusCounts.down)} status="error" />
				{:else if widget.id === 'security-events'}
					<StatusRow label="Blocked (range)" value={securityBlocked} status="error" />
					<StatusRow label="Dashboard" value="via /security" />
				{:else if widget.id === 'pending-sync'}
					<StatusRow label="Queued plans" value="—" />
					<StatusRow label="Awaiting approval" value="—" status="warn" />
				{:else if widget.id === 'cert-expiry'}
					<StatusRow label="Expiring < 14d" value="—" status="ok" />
					<StatusRow label="Expiring < 7d" value="—" />
				{:else if widget.id === 'vault-lock'}
					<StatusRow
						label="Vault"
						value={vaultStatus?.lockState ?? '—'}
						status={vaultStatus?.lockState === 'Unlocked' ? 'ok' : 'warn'}
					/>
					<StatusRow label="Passkey" value={vaultStatus?.hasPasskey ? 'registered' : 'none'} />
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
