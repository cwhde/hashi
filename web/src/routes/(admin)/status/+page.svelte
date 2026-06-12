<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { MonitorEndpoint, MonitorEvent, MonitorRollup } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import MonitorLatencyChart from '$lib/components/monitoring/MonitorLatencyChart.svelte';
	import MonitorStatusStrip from '$lib/components/monitoring/MonitorStatusStrip.svelte';
	import { Input } from '$lib/components/ui/input';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { Switch } from '$lib/components/ui/switch';
	import { HeartPulse, Search } from 'lucide-svelte';

	type GroupMode = 'none' | 'host' | 'firewallHost' | 'status' | 'resourceType';
	type SortMode = 'name' | 'state' | 'latency' | 'uptime' | 'lastEvent';

	const rangeOptions = [
		{ label: 'Last hour', hours: 1 },
		{ label: 'Last 24 hours', hours: 24 },
		{ label: 'Last 7 days', hours: 168 },
		{ label: 'Last 30 days', hours: 720 }
	];

	let endpoints = $state<MonitorEndpoint[]>([]);
	let rangeRollups = $state<MonitorRollup[]>([]);
	let stripRollups = $state<MonitorRollup[]>([]);
	let events = $state<MonitorEvent[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let search = $state('');
	let selectedId = $state<string | null>(null);
	let hours = $state(1);
	let groupBy = $state<GroupMode>('none');
	let sortBy = $state<SortMode>('name');
	let savingPublicId = $state<string | null>(null);

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			const [endpointList, rangeRollupList, stripRollupList, eventList] = await Promise.all([
				api.listStatusEndpoints(),
				api.listStatusRollups({ intervalMinutes: rangeIntervalMinutes(hours), hours }),
				api.listStatusRollups({ intervalMinutes: 1, hours: 1 }),
				api.listStatusEvents({ hours })
			]);
			endpoints = endpointList;
			rangeRollups = rangeRollupList;
			stripRollups = stripRollupList;
			events = eventList;
			if (!selectedId || !endpointList.some((endpoint) => endpoint.id === selectedId)) {
				selectedId = endpointList[0]?.id ?? null;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load monitors';
		} finally {
			loading = false;
		}
	}

	const filtered = $derived.by(() => {
		const term = search.trim().toLowerCase();
		const matches = term
			? endpoints.filter((endpoint) =>
					[
						endpoint.name,
						endpoint.status,
						endpoint.host,
						endpoint.firewallHostName,
						endpoint.resourceType,
						endpoint.url
					]
						.filter(Boolean)
						.some((value) => String(value).toLowerCase().includes(term))
				)
			: [...endpoints];
		return matches.sort((a, b) => compareEndpoints(a, b, sortBy));
	});

	const groupedEndpoints = $derived.by(() => {
		if (groupBy === 'none') {
			return [{ key: 'all', label: 'All endpoints', endpoints: filtered }];
		}

		const groups: Record<string, MonitorEndpoint[]> = {};
		for (const endpoint of filtered) {
			const label = groupLabel(endpoint, groupBy);
			groups[label] = [...(groups[label] ?? []), endpoint];
		}

		return Object.entries(groups)
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([label, groupEndpoints]) => ({
				key: `${groupBy}:${label}`,
				label,
				endpoints: groupEndpoints
			}));
	});

	const counts = $derived({
		up: endpoints.filter((e) => e.status === 'Up').length,
		degraded: endpoints.filter((e) => e.status === 'Degraded').length,
		down: endpoints.filter((e) => e.status === 'Down').length
	});

	const selectedEndpoint = $derived(endpoints.find((e) => e.id === selectedId) ?? null);
	const selectedRollups = $derived(
		selectedId ? rangeRollups.filter((r) => r.monitorEndpointId === selectedId) : []
	);
	const selectedEvents = $derived(
		selectedId ? events.filter((event) => event.monitorEndpointId === selectedId) : []
	);
	const selectedStats = $derived(calculateStats(selectedRollups));

	const chartData = $derived.by(() => {
		const timestamps = selectedRollups.map((r) => new Date(r.bucketStartUtc).getTime() / 1000);
		const latencies = selectedRollups.map((r) => Number(r.averageLatencyMs));
		return { timestamps, latencies };
	});

	function rangeIntervalMinutes(selectedHours: number) {
		if (selectedHours <= 1) return 1;
		if (selectedHours <= 24) return 5;
		return 60;
	}

	function stripFor(endpointId: string) {
		return stripRollups
			.filter((r) => r.monitorEndpointId === endpointId)
			.map((r) => ({ up: Number(r.upCount) >= Number(r.downCount) }));
	}

	function rollupsFor(endpointId: string) {
		return rangeRollups.filter((r) => r.monitorEndpointId === endpointId);
	}

	function calculateStats(rollups: MonitorRollup[]) {
		const latencies = rollups
			.map((rollup) => Number(rollup.averageLatencyMs))
			.filter((latency) => Number.isFinite(latency));
		const sampleCount = rollups.reduce((sum, rollup) => sum + Number(rollup.sampleCount), 0);
		const upCount = rollups.reduce((sum, rollup) => sum + Number(rollup.upCount), 0);
		const downCount = rollups.reduce((sum, rollup) => sum + Number(rollup.downCount), 0);
		const total = upCount + downCount;
		return {
			minLatency: latencies.length ? Math.min(...latencies) : null,
			maxLatency: latencies.length ? Math.max(...latencies) : null,
			avgLatency: latencies.length
				? latencies.reduce((sum, latency) => sum + latency, 0) / latencies.length
				: null,
			sampleCount,
			upCount,
			downCount,
			uptime: total > 0 ? (upCount / total) * 100 : null
		};
	}

	function compareEndpoints(a: MonitorEndpoint, b: MonitorEndpoint, mode: SortMode) {
		if (mode === 'state') return stateRank(a.status) - stateRank(b.status) || a.name.localeCompare(b.name);
		if (mode === 'latency') return nullableNumber(a.lastLatencyMs) - nullableNumber(b.lastLatencyMs);
		if (mode === 'uptime') return endpointUptime(b.id) - endpointUptime(a.id) || a.name.localeCompare(b.name);
		if (mode === 'lastEvent') return lastEventTime(b.id) - lastEventTime(a.id) || a.name.localeCompare(b.name);
		return a.name.localeCompare(b.name);
	}

	function stateRank(status: string) {
		return { Down: 0, Degraded: 1, Unknown: 2, Paused: 3, Up: 4 }[status] ?? 2;
	}

	function nullableNumber(value: number | string | null | undefined) {
		const parsed = Number(value);
		return Number.isFinite(parsed) ? parsed : Number.MAX_SAFE_INTEGER;
	}

	function endpointUptime(endpointId: string) {
		return calculateStats(rollupsFor(endpointId)).uptime ?? -1;
	}

	function lastEventTime(endpointId: string) {
		const latest = events.find((event) => event.monitorEndpointId === endpointId);
		return latest ? new Date(latest.occurredAtUtc).getTime() : 0;
	}

	function groupLabel(endpoint: MonitorEndpoint, mode: GroupMode) {
		if (mode === 'host') return endpoint.host || 'Unknown host';
		if (mode === 'firewallHost') return endpoint.firewallHostName || 'No Linux firewall host';
		if (mode === 'status') return endpoint.status || 'Unknown';
		if (mode === 'resourceType') return endpoint.resourceType || endpoint.checkType || 'Unknown type';
		return 'All endpoints';
	}

	function formatLatency(value: number | string | null | undefined) {
		const parsed = Number(value);
		return Number.isFinite(parsed) ? `${Math.round(parsed)} ms` : '-';
	}

	function formatPercent(value: number | null | undefined) {
		return value == null ? '-' : `${value.toFixed(2)}%`;
	}

	function formatDate(value: string | null | undefined) {
		return value ? new Date(value).toLocaleString() : '-';
	}

	function eventSummary(event: MonitorEvent) {
		return `${event.previousStatus} -> ${event.newStatus}`;
	}

	async function togglePublicStatus(endpoint: MonitorEndpoint, checked: boolean) {
		savingPublicId = endpoint.id;
		error = null;
		try {
			const updated = await api.updateStatusEndpoint(endpoint.id, { publicStatusEnabled: checked });
			endpoints = endpoints.map((item) => (item.id === endpoint.id ? updated : item));
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update public status selection';
		} finally {
			savingPublicId = null;
		}
	}

	async function togglePaused(endpoint: MonitorEndpoint, checked: boolean) {
		error = null;
		try {
			const updated = await api.updateStatusEndpoint(endpoint.id, { paused: checked });
			endpoints = endpoints.map((item) => (item.id === endpoint.id ? updated : item));
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update paused status';
		}
	}
