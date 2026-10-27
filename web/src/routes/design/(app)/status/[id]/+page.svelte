<script lang="ts">
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import LineChart from '$lib/components/hashi/LineChart.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import { spark } from '$lib/design/mock';
	import { ChevronLeft } from '@lucide/svelte';

	let { data } = $props();
	const m = $derived(data.m);
	let range = $state('24h');
</script>

<a
	href="/design/status"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Status</a
>
<div class="mb-4 rounded-2xl border bg-card p-4">
	<div class="flex flex-wrap items-start gap-4">
		<div class="flex-1">
			<h1 class="text-[22px] font-semibold">{m.name}</h1>
			<p class="text-[13.5px] text-muted-foreground">{m.kind} · {m.address} · every 60 s</p>
		</div>
		<StatusBadge state={m.state} />
		<Button variant="outline" size="sm">Pause</Button><Button variant="outline" size="sm"
			>Edit check</Button
		>
	</div>
	<dl class="mt-4 grid grid-cols-2 gap-3 text-[13px] sm:grid-cols-4 lg:grid-cols-7">
		{#each [['Last check', '12 s ago'], ['Response min', '31 ms'], ['avg', m.latency + ' ms'], ['max', '412 ms'], ['Uptime 1 h', '100%'], ['24 h', m.uptime24], ['30 d', '99.91%']] as [k, v] (k)}
			<div>
				<dt class="text-[12px] text-muted-foreground">{k}</dt>
				<dd class="tnum">{v}</dd>
			</div>
		{/each}
	</dl>
	<HistoryStrip samples={m.history} class="mt-4" />
</div>
<div class="mb-3 flex justify-center">
	<ToggleGroup.Root type="single" bind:value={range} variant="outline" size="sm">
		{#each ['1h', '24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}>{r}</ToggleGroup.Item
			>{/each}
	</ToggleGroup.Root>
</div>
<div class="mb-4 grid gap-4 lg:grid-cols-3">
	<LineChart title="Response time" values={spark(4, 60, 60, 30)} unit=" ms" />
	<LineChart title="Public vs internal gap" values={spark(8, 60, 12, 8)} unit=" ms" />
	<LineChart title="Availability" values={spark(1, 60, 99, 1)} current={m.uptime24} />
</div>
<Panel title="Events">
	<ol class="space-y-2.5 text-[13px]">
		<li class="grid grid-cols-[6rem_1fr] gap-3">
			<span class="tnum text-muted-foreground">13:12</span><span
				><StatusBadge state="degraded" /> p95 412 ms over 300 ms for 3 checks</span
			>
		</li>
		<li class="grid grid-cols-[6rem_1fr] gap-3">
			<span class="tnum text-muted-foreground">Yesterday</span><span
				><StatusBadge state="up" /> Recovered after 4 min (public path only was down: edge, not service)</span
			>
		</li>
	</ol>
</Panel>
