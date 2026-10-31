<script lang="ts">
	import { onMount } from 'svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import { api, type Schemas } from '$lib/api/client';
	import { stripHealth, uptimeText } from '$lib/monitor';
	import { dateTime, time } from '$lib/format';
	import { theme, type ThemeId } from '$lib/theme.svelte';

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
			theme.set(r.data.theme as ThemeId);
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
	const groups = $derived(Object.entries(Object.groupBy(data?.endpoints ?? [], (e) => e.group)));
	const down = $derived((data?.endpoints ?? []).filter((e) => e.state === 'down').length);
</script>

<svelte:head><title>Status · {data?.title ?? ''}</title></svelte:head>
<div class="mx-auto max-w-4xl px-4 py-10">
	<h1 class="text-[24px] font-semibold">{data?.title ?? ''} status</h1>
	{#if stale}
		<div
			role="alert"
			class="mt-4 rounded-2xl border border-warn/30 bg-warn/10 px-4 py-3 text-[14px] font-medium text-warn-text"
		>
			Data may be stale since {data?.updated_at
				? dateTime(data.updated_at)
				: 'the page was opened'}.
		</div>
	{:else if data}
		<div
			class="{down
				? 'border-bad/30 bg-bad/10 text-bad-text'
				: 'border-ok/30 bg-ok/10 text-ok-text'} mt-4 rounded-2xl border px-4 py-3 text-[14px] font-medium"
		>
			{down === 1
				? '1 service is down. Others are working.'
				: down
					? `${down} services are down. Others are working.`
					: 'All services are working.'}
		</div>
	{/if}
	{#if data?.updated_at}
		<p class="mt-2 text-[12.5px] text-muted-foreground">Updated {time(data.updated_at)}.</p>
	{/if}
	{#if data && data.endpoints.length === 0}
		<p class="mt-6 text-[13.5px] text-muted-foreground">Nothing is listed here yet.</p>
	{/if}
	{#each groups as [g, list] (g)}
		<section class="mt-6 rounded-2xl border bg-card">
			<h2 class="border-b px-4 py-2.5 text-[14px] font-semibold">{g}</h2>
			<ul class="divide-y">
				{#each list ?? [] as e (e.name)}
					<li class="grid gap-2 px-4 py-3 sm:grid-cols-[12rem_minmax(0,1fr)_7rem] sm:items-center">
						<span class="text-[13.5px] font-medium">{e.name}</span>
						<HistoryStrip samples={stripHealth(e.strip)} />
						<span class="flex items-center gap-2 sm:flex-col sm:items-end sm:gap-0">
							<StatusBadge state={e.state} />
							{#if e.pending_dns}<span class="text-[12px] text-muted-foreground">DNS updating</span
								>{/if}
							<span class="tnum text-[12px] text-muted-foreground">{uptimeText(e.uptime_24h)}</span>
						</span>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
</div>
