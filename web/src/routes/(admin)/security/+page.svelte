<script lang="ts">
	import { api, ApiRequestError, ensureCsrfToken } from '$lib/api/client';
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
	import { Eye, Lock, Plus, Power, PowerOff, RefreshCw, Trash2 } from 'lucide-svelte';

	type BlocklistSource = {
		id: string;
		name: string;
		sourceUrl: string;
		description: string;
		format: string;
		enforcementMode: string;
		canFirewallEnforce: boolean;
		enabled: boolean;
		allowHttp: boolean;
		refreshIntervalHours: number;
		lastFetchStatus: string;
		lastFetchError: string | null;
		lastFetchedAtUtc: string | null;
		entryCount: number;
		isStale: boolean;
		metadataJson: string | null;
	};

	type BlocklistPreview = {
		sourceId: string;
		sourceName: string;
		parsedCount: number;
		ignoredCount: number;
		errorCount: number;
		notModified: boolean;
		entries: { subjectType: string; value: string; normalizedValue: string; lineNumber: number | null }[];
		errors: string[];
		warnings: string[];
	};

	type BlocklistRun = {
		id: string;
		startedAtUtc: string;
		completedAtUtc: string | null;
		status: string;
		entryCount: number;
		addedCount: number;
		removedCount: number;
		unchangedCount: number;
		error: string | null;
	};

	let dashboard = $state<SecurityDashboard | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let hours = $state(24);
	let resourceFilter = $state('');
	let traefikHostFilter = $state('');
	let firewallHostIdFilter = $state('');
	let blocklists = $state<BlocklistSource[]>([]);
	let blocklistsLoading = $state(true);
	let blocklistError = $state<string | null>(null);
	let preview = $state<BlocklistPreview | null>(null);
	let previewLoadingId = $state<string | null>(null);
	let runs = $state<BlocklistRun[]>([]);
	let selectedSourceId = $state<string | null>(null);
	let customName = $state('');
	let customUrl = $state('');
	let customFormat = $state('text');
	let customEnforcement = $state('middleware');
	let customAllowHttp = $state(false);
	let customCanFirewall = $state(true);
	let customColumn = $state('');
	let customJsonField = $state('');
	const topCountries = $derived((dashboard?.topCountries ?? []) as SecurityRankItem[]);
	const topAsns = $derived((dashboard?.topAsns ?? []) as SecurityRankItem[]);
	const topBlockedIps = $derived((dashboard?.topBlockedIps ?? []) as SecurityTopBlockedIpItem[]);
	const topResources = $derived(
		(dashboard?.topResourcesBlockedChallenged ?? []) as SecurityResourceEnforcementItem[]
	);
	const recentEvents = $derived((dashboard?.recentEvents ?? []) as SecurityRecentEventItem[]);
	const selectedSource = $derived(blocklists.find((source) => source.id === selectedSourceId) ?? null);

	const formatEventTimestamp = (value: string) => new Date(value).toLocaleString();
	const formatExpiry = (value: string | null) => (value ? new Date(value).toLocaleString() : 'No expiry');
	const formatMaybeDate = (value: string | null) => (value ? new Date(value).toLocaleString() : 'Never');
	const metadataWarning = (source: BlocklistSource) => {
		try {
			const metadata = JSON.parse(source.metadataJson ?? '{}') as { falsePositiveWarning?: string };
			return metadata.falsePositiveWarning ?? 'Third-party blocklists can create false positives.';
		} catch {
			return 'Third-party blocklists can create false positives.';
		}
	};

	async function securityJson<T>(path: string, init: RequestInit = {}): Promise<T> {
		const method = (init.method ?? 'GET').toUpperCase();
		const headers = new Headers(init.headers);
		if (method !== 'GET' && method !== 'HEAD') {
			headers.set('Content-Type', 'application/json');
			const token = await ensureCsrfToken();
			if (token) headers.set('X-CSRF-TOKEN', token);
		}

		const response = await fetch(path, { ...init, headers, credentials: 'include' });
		if (!response.ok) {
			let message = 'Request failed';
			try {
				const body = (await response.json()) as { error?: string; message?: string; code?: string };
				message = body.error ?? body.message ?? message;
				if (body.code === 'reauth_required') message = 'Recent reauthentication required for this operation.';
			} catch {
				// Keep generic message for non-JSON responses.
			}
			throw new Error(message);
		}

		if (response.status === 204) return undefined as T;
		return (await response.json()) as T;
	}

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

	async function loadBlocklists() {
		blocklistsLoading = true;
		blocklistError = null;
		try {
			blocklists = await securityJson<BlocklistSource[]>('/api/security/blocklists');
			if (!selectedSourceId && blocklists.length > 0) selectedSourceId = blocklists[0].id;
		} catch (e) {
			blocklistError = e instanceof Error ? e.message : 'Failed to load blocklists';
		} finally {
			blocklistsLoading = false;
		}
	}

	async function createCustomSource() {
		blocklistError = null;
		try {
			await securityJson<BlocklistSource>('/api/security/blocklists', {
				method: 'POST',
				body: JSON.stringify({
					name: customName || 'Custom blocklist',
					sourceUrl: customUrl,
					description: 'Custom URL blocklist',
					format: customFormat,
					enforcementMode: customEnforcement,
					canFirewallEnforce: customCanFirewall,
					enabled: false,
					allowHttp: customAllowHttp,
					refreshIntervalHours: 24,
					csvColumnIndex: customColumn ? Number(customColumn) : null,
					jsonValueField: customJsonField || null
				})
			});
			customName = '';
			customUrl = '';
			customColumn = '';
			customJsonField = '';
			await loadBlocklists();
		} catch (e) {
			blocklistError = e instanceof Error ? e.message : 'Failed to add source';
		}
	}

	async function previewSource(source: BlocklistSource) {
		previewLoadingId = source.id;
		blocklistError = null;
		try {
			preview = await securityJson<BlocklistPreview>(
				`/api/security/blocklists/${source.id}/fetch-preview`,
				{ method: 'POST' }
			);
			selectedSourceId = source.id;
			await loadRuns(source.id);
		} catch (e) {
			blocklistError = e instanceof Error ? e.message : 'Preview failed';
		} finally {
			previewLoadingId = null;
		}
	}

	async function mutateSource(source: BlocklistSource, action: 'enable' | 'disable' | 'refresh') {
		blocklistError = null;
		try {
			await securityJson(`/api/security/blocklists/${source.id}/${action}`, { method: 'POST' });
			selectedSourceId = source.id;
			await loadBlocklists();
			await loadRuns(source.id);
		} catch (e) {
			blocklistError = e instanceof Error ? e.message : `${action} failed`;
		}
	}

	async function deleteSource(source: BlocklistSource) {
		blocklistError = null;
		try {
			await securityJson(`/api/security/blocklists/${source.id}`, { method: 'DELETE' });
			if (selectedSourceId === source.id) {
				selectedSourceId = null;
				preview = null;
				runs = [];
			}
			await loadBlocklists();
		} catch (e) {
			blocklistError = e instanceof Error ? e.message : 'Delete failed';
		}
	}

	async function loadRuns(sourceId: string) {
		runs = await securityJson<BlocklistRun[]>(`/api/security/blocklists/${sourceId}/runs`);
	}

	$effect(() => {
		void loadDashboard();
	});

	$effect(() => {
		void loadBlocklists();
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