</script>

<AdminSectionPage
	title="Status"
	description="Monitors, grouped health views, latency ranges, and status events."
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

	<div class="flex flex-wrap items-end gap-3">
		<label class="grid gap-1 text-sm text-muted-foreground" for="status-hours">
			<span>Range</span>
			<select
				id="status-hours"
				class="rounded-md border border-border bg-background px-2 py-1 text-sm text-foreground"
				bind:value={hours}
				onchange={() => void load()}
			>
				{#each rangeOptions as option (option.hours)}
					<option value={option.hours}>{option.label}</option>
				{/each}
			</select>
		</label>
		<label class="grid gap-1 text-sm text-muted-foreground" for="status-group">
			<span>Group</span>
			<select
				id="status-group"
				class="rounded-md border border-border bg-background px-2 py-1 text-sm text-foreground"
				bind:value={groupBy}
			>
				<option value="none">None</option>
				<option value="host">Host</option>
				<option value="firewallHost">Linux firewall host</option>
				<option value="status">Status</option>
				<option value="resourceType">Resource type</option>
			</select>
		</label>
		<label class="grid gap-1 text-sm text-muted-foreground" for="status-sort">
			<span>Sort</span>
			<select
				id="status-sort"
				class="rounded-md border border-border bg-background px-2 py-1 text-sm text-foreground"
				bind:value={sortBy}
			>
				<option value="name">Name</option>
				<option value="state">State</option>
				<option value="latency">Latency</option>
				<option value="uptime">Uptime</option>
				<option value="lastEvent">Last event</option>
			</select>
		</label>
		<div class="relative min-w-64 max-w-md flex-1">
			<Search class="absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
			<Input bind:value={search} placeholder="Search monitors..." class="pl-9" />
		</div>
	</div>

	<PanelSection
		title="Monitored endpoints"
		description="Groups and sorts use endpoint metadata plus rollups and events in the selected range."
	>
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading...</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if filtered.length === 0}
			<p class="text-sm text-muted-foreground">
				No monitor endpoints yet. Enable status on a resource to auto-provision checks.
			</p>
		{:else}
			<div class="space-y-5">
				{#each groupedEndpoints as group (group.key)}
					<section class="space-y-2">
						<div class="flex items-center justify-between gap-3">
							<h3 class="text-sm font-medium">{group.label}</h3>
							<span class="text-xs text-muted-foreground">{group.endpoints.length} endpoints</span>
						</div>
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Name</TableHead>
									<TableHead>60 min</TableHead>
									<TableHead>Status</TableHead>
									<TableHead>Public</TableHead>
									<TableHead>Paused</TableHead>
									<TableHead>Latency</TableHead>
									<TableHead>Uptime</TableHead>
									<TableHead>Last event</TableHead>
									<TableHead>Last check</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{#each group.endpoints as endpoint (endpoint.id)}
									{@const stats = calculateStats(rollupsFor(endpoint.id))}
									{@const latestEvent = events.find((event) => event.monitorEndpointId === endpoint.id)}
									<TableRow
										class={selectedId === endpoint.id ? 'bg-card/60' : 'cursor-pointer'}
										onclick={() => (selectedId = endpoint.id)}
									>
										<TableCell>
											<div class="grid gap-1">
												<span>{endpoint.name}</span>
												<span class="text-xs text-muted-foreground">
													{endpoint.host || endpoint.url}
												</span>
											</div>
										</TableCell>
										<TableCell>
											<MonitorStatusStrip buckets={stripFor(endpoint.id)} />
										</TableCell>
										<TableCell>{endpoint.status}</TableCell>
										<TableCell>
											<Switch
												checked={endpoint.publicStatusEnabled}
												disabled={savingPublicId === endpoint.id}
												onclick={(event) => event.stopPropagation()}
												onCheckedChange={(checked) => void togglePublicStatus(endpoint, checked)}
											/>
										</TableCell>
										<TableCell>
											<Switch
												checked={endpoint.status === 'Paused'}
												onclick={(event) => event.stopPropagation()}
												onCheckedChange={(checked) => void togglePaused(endpoint, checked)}
											/>
										</TableCell>
										<TableCell>{formatLatency(endpoint.lastLatencyMs)}</TableCell>
										<TableCell>{formatPercent(stats.uptime)}</TableCell>
										<TableCell class="text-xs">
											{latestEvent ? formatDate(latestEvent.occurredAtUtc) : '-'}
										</TableCell>
										<TableCell class="text-xs">{formatDate(endpoint.lastCheckedAtUtc)}</TableCell>
									</TableRow>
								{/each}
							</TableBody>
						</Table>
					</section>
				{/each}
			</div>
		{/if}
	</PanelSection>

	{#if selectedEndpoint}
		<div class="grid gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.9fr)]">
			<PanelSection
				title="{selectedEndpoint.name} detail"
				description="Current state, response-time summary, uptime, and selected range latency."
			>
				<div class="grid gap-3 md:grid-cols-4">
					<StatusRow label="Status" value={selectedEndpoint.status} status={selectedEndpoint.status === 'Up' ? 'ok' : selectedEndpoint.status === 'Down' ? 'error' : 'warn'} />
					<StatusRow label="Last check" value={formatDate(selectedEndpoint.lastCheckedAtUtc)} />
					<StatusRow label="Uptime" value={formatPercent(selectedStats.uptime)} />
					<StatusRow label="Samples" value={String(selectedStats.sampleCount)} />
				</div>
				<div class="mt-4 grid gap-3 md:grid-cols-3">
					<StatusRow label="Min response" value={formatLatency(selectedStats.minLatency)} />
					<StatusRow label="Avg response" value={formatLatency(selectedStats.avgLatency)} />
					<StatusRow label="Max response" value={formatLatency(selectedStats.maxLatency)} />
				</div>
				<div class="mt-4">
					<MonitorLatencyChart timestamps={chartData.timestamps} latencies={chartData.latencies} />
				</div>
			</PanelSection>

			<div class="space-y-4">
				<PanelSection title="Endpoint settings" description="Operational settings for the selected monitor.">
					<div class="space-y-3 text-sm">
						<StatusRow label="URL" value={selectedEndpoint.url} />
						<StatusRow label="Check type" value={selectedEndpoint.checkType} />
						<StatusRow label="Source" value={selectedEndpoint.provisioned ? 'Provisioned resource' : 'Manual or infrastructure'} />
						<StatusRow label="Resource type" value={selectedEndpoint.resourceType || '-'} />
						<StatusRow label="Host" value={selectedEndpoint.host || '-'} />
						<StatusRow label="Linux firewall host" value={selectedEndpoint.firewallHostName || '-'} />
						<div class="flex items-center justify-between gap-3 rounded-md border border-border/70 px-3 py-2">
							<span class="text-muted-foreground">Public status</span>
							<Switch
								checked={selectedEndpoint.publicStatusEnabled}
								disabled={savingPublicId === selectedEndpoint.id}
								onCheckedChange={(checked) => void togglePublicStatus(selectedEndpoint, checked)}
							/>
						</div>
						<div class="flex items-center justify-between gap-3 rounded-md border border-border/70 px-3 py-2">
							<span class="text-muted-foreground">Paused</span>
							<Switch
								checked={selectedEndpoint.status === 'Paused'}
								onCheckedChange={(checked) => void togglePaused(selectedEndpoint, checked)}
							/>
						</div>
					</div>
				</PanelSection>

				<PanelSection title="Event timeline" description="Status transitions in the selected range.">
					{#if selectedEvents.length === 0}
						<p class="text-sm text-muted-foreground">No status events in this range.</p>
					{:else}
						<ol class="space-y-3">
							{#each selectedEvents as event (event.id)}
								<li class="rounded-md border border-border/70 px-3 py-2">
									<div class="flex items-center justify-between gap-3">
										<span class="text-sm font-medium">{eventSummary(event)}</span>
										<span class="text-xs text-muted-foreground">{formatDate(event.occurredAtUtc)}</span>
									</div>
									<p class="mt-1 text-xs text-muted-foreground">
										Latency {formatLatency(event.latencyMs)}
									</p>
								</li>
							{/each}
						</ol>
					{/if}
				</PanelSection>
			</div>
		</div>
	{/if}
</AdminSectionPage>
