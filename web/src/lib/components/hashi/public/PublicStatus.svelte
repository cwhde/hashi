<script lang="ts">
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { onMount } from 'svelte';
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import { api, type Schemas } from '#lib/api/client.ts';
	import { publicSummary, stripHealth, uptimeText } from '#lib/monitor.ts';
	import { dateTime, time } from '#lib/format.ts';
	import { theme } from '#lib/theme.svelte.ts';

	// Public port: names, groups, state, 60-minute strips, uptime; never hosts, IPs or errors (07 §7).
	let data = $state<Schemas['PublicStatus'] | null>(null);
	let failedSince = $state<Date | null>(null);
	let now = $state(Date.now());

	async function load() {
		now = Date.now();
		try {
			const r = await api.GET('/api/public/status');
			if (!r.data) throw new Error('no data');
			data = r.data;
			failedSince = null;
			theme.apply(r.data);
		} catch {
			failedSince ??= new Date();
		}
	}
	onMount(() => {
		load();
		const t = setInterval(load, 30_000);
		return () => clearInterval(t);
	});

	const STALE_MS = 5 * 60_000;
	const stale = $derived(
		failedSince !== null ||
			(!!data?.updated_at && now - new Date(data.updated_at).getTime() > STALE_MS)
	);
	// The server sends the endpoints in display order; categories off (07 §7) means one list without headings.
	const groups = $derived(
		data?.grouped === false
			? data.endpoints.length
				? [['', data.endpoints] as const]
				: []
			: Object.entries(Object.groupBy(data?.endpoints ?? [], (e) => e.group))
	);
	const summary = $derived(data ? publicSummary(data.endpoints.map((e) => e.state)) : null);
	const tones = {
		ok: 'border-ok/30 bg-ok/10 text-ok-text',
		warn: 'border-warn/30 bg-warn/10 text-warn-text',
		bad: 'border-bad/30 bg-bad/10 text-bad-text'
	};
</script>

<svelte:head><title>Status · {data?.title ?? ''}</title></svelte:head>
<div class="mx-auto max-w-4xl px-4 py-10">
	<h1 class="text-title font-semibold">{data?.title ?? ''} status</h1>
	{#if stale}
		<div
			role="alert"
			class="mt-4 rounded-2xl border border-warn/30 bg-warn/10 px-4 py-3 text-base font-medium text-warn-text"
		>
			Data may be stale since {data?.updated_at
				? dateTime(data.updated_at)
				: 'the page was opened'}.
		</div>
	{:else if summary}
		<!-- The update time sits in the banner, so it belongs to it and not to the first card (bug 145). -->
		<div
			class="{tones[
				summary.tone
			]} mt-4 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 rounded-2xl border px-4 py-3"
		>
			<span class="text-base font-medium">{summary.text}</span>
			{#if data?.updated_at}<span class="text-sm text-muted-foreground"
					>Updated {time(data.updated_at)}</span
				>{/if}
		</div>
	{/if}
	{#if !data && !stale}
		<Skeleton class="mt-4 h-12 rounded-2xl" />
		<div class="mt-6 space-y-3">
			{#each [0, 1, 2, 3, 4] as i (i)}<Skeleton class="h-16 rounded-2xl" />{/each}
		</div>
	{/if}
	{#if data && data.endpoints.length === 0}
		<p class="mt-6 text-md text-muted-foreground">Nothing is listed here yet.</p>
	{/if}
	{#each groups as [g, list] (g)}
		<section class="mt-6 rounded-2xl border bg-card">
			{#if g}<h2 class="border-b px-4 py-2.5 text-base font-semibold">{g}</h2>{/if}
			<ul class="divide-y">
				{#each list ?? [] as e, i (i)}
					<li class="grid gap-2 px-4 py-3 sm:grid-cols-[12rem_minmax(0,1fr)_7rem] sm:items-center">
						<span class="text-md font-medium">{e.name}</span>
						<HistoryStrip samples={stripHealth(e.strip)} />
						<span class="flex items-center gap-2 sm:flex-col sm:items-end sm:gap-0">
							<StatusBadge state={e.state} />
							{#if e.pending_dns}<span class="text-xs text-muted-foreground">DNS updating</span
								>{/if}
							<span class="tnum text-xs text-muted-foreground">{uptimeText(e.uptime_24h)}</span>
						</span>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
</div>
