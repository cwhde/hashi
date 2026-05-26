<script lang="ts">
	import { api } from '$lib/api/client';
	import type { PublicStatusItem } from '$lib/api/types';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Search } from 'lucide-svelte';

	let items = $state<PublicStatusItem[]>([]);
	let search = $state('');
	let loading = $state(true);

	$effect(() => {
		void (async () => {
			try {
				items = await api.getPublicStatus();
			} catch {
				items = [];
			} finally {
				loading = false;
			}
		})();
	});

	const filtered = $derived(
		items.filter((i) => i.name.toLowerCase().includes(search.toLowerCase()))
	);

	const up = $derived(items.filter((i) => i.status === 'Up').length);
</script>

<section class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-white">Status</h1>
		<p class="text-sm text-muted-foreground">Public uptime view on port 8082.</p>
	</div>

	<div class="relative max-w-md">
		<Search class="absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
		<Input bind:value={search} placeholder="Search monitored services…" class="pl-9" />
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<OverviewWidget title="Overall uptime" description="Aggregate public status summary.">
			<StatusRow label="Services up" value="{up} / {items.length}" status="ok" />
		</OverviewWidget>
	</div>

	<div class="overflow-hidden rounded-lg border border-border">
		<div
			class="grid grid-cols-[1fr_5rem_5rem] gap-2 border-b border-border bg-card/40 px-3 py-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground"
		>
			<span>Service</span>
			<span>State</span>
			<span>Latency</span>
		</div>
		{#if loading}
			<p class="px-3 py-6 text-sm text-muted-foreground">Loading…</p>
		{:else if filtered.length === 0}
			<p class="px-3 py-6 text-sm text-muted-foreground">No public status entries.</p>
		{:else}
			{#each filtered as item}
				<div class="grid grid-cols-[1fr_5rem_5rem] gap-2 border-b border-border/50 px-3 py-2 text-sm last:border-0">
					<span class="truncate text-white">{item.name}</span>
					<span class="text-xs">{item.status}</span>
					<span class="text-xs tabular-nums">{item.lastLatencyMs ?? '—'} ms</span>
				</div>
			{/each}
		{/if}
	</div>
</section>
