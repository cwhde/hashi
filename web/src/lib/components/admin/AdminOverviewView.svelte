<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { AuditEvent, VaultStatus, HealthResponse } from '$lib/api/types';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import { DEFAULT_WIDGETS, loadDashboardWidgetPrefs, loadWidgetPrefs } from '$lib/overview/widgets';
	import { LayoutDashboard } from 'lucide-svelte';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';

	let prefs = $state(loadWidgetPrefs());
	let audit = $state<AuditEvent[]>([]);
	let healthVersion = $state('—');
	let healthStatus = $state<HealthResponse | null>(null);
	let vaultStatus = $state<VaultStatus | null>(null);
	let resourceCounts = $state({ total: 0, enabled: 0 });
	let statusCounts = $state({ up: 0, degraded: 0, down: 0 });
	let securityBlocked = $state('—');
	let securityAllowed = $state('—');
	let securityChallenged = $state('—');
	let dnsConnections = $state(0);
	let pulseAgents = $state(0);
	let syncRuns = $state<{ pending: number; recent: number }>({ pending: 0, recent: 0 });

	onMount(async () => {
		try {
			const [dashboard, data] = await Promise.all([
				api.getDashboardSettings().catch(() => null),
				api.getAdminDashboard()
			]);
			prefs = loadDashboardWidgetPrefs(dashboard);
			audit = data.auditEvents.slice(0, 5);
			healthVersion = data.health?.version ?? '—';
			healthStatus = data.health;
			vaultStatus = data.vault;
			resourceCounts = {
				total: data.resources.length,
				enabled: data.resources.filter((r) => r.enabled).length
			};
			statusCounts = {
				up: data.monitors.filter((m) => m.status === 'Up').length,
				degraded: data.monitors.filter((m) => m.status === 'Degraded').length,
				down: data.monitors.filter((m) => m.status === 'Down').length
			};
			securityBlocked = data.security ? String(data.security.blocked) : '—';
			securityAllowed = data.security ? String(data.security.allowed) : '—';
			securityChallenged = data.security ? String(data.security.challenged) : '—';
			dnsConnections = data.dnsConnections.length;
			pulseAgents = data.pulseAgents.length;
			syncRuns = {
				recent: data.syncRuns.length,
				pending: data.syncRuns.filter((r) => r.status === 'awaiting_confirmation').length
			};
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

	{#if healthStatus?.providerSyncPaused}
		<Alert variant="destructive" class="border-destructive bg-destructive/15 text-destructive-foreground">
			<AlertTitle>Critical Health Warning: background synchronization is paused</AlertTitle>
			<AlertDescription class="text-xs">
				The service-sync vault is locked or unavailable. All provider synchronization jobs are currently paused. Please unlock the vault or check the service-sync vault configuration to resume sync.
			</AlertDescription>
		</Alert>
	{/if}

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
					<StatusRow label="Allowed (24h)" value={securityAllowed} status="ok" />
					<StatusRow label="Blocked (24h)" value={securityBlocked} status="error" />
					<StatusRow label="Challenged (24h)" value={securityChallenged} status="warn" />
				{:else if widget.id === 'pending-sync'}
					<StatusRow label="Recent runs" value={String(syncRuns.recent)} />
					<StatusRow
						label="Awaiting approval"
						value={String(syncRuns.pending)}
						status={syncRuns.pending > 0 ? 'warn' : 'ok'}
					/>
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
						{#each audit as event (event.id)}
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
		Hashi {healthVersion}
	</p>
</section>
