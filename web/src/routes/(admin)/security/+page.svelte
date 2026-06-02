<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type {
		SecurityDashboard,
		SecurityRankItem,
		SecurityRecentEventItem,
		SecurityResourceEnforcementItem,
		SecurityTopBlockedIpItem
	} from '$lib/api/types';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import { Lock } from 'lucide-svelte';

	let dashboard = $state<SecurityDashboard | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let hours = $state(24);
	let resourceFilter = $state('');
	let traefikHostFilter = $state('');
	let firewallHostIdFilter = $state('');
	const topCountries = $derived((dashboard?.topCountries ?? []) as SecurityRankItem[]);
	const topAsns = $derived((dashboard?.topAsns ?? []) as SecurityRankItem[]);
	const topBlockedIps = $derived((dashboard?.topBlockedIps ?? []) as SecurityTopBlockedIpItem[]);
	const topResources = $derived(
		(dashboard?.topResourcesBlockedChallenged ?? []) as SecurityResourceEnforcementItem[]
	);
	const recentEvents = $derived((dashboard?.recentEvents ?? []) as SecurityRecentEventItem[]);

	const formatEventTimestamp = (value: string) => new Date(value).toLocaleString();
	const formatExpiry = (value: string | null) => (value ? new Date(value).toLocaleString() : 'No expiry');

	async function loadDashboard() {
		loading = true;
		error = null;
		try {
			dashboard = await api.getSecurityDashboard({
				hours,
				resource: resourceFilter || undefined,
				traefikHost: traefikHostFilter || undefined,
				firewallHostId: firewallHostIdFilter || undefined
			});
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
			<option value={720}>Last 30 days</option>
		</select>
		<label class="text-sm text-muted-foreground" for="security-resource-filter">Resource</label>
		<select
			id="security-resource-filter"
			class="rounded-md border border-border bg-background px-2 py-1 text-sm"
			bind:value={resourceFilter}
			onchange={() => void loadDashboard()}
		>
			<option value="">All resources</option>
			{#if dashboard}
				{#each dashboard.resourceFilters as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			{/if}
		</select>
		<label class="text-sm text-muted-foreground" for="security-traefik-filter">Traefik host</label>
		<select
			id="security-traefik-filter"
			class="rounded-md border border-border bg-background px-2 py-1 text-sm"
			bind:value={traefikHostFilter}
			onchange={() => void loadDashboard()}
		>
			<option value="">All Traefik hosts</option>
			{#if dashboard}
				{#each dashboard.traefikHostFilters as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			{/if}
		</select>
		<label class="text-sm text-muted-foreground" for="security-firewall-filter">Firewall host</label>
		<select
			id="security-firewall-filter"
			class="rounded-md border border-border bg-background px-2 py-1 text-sm"
			bind:value={firewallHostIdFilter}
			onchange={() => void loadDashboard()}
		>
			<option value="">All firewall hosts</option>
			{#if dashboard}
				{#each dashboard.firewallHostFilters as option (option.id)}
					<option value={option.id}>{option.name}</option>
				{/each}
			{/if}
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
			<OverviewWidget title="WAF detections" description="WAF events raised in range.">
				<StatusRow label="Detections" value={String(dashboard.wafDetections)} status="warn" />
				<StatusRow label="Blocked" value={String(dashboard.wafBlocks)} status="error" />
			</OverviewWidget>
			<OverviewWidget title="Firewall active IP blocks" description="Current firewall blocklist entries.">
				<StatusRow label="Blocked IPs" value={String(dashboard.firewallActiveIpBlocks)} status="error" />
			</OverviewWidget>
			<OverviewWidget title="Top blocked IPs" description="Most active blocklist entries.">
				{#if topBlockedIps.length === 0}
					<p class="text-xs text-muted-foreground">None</p>
				{:else}
					{#each topBlockedIps as item (item.ip)}
						<div class="space-y-1 border-b border-border/60 pb-2 last:border-0 last:pb-0">
							<StatusRow label={item.ip} value={String(item.count)} status="error" />
							<p class="text-xs text-muted-foreground">
								{formatEventTimestamp(item.lastSeenAtUtc)}
								{#if item.countryCode}
									Â· {item.countryCode}
								{/if}
								{#if item.asn}
									Â· {item.asn}
								{/if}
							</p>
							<p class="text-xs text-muted-foreground">
								{item.reason ?? 'No reason'} Â· {formatExpiry(item.expiresAtUtc)}
							</p>
						</div>
					{/each}
				{/if}
			</OverviewWidget>
			<OverviewWidget title="Top blocked/challenged resources" description="Most impacted resources in range.">
				{#if topResources.length === 0}
					<p class="text-xs text-muted-foreground">None</p>
				{:else}
					{#each topResources as item (item.resource)}
						<StatusRow
							label={item.resource}
							value={`B:${item.blocked} · C:${item.challenged}`}
							status={item.blocked > item.challenged ? 'error' : 'warn'}
						/>
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
			<OverviewWidget title="Blocklist" description="Active blocked IPs.">
				<StatusRow label="Entries" value={String(dashboard.blocklistCount ?? 0)} status="error" />
			</OverviewWidget>
			<OverviewWidget title="Security events" description="Access, WAF, and forward-auth events.">
				<StatusRow label="Events in range" value={String(dashboard.securityEventCount ?? 0)} />
				{#if recentEvents.length === 0}
					<p class="text-xs text-muted-foreground">No recent events</p>
				{:else}
					{#each recentEvents as event (event.occurredAtUtc + event.category + event.action + (event.clientIp ?? ''))}
						<p class="text-xs text-muted-foreground">
							{formatEventTimestamp(event.occurredAtUtc)} · {event.category}:{event.action}
							{#if event.host}
								· {event.host}
							{/if}
							{#if event.clientIp}
								· {event.clientIp}
							{/if}
						</p>
					{/each}
				{/if}
			</OverviewWidget>
		</div>
	{/if}
</section>
