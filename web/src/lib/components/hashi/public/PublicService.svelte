<script lang="ts">
	// One service on the status listener, with an address to share (07 §7.2, approved mock design/public-status/s).
	// Public fields only: state, uptime, response times, downtimes with times and lengths. Three charts that show the
	// value under the pointer; the 90-day strip says what happened on each day; one Badges button holds every badge.
	import { every } from '#lib/poll.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { ArrowLeft } from '@lucide/svelte';
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import DayStrip from '#lib/components/hashi/DayStrip.svelte';
	import LineChart from '#lib/components/hashi/LineChart.svelte';
	import BadgesButton from './BadgesButton.svelte';
	import PublicState from './PublicState.svelte';
	import PublicHeader from './PublicHeader.svelte';
	import { api, type Schemas } from '#lib/api/client.ts';
	import type { Health } from '#lib/design/mock.ts';
	import { uptimeText } from '#lib/monitor.ts';
	import { dayDetail, downtimeRow, sinceTile } from '#lib/public.ts';
	import { theme } from '#lib/theme.svelte.ts';
	import { ChartSync } from '#lib/chart-sync.svelte.ts';

	let { slug }: { slug: string } = $props();
	let data = $state<Schemas['PublicService'] | null>(null);
	let missing = $state(false);
	let failed = $state(false);
	// The range is in the address (?range=7d), so a shared link or a reload shows the same one; 24 h leaves none.
	const ranges = ['24h', '7d', '30d'] as const;
	type Range = (typeof ranges)[number];
	const asked = page.url.searchParams.get('range') as Range | null;
	let range = $state<Range>(asked && ranges.includes(asked) ? asked : '24h');
	function pick(v: Range) {
		range = v;
		const u = new URL(page.url.href);
		if (v === '24h') u.searchParams.delete('range');
		else u.searchParams.set('range', v);
		replaceState(u, {});
	}
	let now = $state(Date.now());

	async function load() {
		now = Date.now();
		const want = range;
		try {
			const r = await api.GET('/api/public/status/{slug}', {
				params: { path: { slug }, query: { range: want } }
			});
			// An answer for a range no longer chosen is dropped: it came back after the newer one (bug 166).
			if (want !== range) return;
			if (r.response.status === 404) {
				missing = true;
				return;
			}
			if (!r.data) throw new Error('no data');
			data = r.data;
			missing = failed = false;
			theme.apply(r.data);
		} catch {
			failed = true;
		}
	}
	onMount(() => every(30_000, load));
	// Every range change loads again (and the first run is the first load).
	$effect(() => {
		void range;
		load();
	});

	const e = $derived(data?.entry);
	const label = (iso: string) =>
		new Date(iso).toLocaleString('en-GB', {
			day: 'numeric',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		});
	const labels = $derived(data?.series.map((p) => label(p.at)) ?? []);
	// The three charts share one pointer: the same moment shows in each of them.
	const sync = new ChartSync();
	const times = $derived(data?.series.map((p) => Date.parse(p.at)) ?? []);
	const pct = (v: number) => `${v.toFixed(v === 100 ? 0 : 2)}%`;
	const tiles = $derived(
		e && data
			? [
					sinceTile(e.state, e.since, now),
					['Uptime 24 h', uptimeText(e.uptime_24h)],
					['Uptime 7 d', uptimeText(data.uptime_7d)],
					['Uptime 30 d', uptimeText(data.uptime_30d)],
					['Response 24 h', data.avg_ms_24h === undefined ? 'no data' : `${data.avg_ms_24h} ms`]
				]
			: []
	);
	const title = $derived(data?.title ?? '');
</script>

<svelte:head><title>{e ? `${e.name} · ` : ''}Status · {title}</title></svelte:head>
<main class="mx-auto max-w-5xl space-y-5 px-4 py-10">
	<a
		href="/"
		class="inline-flex items-center gap-1.5 text-md text-muted-foreground hover:text-foreground"
		><ArrowLeft class="size-4" />{title ? `${title} status` : 'Status'}</a
	>
	<PublicHeader {title} assets={data?.assets} heading={false} />
	{#if missing}
		<p class="text-md text-muted-foreground">This service is not on the status page.</p>
	{:else if !data}
		{#if failed}
			<p role="alert" class="text-md text-warn-text">Could not load this service.</p>
		{:else}
			<Skeleton class="h-12 rounded-2xl" />
			<Skeleton class="h-16 rounded-2xl" />
			<Skeleton class="h-40 rounded-2xl" />
		{/if}
	{:else if e}
		{#if failed}
			<p role="alert" class="text-md text-warn-text">Could not refresh.</p>
		{/if}
		<header class="flex flex-wrap items-center gap-3">
			<AppIcon
				name={e.name}
				icon={e.mark ? data.assets?.icon : e.icon}
				light={e.icon_light}
				mark={e.mark}
				class="size-11 rounded-xl text-lg"
			/>
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-title font-semibold">{e.name}</h1>
				<p class="text-md text-muted-foreground">{data.group}</p>
			</div>
			<PublicState state={e.state} />
			{#if data.badges.length}<BadgesButton name={e.name} slug={e.slug} kinds={data.badges} />{/if}
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
			<DayStrip
				samples={e.days.map((d) => d.state as Health)}
				detail={(i) => dayDetail(e.days[i]!, i === e.days.length - 1, now)}
			/>
		</section>

		<section>
			<div class="mb-2 flex justify-end">
				<ToggleGroup.Root
					type="single"
					value={range}
					onValueChange={(v) => v && pick(v as Range)}
					variant="outline"
					size="sm"
				>
					{#each ['24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}>{r}</ToggleGroup.Item
						>{/each}
				</ToggleGroup.Root>
			</div>
			{#if data.series.length}
				<div class="grid gap-4 lg:grid-cols-3">
					<LineChart
						title="Response time"
						values={data.series.map((p) => p.avg_ms)}
						unit=" ms"
						{labels}
						{times}
						{sync}
					/>
					<LineChart
						title="Slowest responses"
						values={data.series.map((p) => p.p95_ms)}
						unit=" ms"
						{labels}
						{times}
						{sync}
					/>
					<LineChart
						title="Availability"
						values={data.series.map((p) => p.availability)}
						{labels}
						{times}
						{sync}
						format={pct}
					/>
				</div>
			{:else}
				<p class="rounded-2xl border bg-card px-4 py-3 text-md text-muted-foreground">
					No checks in this range.
				</p>
			{/if}
		</section>

		<section class="rounded-2xl border bg-card">
			<h2 class="border-b px-4 py-2.5 text-md font-semibold">Recent downtimes</h2>
			{#if data.downtimes.length === 0}
				<p class="px-4 py-3 text-md text-muted-foreground">No downtime in the last 90 days.</p>
			{:else}
				<ol class="divide-y text-md">
					{#each data.downtimes as d (d.start)}
						{@const row = downtimeRow(d, now)}
						<li class="grid grid-cols-[1fr_auto] gap-3 px-4 py-2">
							<span class="tnum">{row.start} – {row.end}</span><span
								class="tnum text-muted-foreground">{row.length}</span
							>
						</li>
					{/each}
				</ol>
			{/if}
		</section>
	{/if}
</main>
