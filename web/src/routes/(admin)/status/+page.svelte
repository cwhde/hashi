<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { MonitorEndpoint, MonitorRollup } from '$lib/api/types';
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

	let endpoints = $state<MonitorEndpoint[]>([]);
	let rollups = $state<MonitorRollup[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let search = $state('');
	let selectedId = $state<string | null>(null);
	let hours = $state(1);
	let savingPublicId = $state<string | null>(null);

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			const [endpointList, rollupList] = await Promise.all([
				api.listStatusEndpoints(),
				api.listStatusRollups({ intervalMinutes: 1, hours })
			]);
			endpoints = endpointList;
			rollups = rollupList;
			if (!selectedId && endpointList.length > 0) {
				selectedId = endpointList[0]?.id ?? null;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load monitors';
		} finally {
			loading = false;
		}
	}

	const filtered = $derived(
		endpoints.filter((e) => e.name.toLowerCase().includes(search.toLowerCase()))
	);

	const counts = $derived({
		up: endpoints.filter((e) => e.status === 'Up').length,
		degraded: endpoints.filter((e) => e.status === 'Degraded').length,
		down: endpoints.filter((e) => e.status === 'Down').length
	});

	function stripFor(endpointId: string) {
		return rollups
			.filter((r) => r.monitorEndpointId === endpointId)
			.map((r) => ({ up: r.upCount >= r.downCount }));
	}

	const selectedRollups = $derived(
		selectedId ? rollups.filter((r) => r.monitorEndpointId === selectedId) : []
	);

	const chartData = $derived.by(() => {
		const timestamps = selectedRollups.map((r) => new Date(r.bucketStartUtc).getTime() / 1000);
		const latencies = selectedRollups.map((r) => Number(r.averageLatencyMs));
		return { timestamps, latencies };
	});

	const selectedEndpoint = $derived(endpoints.find((e) => e.id === selectedId) ?? null);

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
</script>

<AdminSectionPage
	title="Status"
	description="Monitors, 60-minute strips, latency charts, and public status page config."
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

	<div class="flex flex-wrap items-center gap-3">
		<label class="text-sm text-muted-foreground" for="status-hours">Range</label>
		<select
			id="status-hours"
			class="rounded-md border border-border bg-background px-2 py-1 text-sm"
			bind:value={hours}
			onchange={() => void load()}
		>
			<option value={1}>Last hour</option>
			<option value={24}>Last 24 hours</option>
			<option value={168}>Last 7 days</option>
		</select>
	</div>

	<div class="relative max-w-md">
		<Search class="absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
		<Input bind:value={search} placeholder="Search monitors…" class="pl-9" />
	</div>

	<PanelSection title="Monitored endpoints" description="Rollups at 1-minute resolution for the selected range.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if filtered.length === 0}
			<p class="text-sm text-muted-foreground">
				No monitor endpoints yet. Enable status on a resource to auto-provision checks.
			</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>60 min</TableHead>
						<TableHead>Status</TableHead>
						<TableHead>Public</TableHead>
						<TableHead>Latency</TableHead>
						<TableHead>Last check</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each filtered as endpoint (endpoint.id)}
						<TableRow
							class={selectedId === endpoint.id ? 'bg-card/60' : 'cursor-pointer'}
							onclick={() => (selectedId = endpoint.id)}
						>
							<TableCell>{endpoint.name}</TableCell>
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

	{#if selectedEndpoint}
		<PanelSection
			title="{selectedEndpoint.name} — selected range"
			description="Latency from 1-minute rollups. Select another row to switch."
		>
			<MonitorLatencyChart timestamps={chartData.timestamps} latencies={chartData.latencies} />
		</PanelSection>
	{/if}
</AdminSectionPage>
