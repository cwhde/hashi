<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import AddCheck from '$lib/components/hashi/status/AddCheck.svelte';
	import RecentNotifications from '$lib/components/hashi/status/RecentNotifications.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Select from '$lib/components/ui/select';
	import { Plus, Search } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import {
		checkSummary,
		sourceText,
		stateOrder,
		stripHealth,
		uptimeText,
		type Endpoint
	} from '$lib/monitor';

	// 07 §5, 10b §3: endpoint cards grouped, with history strips; Notifications tab.
	let endpoints = $state<Endpoint[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let query = $state('');
	let groupBy = $state('group');
	let sortBy = $state('name');
	let addOpen = $state(false);

	async function load() {
		const r = await api.GET('/api/monitor/endpoints');
		if (r.error || !r.data) throw r.error;
		endpoints = r.data.endpoints;
		loaded = true;
	}
	onMount(() => {
		load().catch((e) => (error = problemText(e)));
		const t = setInterval(() => load().catch(() => {}), 20_000);
		return () => clearInterval(t);
	});

	const groupKey = (e: Endpoint) =>
		groupBy === 'status'
			? e.state
			: groupBy === 'source'
				? (sourceText[e.source_type] ?? 'Other')
				: e.group;
	const sorters: Record<string, (a: Endpoint, b: Endpoint) => number> = {
		name: (a, b) => a.name.localeCompare(b.name),
		state: (a, b) => (stateOrder[a.state] ?? 9) - (stateOrder[b.state] ?? 9),
		latency: (a, b) => (b.last_latency_ms ?? 0) - (a.last_latency_ms ?? 0),
		uptime: (a, b) => (a.uptime_24h ?? 101) - (b.uptime_24h ?? 101),
		event: (a, b) => (b.state_since ?? '').localeCompare(a.state_since ?? '')
	};
	const groupNames: Record<string, string> = {
		group: 'Group by group',
		status: 'Group by status',
		source: 'Group by type'
	};
	const sortNames: Record<string, string> = {
		name: 'Sort by name',
		state: 'Sort by state',
		latency: 'Sort by latency',
		uptime: 'Sort by uptime',
		event: 'Sort by last event'
	};
	const groups = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const list = endpoints
			.filter((e) => !q || e.name.toLowerCase().includes(q) || e.group.toLowerCase().includes(q))
			.sort(sorters[sortBy]);
		return Object.entries(Object.groupBy(list, groupKey)).sort(([a], [b]) => a.localeCompare(b));
	});
</script>

<svelte:head><title>Status · Hashi</title></svelte:head>

<PageHeader
	title="Status"
	description="Built-in monitoring for services, hosts, connections and your own checks."
>
	{#snippet actions()}<Button onclick={() => (addOpen = true)}><Plus />Add check</Button>{/snippet}
</PageHeader>

{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}

<Tabs.Root value="endpoints">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="endpoints">Endpoints</Tabs.Trigger>
		<Tabs.Trigger value="notify">Notifications</Tabs.Trigger>
	</Tabs.List>
	<Tabs.Content value="endpoints">
		<div class="mb-4 flex flex-wrap items-center gap-2">
			<div class="relative w-full max-w-72">
				<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					bind:value={query}
					placeholder="Search endpoints"
					class="pl-8"
					aria-label="Search endpoints"
				/>
			</div>
			<Select.Root type="single" bind:value={groupBy}>
				<Select.Trigger class="w-44">{groupNames[groupBy]}</Select.Trigger>
				<Select.Content>
					{#each Object.entries(groupNames) as [v, l] (v)}<Select.Item value={v}>{l}</Select.Item
						>{/each}
				</Select.Content>
			</Select.Root>
			<Select.Root type="single" bind:value={sortBy}>
				<Select.Trigger class="w-44">{sortNames[sortBy]}</Select.Trigger>
				<Select.Content>
					{#each Object.entries(sortNames) as [v, l] (v)}<Select.Item value={v}>{l}</Select.Item
						>{/each}
				</Select.Content>
			</Select.Root>
		</div>
		{#if !loaded && !error}
			<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
				<Skeleton class="h-32" /><Skeleton class="h-32" /><Skeleton class="h-32" />
			</div>
		{/if}
		{#if loaded && groups.length === 0}
			<p class="text-[13.5px] text-muted-foreground">
				{query ? 'No endpoint matches your search.' : 'Checks start within a few seconds.'}
			</p>
		{/if}
		<div class="space-y-6">
			{#each groups as [name, items] (name)}
				{@const failing = (items ?? []).filter(
					(m) => m.state === 'down' || m.state === 'degraded'
				).length}
				<section>
					<h2 class="mb-2 flex items-center gap-2 text-[14px] font-semibold capitalize">
						{name}{#if failing}<Tag tone="bad">{failing} failing</Tag>{/if}
					</h2>
					<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
						{#each items ?? [] as m (m.id)}
							<a
								href="/status/{m.id}"
								class="block rounded-2xl border bg-card p-3.5 transition-colors hover:border-muted-foreground/40"
							>
								<div class="mb-2.5 flex items-start gap-3">
									<div class="min-w-0 flex-1">
										<div class="truncate text-[13.5px] font-medium">{m.name}</div>
										<div class="truncate text-[12px] text-muted-foreground">{checkSummary(m)}</div>
									</div>
									{#if m.pending_dns}<Tag>DNS updating</Tag>{/if}<StatusBadge state={m.state} />
								</div>
								<HistoryStrip samples={stripHealth(m.strip)} />
								<div class="mt-1.5 flex justify-between text-[12px] text-muted-foreground">
									<span>{sourceText[m.source_type]}</span><span class="tnum"
										>{m.last_latency_ms !== undefined
											? m.last_latency_ms + ' ms · '
											: ''}{uptimeText(m.uptime_24h)}</span
									>
								</div>
							</a>
						{/each}
					</div>
				</section>
			{/each}
		</div>
	</Tabs.Content>
	<Tabs.Content value="notify"><RecentNotifications /></Tabs.Content>
</Tabs.Root>

<AddCheck bind:open={addOpen} onadded={() => load().catch(() => {})} />
