<script lang="ts">
	// Approved mock (07 §7.2, reworked 2026-11-22 after the owner's review): one service on the status
	// listener, with an address to share. Public fields only: state, uptime, response times, downtimes with times and
	// lengths. Three charts that show the value under the pointer, as the admin endpoint page does; the 90-day strip
	// says what happened on each day; one Badges button holds every badge (07 §7.3).
	import { page } from '$app/state';
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import LineChart from '#lib/components/hashi/LineChart.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { ArrowLeft } from '@lucide/svelte';
	import BadgesButton from '../../../../BadgesButton.svelte';
	import DayStrip from '#lib/components/hashi/DayStrip.svelte';
	import { dayDetail, downtimes, entries, responseSeries, slug } from '../../../../ideas.ts';

	const e = $derived(entries.find((x) => slug(x.name) === page.params.slug) ?? entries[0]!);
	let range = $state('24h');
	const seed = $derived(Number(e.id.slice(1)) + range.length);
	const series = $derived(responseSeries(seed));
	const slowest = $derived(series.map((v, i) => Math.round(v * 1.6 + ((i * 13 + seed) % 9) * 6)));
	const avail = $derived(
		series.map((_, i) =>
			e.state === 'down' && i === series.length - 1 ? 80 : (i * 7 + seed) % 23 === 0 ? 98.6 : 100
		)
	);
	// One point per half hour, per 3.5 hours or per 15 hours, ending now (13:42 on 22 Nov).
	const step = $derived({ '24h': 30, '7d': 210, '30d': 900 }[range] ?? 30);
	const labels = $derived(
		series.map((_, i) => {
			const t = new Date(Date.UTC(2026, 10, 22, 13, 42) - (series.length - 1 - i) * step * 60_000);
			return t.toLocaleString('en-GB', {
				day: 'numeric',
				month: 'short',
				hour: '2-digit',
				minute: '2-digit',
				timeZone: 'UTC'
			});
		})
	);
	const pct = (v: number) => `${v.toFixed(v === 100 ? 0 : 2)}%`;
	const since = $derived(
		e.state === 'down' ? 'Down for' : e.state === 'degraded' ? 'Slow for' : 'Up for'
	);
	const tiles = $derived([
		[since, e.state === 'down' ? '6 min' : e.state === 'degraded' ? '12 min' : '14 h'],
		['Uptime 24 h', `${e.uptime.h24}%`],
		['Uptime 7 d', `${e.uptime.d7}%`],
		['Uptime 30 d', `${e.uptime.d30.toFixed(2)}%`],
		['Response 24 h', `${e.ms} ms`]
	]);
</script>

<svelte:head><title>{e.name} · example.com status</title></svelte:head>
<div class="mx-auto max-w-5xl space-y-5 px-4 py-10">
	<a
		href="/design/public-status"
		class="inline-flex items-center gap-1.5 text-md text-muted-foreground hover:text-foreground"
		><ArrowLeft class="size-4" />example.com status</a
	>
	<header class="flex flex-wrap items-center gap-3">
		<AppIcon name={e.name} class="size-11 rounded-xl text-lg" />
		<div class="min-w-0 flex-1">
			<h1 class="truncate text-title font-semibold">{e.name}</h1>
			<p class="text-md text-muted-foreground">{e.group || 'Other'}</p>
		</div>
		<StatusBadge state={e.state} />
		<BadgesButton {e} />
	</header>

	<dl class="grid grid-cols-2 gap-2 sm:grid-cols-5">
		{#each tiles as [k, v] (k)}
			<div class="rounded-xl border bg-card px-3 py-2 first:col-span-2 sm:first:col-span-1">
				<dt class="text-sm text-muted-foreground">{k}</dt>
				<dd class="tnum text-base font-medium">{v}</dd>
			</div>
		{/each}
	</dl>

	<section class="rounded-2xl border bg-card p-4">
		<h2 class="mb-2 text-md font-semibold">Last 90 days</h2>
		<DayStrip samples={e.days} detail={(i) => dayDetail(e, i)} />
	</section>

	<section>
		<div class="mb-2 flex justify-end">
			<ToggleGroup.Root
				type="single"
				value={range}
				onValueChange={(v) => v && (range = v)}
				variant="outline"
				size="sm"
			>
				{#each ['24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}>{r}</ToggleGroup.Item
					>{/each}
			</ToggleGroup.Root>
		</div>
		<div class="grid gap-4 lg:grid-cols-3">
			<LineChart title="Response time" values={series} unit=" ms" {labels} />
			<LineChart title="Slowest responses" values={slowest} unit=" ms" {labels} />
			<LineChart title="Availability" values={avail} {labels} format={pct} />
		</div>
	</section>

	<section class="rounded-2xl border bg-card">
		<h2 class="border-b px-4 py-2.5 text-md font-semibold">Recent downtimes</h2>
		{#if e.state === 'up' && e.id === 'e4'}
			<p class="px-4 py-3 text-md text-muted-foreground">No downtime in the last 90 days.</p>
		{:else}
			<ol class="divide-y text-md">
				{#each downtimes as d (d.start)}
					<li class="grid grid-cols-[1fr_auto] gap-3 px-4 py-2">
						<span class="tnum">{d.start} – {d.end}</span><span class="tnum text-muted-foreground"
							>{d.length}</span
						>
					</li>
				{/each}
			</ol>
		{/if}
	</section>
</div>
