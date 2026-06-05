<script lang="ts">
	import { api, ApiRequestError, ensureCsrfToken } from '$lib/api/client';
	import type {
		SecurityDashboard,
		SecurityRankItem,
		SecurityRecentEventItem,
		SecurityResourceEnforcementItem,
		SecurityTopBlockedIpItem
	} from '$lib/api/types';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import {
		Ban,
		Clock,
		Eye,
		Filter,
		Lock,
		Plus,
		Power,
		PowerOff,
		RefreshCw,
		Search,
		Shield,
		Trash2
	} from 'lucide-svelte';
	import {
		filterTimelineEvents,
		parseSecuritySubjectQuery,
		type ParsedSecuritySubjectQuery
	} from './subject-tools';

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

	type SubjectSummary = {
		id: string;
		subjectType: string;
		subjectValue: string;
		normalizedValue: string;
		currentState: string;
		firstSeenAtUtc: string;
		lastSeenAtUtc: string;
		lastCountry: string | null;
		lastRegion: string | null;
		lastAsn: string | null;
		lastAsOrg: string | null;
	};

	type ManualEntry = {
		id: string;
		subjectType: string;
		subjectValue: string;
		normalizedValue: string;
		entryType: string;
		scopeType: string;
		scopeId: string | null;
		reason: string | null;
		createdAtUtc: string;
		expiresAtUtc: string | null;
		isPermanent: boolean;
		bypassBlocking: boolean;
		bypassAdaptiveEscalation: boolean;
		bypassRateLimit: boolean;
		bypassChallenge: boolean;
		bypassSso: boolean;
		enabled: boolean;
		lastHitAtUtc: string | null;
	};

	type SubjectState = {
		securitySubjectId: string;
		challengeRequired: boolean;
		challengeReason: string | null;
		requestsWhileChallenged: number;
		softBlockedUntilUtc: string | null;
		firewallBlockedUntilUtc: string | null;
		manualAllowActive: boolean;
		manualBlockActive: boolean;
		lastEscalationReason: string | null;
		lastEscalationAtUtc: string | null;
		updatedAtUtc: string;
	};

	type SubjectDetail = {
		subject: SubjectSummary;
		state: SubjectState | null;
		manualEntries: ManualEntry[];
		blocklistEntries: {
			id: string;
			subjectType: string;
			value: string;
			normalizedValue: string;
			reason: string;
			source: string;
			enabled: boolean;
			enforcementMode: string;
			syncedToFirewall: boolean;
			expiresAtUtc: string | null;
			lastHitAtUtc: string | null;
		}[];
		resourceRules: { id: string; enabled: boolean; priority: number; action: string; matchType: string; matchValue: string }[];
		firewallApplications: {
			firewallHostId: string | null;
			firewallHostName: string | null;
			enforcement: string;
			status: string;
			appliedAtUtc: string | null;
			lastError: string | null;
		}[];
	};

	type TimelineEvent = {
		id: string;
		occurredAtUtc: string;
		resourceId: string | null;
		eventType: string | null;
		severity: string | null;
		decision: string | null;
		source: string | null;
		reason: string | null;
		requestMethod: string | null;
		requestPath: string | null;
		statusCode: number | null;
	};

	type RequestBucket = {
		id: string;
		bucketStartUtc: string;
		resourceId: string | null;
		method: string;
		pathPrefix: string;
		statusClass: number;
		requestCount: number;
		blockedCount: number;
		challengedCount: number;
		challengeIgnoredCount: number;
		failedChallengeCount: number;
	};

	type EffectiveDecision = {
		decision: string;
		action: string;
		reason: string;
		explanation: string[];
		matchedManualEntryIds: string[];
		matchedBlocklistEntryIds: string[];
		matchedResourceRuleIds: string[];
	};

	let dashboard = $state<SecurityDashboard | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let hours = $state(24);
	let resourceFilter = $state('');
	let traefikHostFilter = $state('');
	let firewallHostIdFilter = $state('');

	let query = $state('');
	let parsedQuery = $state<ParsedSecuritySubjectQuery | null>(null);
	let searchResults = $state<SubjectSummary[]>([]);
	let selectedSubjectId = $state<string | null>(null);
	let subjectDetail = $state<SubjectDetail | null>(null);
	let effectiveDecision = $state<EffectiveDecision | null>(null);
	let timeline = $state<TimelineEvent[]>([]);
	let buckets = $state<RequestBucket[]>([]);
	let subjectLoading = $state(false);
	let subjectError = $state<string | null>(null);
	let actionReason = $state('');
	let blockDurationHours = $state(24);
	let timelineTypeFilter = $state('');
	let timelineResourceFilter = $state('');

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
	const filteredTimeline = $derived(
		filterTimelineEvents(timeline, timelineTypeFilter, timelineResourceFilter)
	);
	const activeManualBlock = $derived(
		subjectDetail?.manualEntries.find((entry) => entry.entryType === 'block' && entry.enabled) ?? null
	);
	const activeManualAllow = $derived(
		subjectDetail?.manualEntries.find((entry) => entry.entryType === 'allow' && entry.enabled) ?? null
	);

	const formatDate = (value: string | null) => (value ? new Date(value).toLocaleString() : 'Never');
	const formatExpiry = (value: string | null, permanent = false) =>
		permanent ? 'Permanent' : value ? new Date(value).toLocaleString() : 'No expiry';
	const compact = (value: string) => value.replaceAll('_', ' ');
	const metadataWarning = (source: BlocklistSource) => {
		try {
			const metadata = JSON.parse(source.metadataJson ?? '{}') as { falsePositiveWarning?: string };
			return metadata.falsePositiveWarning ?? 'Third-party blocklists can create false positives.';
		} catch {
			return 'Third-party blocklists can create false positives.';
		}
	};

	type FetchInit = NonNullable<Parameters<typeof fetch>[1]>;

	async function securityJson<T>(path: string, init: FetchInit = {}, retry = true): Promise<T> {
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
			let code: string | null = null;
			try {
				const body = (await response.json()) as { error?: string; message?: string; code?: string };
				message = body.error ?? body.message ?? message;
				code = body.code ?? null;
			} catch {
				// Keep generic message for non-JSON responses.
			}
			if (code === 'reauth_required' && retry) {
				const ok = await performPasskeyReauthentication();
				if (ok) return securityJson<T>(path, init, false);
				message = 'Passkey reauthentication failed.';
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

	async function searchSubjects() {
		subjectError = null;
		parsedQuery = parseSecuritySubjectQuery(query);
		try {
			const response = await securityJson<{ results: SubjectSummary[] }>(
				`/api/security/subjects/search?q=${encodeURIComponent(query)}`
			);
			searchResults = response.results;
			if (searchResults.length > 0) await selectSubject(searchResults[0].id);
		} catch (e) {
			subjectError = e instanceof Error ? e.message : 'Search failed';
		}
	}

	async function selectSubject(id: string) {
		selectedSubjectId = id;
		subjectLoading = true;
		subjectError = null;
		try {
			const [detail, decision, events, bucketRows] = await Promise.all([
				securityJson<SubjectDetail>(`/api/security/subjects/${id}`),
				securityJson<EffectiveDecision>(`/api/security/subjects/${id}/effective-decision`),
				securityJson<TimelineEvent[]>(`/api/security/subjects/${id}/events`),
				securityJson<RequestBucket[]>(`/api/security/subjects/${id}/buckets?hours=${hours}`)
			]);
			subjectDetail = detail;
			effectiveDecision = decision;
			timeline = events;
			buckets = bucketRows;
		} catch (e) {
			subjectError = e instanceof Error ? e.message : 'Failed to load subject';
		} finally {
			subjectLoading = false;
		}
	}

	async function createManualEntry(entryType: 'allow' | 'block', firewallEnforced = false) {
		if (!subjectDetail) return;
		const subject = subjectDetail.subject;
		subjectError = null;
		const expiresAtUtc =
			entryType === 'block'
				? new Date(Date.now() + blockDurationHours * 60 * 60 * 1000).toISOString()
				: null;
		try {
			if (entryType === 'block') {
				await securityJson('/api/security/blocks', {
					method: 'POST',
					body: JSON.stringify({
						subjectType: subject.subjectType,
						subjectValue: subject.normalizedValue,
						blockType: firewallEnforced ? 'firewall' : 'soft',
						reason: actionReason || null,
						expiresAtUtc,
						isPermanent: false,
						firewallEnforced
					})
				});
			} else {
				await securityJson('/api/security/manual-entries', {
					method: 'POST',
					body: JSON.stringify({
						subjectType: subject.subjectType,
						subjectValue: subject.normalizedValue,
						entryType,
						scopeType: 'global',
						scopeId: null,
						reason: actionReason || null,
						expiresAtUtc: null,
						isPermanent: true,
						enabled: true
					})
				});
			}
			actionReason = '';
			await selectSubject(subject.id);
			await loadDashboard();
		} catch (e) {
			subjectError = e instanceof Error ? e.message : 'Manual action failed';
		}
	}

	async function mutateBlock(action: 'extend' | 'shorten' | 'make-permanent' | 'expire') {
		if (!activeManualBlock || !subjectDetail) return;
		const body =
			action === 'extend' || action === 'shorten'
				? JSON.stringify({ durationSeconds: blockDurationHours * 60 * 60 })
				: '{}';
		try {
			await securityJson(`/api/security/blocks/${activeManualBlock.id}/${action}`, {
				method: 'POST',
				body
			});
			await selectSubject(subjectDetail.subject.id);
			await loadDashboard();
		} catch (e) {
			subjectError = e instanceof Error ? e.message : `${action} failed`;
		}
	}

	async function removeManualEntry(entry: ManualEntry) {
		if (!subjectDetail) return;
		try {
			await securityJson(`/api/security/manual-entries/${entry.id}/expire`, { method: 'POST' });
			await selectSubject(subjectDetail.subject.id);
			await loadDashboard();
		} catch (e) {
			subjectError = e instanceof Error ? e.message : 'Expire failed';
		}
	}

	async function previewFirewallSync() {
		if (!activeManualBlock || !subjectDetail) return;
		try {
			await securityJson(`/api/security/blocks/${activeManualBlock.id}/preview-firewall-sync`, {
				method: 'POST'
			});
			await selectSubject(subjectDetail.subject.id);
		} catch (e) {
			subjectError = e instanceof Error ? e.message : 'Firewall preview failed';
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
		description="Edge abuse visibility, incident response, and blocklist operations."
		icon={Lock}
	/>

	<div class="flex flex-wrap items-center gap-3">
		<label class="text-sm text-muted-foreground" for="security-hours">Range</label>
		<select id="security-hours" class="rounded-md border border-border bg-background px-2 py-1 text-sm" bind:value={hours} onchange={() => void loadDashboard()}>
			<option value={1}>Last hour</option>
			<option value={24}>Last 24 hours</option>
			<option value={168}>Last 7 days</option>
			<option value={720}>Last 30 days</option>
		</select>
		<label class="text-sm text-muted-foreground" for="security-resource-filter">Resource</label>
		<select id="security-resource-filter" class="rounded-md border border-border bg-background px-2 py-1 text-sm" bind:value={resourceFilter} onchange={() => void loadDashboard()}>
			<option value="">All resources</option>
			{#if dashboard}
				{#each dashboard.resourceFilters as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			{/if}
		</select>
		<label class="text-sm text-muted-foreground" for="security-traefik-filter">Traefik host</label>
		<select id="security-traefik-filter" class="rounded-md border border-border bg-background px-2 py-1 text-sm" bind:value={traefikHostFilter} onchange={() => void loadDashboard()}>
			<option value="">All Traefik hosts</option>
			{#if dashboard}
				{#each dashboard.traefikHostFilters as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			{/if}
		</select>
		<label class="text-sm text-muted-foreground" for="security-firewall-filter">Firewall host</label>
		<select id="security-firewall-filter" class="rounded-md border border-border bg-background px-2 py-1 text-sm" bind:value={firewallHostIdFilter} onchange={() => void loadDashboard()}>
			<option value="">All firewall hosts</option>
			{#if dashboard}
				{#each dashboard.firewallHostFilters as option (option.id)}
					<option value={option.id}>{option.name}</option>
				{/each}
			{/if}
		</select>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading security metrics...</p>
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
			<OverviewWidget title="WAF" description="Detections and blocks.">
				<StatusRow label="Detections" value={String(dashboard.wafDetections)} status="warn" />
				<StatusRow label="Blocked" value={String(dashboard.wafBlocks)} status="error" />
			</OverviewWidget>
			<OverviewWidget title="Active blocks" description="Soft and firewall state.">
				<StatusRow label="Firewall IP blocks" value={String(dashboard.firewallActiveIpBlocks)} status="error" />
				<StatusRow label="Blocklist entries" value={String(dashboard.blocklistCount ?? 0)} status="warn" />
			</OverviewWidget>
			<OverviewWidget title="Top blocked IPs" description="Most active blocked subjects.">
				{#if topBlockedIps.length === 0}
					<p class="text-xs text-muted-foreground">None</p>
				{:else}
					{#each topBlockedIps.slice(0, 4) as item (item.ip)}
						<StatusRow label={item.ip} value={String(item.count)} status="error" />
					{/each}
				{/if}
			</OverviewWidget>
			<OverviewWidget title="Top challenged/blocked resources" description="Most impacted resources.">
				{#if topResources.length === 0}
					<p class="text-xs text-muted-foreground">None</p>
				{:else}
					{#each topResources.slice(0, 4) as item (item.resource)}
						<StatusRow label={item.resource} value={`B:${item.blocked} C:${item.challenged}`} status={item.blocked > item.challenged ? 'error' : 'warn'} />
					{/each}
				{/if}
			</OverviewWidget>
			<OverviewWidget title="Signals" description="Top origin summaries.">
				<StatusRow label="Countries" value={topCountries.slice(0, 2).map((x) => x.label).join(', ') || 'None'} />
				<StatusRow label="ASNs" value={topAsns.slice(0, 2).map((x) => x.label).join(', ') || 'None'} />
			</OverviewWidget>
		</div>
	{/if}

	<section class="space-y-4 border-y border-border py-5">
		<div class="flex flex-wrap items-end gap-3">
			<div class="min-w-72 flex-1 space-y-1">
				<label class="text-sm font-medium" for="subject-search">Subject search</label>
				<div class="flex gap-2">
					<input id="subject-search" class="h-9 flex-1 rounded-md border border-border bg-background px-3 text-sm" bind:value={query} placeholder="IP, CIDR, ASN, country, region, or event text" onkeydown={(event) => event.key === 'Enter' && void searchSubjects()} />
					<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" onclick={() => void searchSubjects()}>
						<Search class="size-4" /> Search
					</button>
				</div>
				{#if parsedQuery}
					<p class="text-xs text-muted-foreground">Parsed as {parsedQuery.type}: {parsedQuery.value}</p>
				{/if}
			</div>
			<div class="space-y-1">
				<label class="text-sm font-medium" for="block-duration">Block duration</label>
				<input id="block-duration" class="h-9 w-28 rounded-md border border-border bg-background px-3 text-sm" type="number" min="1" bind:value={blockDurationHours} />
			</div>
			<div class="min-w-64 flex-1 space-y-1">
				<label class="text-sm font-medium" for="action-reason">Action reason</label>
				<input id="action-reason" class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm" bind:value={actionReason} placeholder="Audit reason" />
			</div>
		</div>

		{#if subjectError}
			<p class="text-sm text-destructive">{subjectError}</p>
		{/if}

		<div class="grid gap-4 lg:grid-cols-[minmax(220px,0.8fr)_minmax(0,2fr)]">
			<div class="space-y-2">
				{#if searchResults.length === 0}
					<p class="text-sm text-muted-foreground">No subject selected.</p>
				{:else}
					{#each searchResults as result (result.id)}
						<button class={`w-full rounded-md border px-3 py-2 text-left text-sm ${selectedSubjectId === result.id ? 'border-primary bg-primary/5' : 'border-border'}`} onclick={() => void selectSubject(result.id)}>
							<span class="block font-medium">{result.normalizedValue}</span>
							<span class="text-xs text-muted-foreground">{result.subjectType} / {compact(result.currentState)} / last {formatDate(result.lastSeenAtUtc)}</span>
						</button>
					{/each}
				{/if}
			</div>

			<div class="space-y-4">
				{#if subjectLoading}
					<p class="text-sm text-muted-foreground">Loading subject...</p>
				{:else if subjectDetail}
					<div class="grid gap-3 md:grid-cols-3">
						<div class="rounded-md border border-border p-3">
							<p class="text-xs text-muted-foreground">Effective decision</p>
							<p class="text-lg font-semibold capitalize">{effectiveDecision?.decision ?? 'unknown'}</p>
							<p class="text-xs text-muted-foreground">{compact(effectiveDecision?.action ?? 'pending')} / {compact(effectiveDecision?.reason ?? 'pending')}</p>
						</div>
						<div class="rounded-md border border-border p-3">
							<p class="text-xs text-muted-foreground">Active state</p>
							<p class="text-sm">Allow: {subjectDetail.state?.manualAllowActive ? 'yes' : 'no'} / Block: {subjectDetail.state?.manualBlockActive ? 'yes' : 'no'}</p>
							<p class="text-xs text-muted-foreground">Challenge: {subjectDetail.state?.challengeRequired ? subjectDetail.state.challengeReason ?? 'required' : 'none'}</p>
						</div>
						<div class="rounded-md border border-border p-3">
							<p class="text-xs text-muted-foreground">Identity</p>
							<p class="text-sm">{subjectDetail.subject.normalizedValue}</p>
							<p class="text-xs text-muted-foreground">{subjectDetail.subject.lastCountry ?? 'country ?'} / {subjectDetail.subject.lastAsn ?? 'ASN ?'}</p>
						</div>
					</div>

					<div class="flex flex-wrap gap-2">
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!!activeManualAllow} onclick={() => void createManualEntry('allow')}>
							<Shield class="size-4" /> Allow
						</button>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!!activeManualBlock} onclick={() => void createManualEntry('block')}>
							<Ban class="size-4" /> Soft block
						</button>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!!activeManualBlock || !['ip', 'cidr'].includes(subjectDetail.subject.subjectType)} onclick={() => void createManualEntry('block', true)}>
							<Lock class="size-4" /> Firewall block
						</button>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!activeManualBlock} onclick={() => void mutateBlock('extend')}>
							<Clock class="size-4" /> Extend
						</button>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!activeManualBlock} onclick={() => void mutateBlock('shorten')}>
							<Clock class="size-4" /> Shorten
						</button>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!activeManualBlock} onclick={() => void mutateBlock('make-permanent')}>
							<Power class="size-4" /> Permanent
						</button>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!activeManualBlock} onclick={() => void mutateBlock('expire')}>
							<PowerOff class="size-4" /> Expire
						</button>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" disabled={!activeManualBlock} onclick={() => void previewFirewallSync()}>
							<Eye class="size-4" /> Preview firewall
						</button>
					</div>

					<div class="grid gap-4 xl:grid-cols-2">
						<div class="space-y-2">
							<h2 class="text-sm font-semibold">Manual entries</h2>
							{#if subjectDetail.manualEntries.length === 0}
								<p class="text-sm text-muted-foreground">No manual entries.</p>
							{:else}
								{#each subjectDetail.manualEntries as entry (entry.id)}
									<div class="rounded-md border border-border p-3 text-sm">
										<div class="flex items-center justify-between gap-2">
											<span class="font-medium capitalize">{entry.entryType}</span>
											<button class="inline-flex h-8 items-center gap-2 rounded-md border border-border px-2 text-xs" onclick={() => void removeManualEntry(entry)}>
												<Trash2 class="size-3" /> Expire
											</button>
										</div>
										<p class="text-xs text-muted-foreground">{entry.reason ?? 'No reason'} / {formatExpiry(entry.expiresAtUtc, entry.isPermanent)}</p>
										<p class="text-xs text-muted-foreground">Bypass blocking {entry.bypassBlocking ? 'yes' : 'no'}, challenge {entry.bypassChallenge ? 'yes' : 'no'}, SSO {entry.bypassSso ? 'yes' : 'no'}</p>
									</div>
								{/each}
							{/if}
						</div>
						<div class="space-y-2">
							<h2 class="text-sm font-semibold">Matching controls</h2>
							{#each subjectDetail.blocklistEntries as entry (entry.id)}
								<p class="rounded-md border border-border p-2 text-xs">{entry.source}: {entry.normalizedValue} / {entry.enforcementMode} / {entry.reason}</p>
							{/each}
							{#each subjectDetail.resourceRules as rule (rule.id)}
								<p class="rounded-md border border-border p-2 text-xs">Rule {rule.priority}: {rule.matchType} {rule.matchValue} -> {rule.action}</p>
							{/each}
							{#each subjectDetail.firewallApplications as item (`${item.firewallHostId}-${item.enforcement}`)}
								<p class="rounded-md border border-border p-2 text-xs">{item.firewallHostName ?? 'Firewall'}: {item.enforcement} / {item.status}</p>
							{/each}
							{#if subjectDetail.blocklistEntries.length === 0 && subjectDetail.resourceRules.length === 0 && subjectDetail.firewallApplications.length === 0}
								<p class="text-sm text-muted-foreground">No matching blocklists, resource rules, or firewall state.</p>
							{/if}
						</div>
					</div>

					<div class="space-y-2">
						<div class="flex flex-wrap items-center gap-2">
							<h2 class="text-sm font-semibold">Timeline</h2>
							<Filter class="size-4 text-muted-foreground" />
							<input class="h-8 rounded-md border border-border bg-background px-2 text-xs" bind:value={timelineTypeFilter} placeholder="event type" />
							<input class="h-8 rounded-md border border-border bg-background px-2 text-xs" bind:value={timelineResourceFilter} placeholder="resource id" />
						</div>
						<div class="max-h-96 space-y-2 overflow-auto">
							{#each filteredTimeline as event (event.id)}
								<div class="grid gap-1 rounded-md border border-border p-3 text-xs md:grid-cols-[180px_1fr_100px]">
									<span>{formatDate(event.occurredAtUtc)}</span>
									<span>{compact(event.eventType ?? 'event')} / {event.reason ?? event.requestPath ?? 'no details'}</span>
									<span class="text-muted-foreground">{event.decision ?? event.severity ?? ''}</span>
								</div>
							{/each}
							{#if filteredTimeline.length === 0}
								<p class="text-sm text-muted-foreground">No timeline events match the filters.</p>
							{/if}
						</div>
					</div>

					<div class="space-y-2">
						<h2 class="text-sm font-semibold">Request buckets</h2>
						<div class="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
							{#each buckets.slice(0, 6) as bucket (bucket.id)}
								<p class="rounded-md border border-border p-2 text-xs">{formatDate(bucket.bucketStartUtc)} / {bucket.method} {bucket.pathPrefix} / R:{bucket.requestCount} B:{bucket.blockedCount} C:{bucket.challengedCount}</p>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		</div>
	</section>

	<section class="space-y-4">
		<div class="flex items-center justify-between gap-3">
			<div>
				<h2 class="text-lg font-semibold">Blocklist sources</h2>
				<p class="text-sm text-muted-foreground">Recommended and custom feeds remain disabled until explicitly enabled.</p>
			</div>
			<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" onclick={() => void loadBlocklists()}>
				<RefreshCw class="size-4" /> Reload
			</button>
		</div>

		{#if blocklistError}
			<p class="text-sm text-destructive">{blocklistError}</p>
		{/if}

		<div class="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,0.8fr)]">
			<div class="space-y-2">
				{#if blocklistsLoading}
					<p class="text-sm text-muted-foreground">Loading blocklists...</p>
				{:else}
					{#each blocklists as source (source.id)}
						<div class="rounded-md border border-border p-3">
							<div class="flex flex-wrap items-start justify-between gap-3">
								<div>
									<p class="font-medium">{source.name}</p>
									<p class="max-w-3xl break-all text-xs text-muted-foreground">{source.sourceUrl}</p>
									<p class="text-xs text-muted-foreground">{metadataWarning(source)}</p>
								</div>
								<div class="flex flex-wrap gap-2">
									<button class="inline-flex h-8 items-center gap-2 rounded-md border border-border px-2 text-xs" onclick={() => void previewSource(source)}>
										<Eye class="size-3" /> {previewLoadingId === source.id ? 'Previewing' : 'Preview'}
									</button>
									<button class="inline-flex h-8 items-center gap-2 rounded-md border border-border px-2 text-xs" onclick={() => void mutateSource(source, source.enabled ? 'disable' : 'enable')}>
										{#if source.enabled}<PowerOff class="size-3" /> Disable{:else}<Power class="size-3" /> Enable{/if}
									</button>
									<button class="inline-flex h-8 items-center gap-2 rounded-md border border-border px-2 text-xs" onclick={() => void mutateSource(source, 'refresh')}>
										<RefreshCw class="size-3" /> Refresh
									</button>
									<button class="inline-flex h-8 items-center gap-2 rounded-md border border-border px-2 text-xs" onclick={() => void deleteSource(source)}>
										<Trash2 class="size-3" /> Delete
									</button>
								</div>
							</div>
							<div class="mt-2 grid gap-2 text-xs text-muted-foreground sm:grid-cols-4">
								<span>{source.enabled ? 'Enabled' : 'Disabled'}</span>
								<span>{source.format}</span>
								<span>{source.enforcementMode}</span>
								<span>{source.entryCount} entries</span>
							</div>
						</div>
					{/each}
				{/if}
			</div>

			<div class="space-y-4">
				<div class="rounded-md border border-border p-3">
					<h3 class="text-sm font-semibold">Custom URL</h3>
					<div class="mt-3 space-y-2">
						<input class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm" bind:value={customName} placeholder="Name" />
						<input class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm" bind:value={customUrl} placeholder="https://example.test/feed.txt" />
						<div class="grid grid-cols-2 gap-2">
							<select class="h-9 rounded-md border border-border bg-background px-2 text-sm" bind:value={customFormat}>
								<option value="text">Text</option>
								<option value="netset">Netset</option>
								<option value="csv">CSV</option>
								<option value="tsv">TSV</option>
								<option value="json">JSON</option>
								<option value="json_lines">JSON lines</option>
							</select>
							<select class="h-9 rounded-md border border-border bg-background px-2 text-sm" bind:value={customEnforcement}>
								<option value="observe">Observe</option>
								<option value="middleware">Middleware</option>
								<option value="firewall">Firewall</option>
							</select>
						</div>
						<input class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm" bind:value={customColumn} placeholder="CSV/TSV column index" />
						<input class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm" bind:value={customJsonField} placeholder="JSON value field" />
						<label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={customCanFirewall} /> Can firewall enforce</label>
						<label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={customAllowHttp} /> Allow HTTP with warning</label>
						<button class="inline-flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm" onclick={() => void createCustomSource()}>
							<Plus class="size-4" /> Add source
						</button>
					</div>
				</div>

				{#if selectedSource}
					<div class="rounded-md border border-border p-3">
						<h3 class="text-sm font-semibold">Selected source</h3>
						<p class="text-xs text-muted-foreground">{selectedSource.name} / last {formatDate(selectedSource.lastFetchedAtUtc)}</p>
						<p class="text-xs text-muted-foreground">Status: {selectedSource.lastFetchStatus}{selectedSource.isStale ? ' / stale' : ''}</p>
						{#if selectedSource.lastFetchError}
							<p class="text-xs text-destructive">{selectedSource.lastFetchError}</p>
						{/if}
					</div>
				{/if}

				{#if preview}
					<div class="rounded-md border border-border p-3">
						<h3 class="text-sm font-semibold">Preview</h3>
						<p class="text-xs text-muted-foreground">Parsed {preview.parsedCount}, ignored {preview.ignoredCount}, errors {preview.errorCount}</p>
						{#each preview.warnings.slice(0, 3) as warning (warning)}
							<p class="text-xs text-muted-foreground">{warning}</p>
						{/each}
						{#each preview.errors.slice(0, 3) as item (item)}
							<p class="text-xs text-destructive">{item}</p>
						{/each}
					</div>
				{/if}

				<div class="rounded-md border border-border p-3">
					<h3 class="text-sm font-semibold">Fetch runs</h3>
					{#each runs.slice(0, 5) as run (run.id)}
						<p class="text-xs text-muted-foreground">{formatDate(run.startedAtUtc)} / {run.status} / +{run.addedCount} -{run.removedCount} same {run.unchangedCount}</p>
					{/each}
					{#if runs.length === 0}
						<p class="text-xs text-muted-foreground">No runs selected.</p>
					{/if}
				</div>
			</div>
		</div>
	</section>

	<section class="space-y-2">
		<h2 class="text-lg font-semibold">Recent events</h2>
		<div class="grid gap-2 md:grid-cols-2">
			{#each recentEvents.slice(0, 8) as event (event.occurredAtUtc + event.category + event.action + (event.clientIp ?? ''))}
				<p class="rounded-md border border-border p-2 text-xs">{formatDate(event.occurredAtUtc)} / {event.category}:{event.action} / {event.host ?? 'unknown host'} / {event.clientIp ?? 'unknown subject'}</p>
			{/each}
		</div>
	</section>
</section>
