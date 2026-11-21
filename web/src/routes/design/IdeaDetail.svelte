<script lang="ts">
	// Idea mock: one service on the public status page. Only what is safe in public (07 §7): state, uptime, response
	// time, downtimes with times and lengths. No targets, ports, protocols, status codes or error text.
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import LineChart from '#lib/components/hashi/LineChart.svelte';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import IdeaBadges from './IdeaBadges.svelte';
	import { downtimes, responseSeries, type Entry } from './ideas.ts';

	let { e }: { e: Entry } = $props();
	let range = $state('24h');
	const series = $derived(responseSeries(e.id.length + Number(e.id.slice(1)) + range.length));
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center gap-3 pr-8">
		<span
			class="grid size-10 place-items-center rounded-xl text-lg font-semibold text-white"
			style="background:hsl({e.hue} 45% 42%)">{e.letter}</span
		>
		<div class="min-w-0 flex-1">
			<h2 class="truncate text-title font-semibold">{e.name}</h2>
			<p class="text-md text-muted-foreground">{e.group || 'Other'}</p>
		</div>
		<StatusBadge state={e.state} />
		<IdeaBadges slug={e.name.toLowerCase().replace(/\s+/g, '-')} />
	</div>

	<dl class="grid grid-cols-2 gap-2 sm:grid-cols-5">
		{#each [['Now', e.state === 'down' ? 'Down for 6 min' : e.state === 'degraded' ? 'Slow for 12 min' : 'Up for 14 h'], ['24 hours', e.uptime.h24 + '%'], ['7 days', e.uptime.d7 + '%'], ['30 days', e.uptime.d30.toFixed(2) + '%'], ['Response', e.ms + ' ms']] as [k, v] (k)}
			<div class="rounded-xl border bg-card px-3 py-2">
				<dt class="text-xs text-muted-foreground">{k}</dt>
				<dd class="tnum text-base font-medium">{v}</dd>
			</div>
		{/each}
	</dl>

	<div class="rounded-xl border bg-card p-3">
		<HistoryStrip samples={e.days} from="90 days ago" to="today" />
	</div>

	<div>
		<div class="mb-2 flex justify-end">
			<ToggleGroup.Root type="single" bind:value={range} variant="outline" size="sm">
				{#each ['24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}>{r}</ToggleGroup.Item
					>{/each}
			</ToggleGroup.Root>
		</div>
		<LineChart title="Response time" values={series} unit=" ms" />
	</div>

	<div class="rounded-xl border bg-card">
		<h3 class="border-b px-3 py-2 text-md font-semibold">Recent downtimes</h3>
		<ol class="divide-y text-md">
			{#each downtimes as d (d.start)}
				<li class="grid grid-cols-[1fr_auto] gap-3 px-3 py-2">
					<span class="tnum">{d.start} – {d.end}</span><span class="tnum text-muted-foreground"
						>{d.length}</span
					>
				</li>
			{/each}
		</ol>
	</div>
</div>
