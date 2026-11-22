<script lang="ts">
	// The public status page (07 §7.1, approved mock design/public-status): group tiles on top, each group in a panel
	// that folds, dense rows with the app icon, 90 days by default with a Last hour switch. Pointing at a bar says what
	// happened then; a row opens the service page. Public fields only: names, states, history, uptime.
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { onMount } from 'svelte';
	import { ChevronDown } from '@lucide/svelte';
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import DayStrip from '#lib/components/hashi/DayStrip.svelte';
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import PublicHeader from './PublicHeader.svelte';
	import PublicState from './PublicState.svelte';
	import { api, type Schemas } from '#lib/api/client.ts';
	import type { Health } from '#lib/design/mock.ts';
	import { publicSummary, uptimeText } from '#lib/monitor.ts';
	import { dayDetail, minuteDetail, minuteHealth, when, worst } from '#lib/public.ts';
	import { theme } from '#lib/theme.svelte.ts';

	type Entry = Schemas['PublicStatusEntry'];
	let data = $state<Schemas['PublicStatus'] | null>(null);
	let failedSince = $state<Date | null>(null);
	let now = $state(Date.now());
	let span = $state('days');

	// Folded groups are the visitor's own (this browser only); a group with trouble opens again on the next visit.
	const FOLD_KEY = 'hashi.status.folded';
	let folded = $state<Record<string, boolean>>(read());
	function read(): Record<string, boolean> {
		try {
			return JSON.parse(localStorage.getItem(FOLD_KEY) ?? '{}') ?? {};
		} catch {
			return {};
		}
	}
	// A group being folded or unfolded clips its rows for the animation; open and at rest it clips nothing, so the bar
	// details of its first row can show above the row.
	let moving = $state<Record<string, boolean>>({});
	function fold(name: string, value: boolean) {
		if (!matchMedia('(prefers-reduced-motion: reduce)').matches) moving[name] = true;
		folded[name] = value;
		try {
			localStorage.setItem(FOLD_KEY, JSON.stringify(folded));
		} catch {
			// private window or blocked storage: folding still works for this visit
		}
	}

	let first = true;
	async function load() {
		now = Date.now();
		try {
			const r = await api.GET('/api/public/status');
			if (!r.data) throw new Error('no data');
			if (first) {
				first = false;
				for (const g of r.data.groups)
					if (worst(g.entries.map((e) => e.state)) !== 'up') delete folded[g.name];
			}
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
	const all = $derived(data?.groups.flatMap((g) => g.entries) ?? []);
	const summary = $derived(data ? publicSummary(all.map((e) => e.state)) : null);
	const tones = {
		ok: 'border-ok/30 bg-ok/10 text-ok-text',
		warn: 'border-warn/30 bg-warn/10 text-warn-text',
		bad: 'border-bad/30 bg-bad/10 text-bad-text'
	};
	const up = (list: Entry[]) => list.filter((e) => e.state === 'up').length;
	const days = (e: Entry) => e.days.map((d) => d.state as Health);
</script>

<svelte:head><title>Status · {data?.title ?? ''}</title></svelte:head>
<main class="mx-auto max-w-4xl px-4 py-10">
	<PublicHeader title={data?.title ?? ''} assets={data?.assets} suffix="status" />
	{#if stale}
		<div
			role="alert"
			class="mt-4 rounded-2xl border border-warn/30 bg-warn/10 px-4 py-3 text-base font-medium text-warn-text"
		>
			Data may be stale since {data?.updated_at ? when(data.updated_at) : 'the page was opened'}.
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
					>Updated {when(data.updated_at)}</span
				>{/if}
		</div>
	{/if}
	{#if !data && !stale}
		<Skeleton class="mt-4 h-12 rounded-2xl" />
		<div class="mt-6 space-y-3">
			{#each [0, 1, 2, 3, 4] as i (i)}<Skeleton class="h-16 rounded-2xl" />{/each}
		</div>
	{/if}
	{#if data && all.length === 0}
		<p class="mt-6 text-md text-muted-foreground">Nothing is listed here yet.</p>
	{/if}

	{#if data && data.groups.length > 1}
		<!-- The groups at a glance: a summary, not controls (the owner, 2026-11-22). -->
		<ul
			aria-label="Groups"
			class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-[repeat(auto-fit,minmax(8.5rem,1fr))]"
		>
			{#each data.groups as g (g.name)}
				{@const w = worst(g.entries.map((e) => e.state))}
				<li
					class="rounded-xl border bg-card px-3 py-2.5 {w === 'down'
						? 'border-bad/40'
						: w === 'degraded'
							? 'border-warn/40'
							: ''}"
				>
					<span class="flex items-center justify-between gap-2 text-md font-medium"
						><span class="truncate">{g.name}</span><StatusDot state={w} /></span
					>
					<span class="tnum mt-0.5 block text-sm text-muted-foreground"
						>{up(g.entries)} of {g.entries.length} up</span
					>
				</li>
			{/each}
		</ul>
	{/if}

	{#if all.length}
		<div class="mt-6 flex justify-end">
			<ToggleGroup.Root
				type="single"
				value={span}
				onValueChange={(v) => v && (span = v)}
				variant="outline"
				size="sm"
			>
				<ToggleGroup.Item value="days">90 days</ToggleGroup.Item>
				<ToggleGroup.Item value="hour">Last hour</ToggleGroup.Item>
			</ToggleGroup.Root>
		</div>
	{/if}

	{#each data?.groups ?? [] as g (g.name)}
		{@const o = !folded[g.name]}
		<section class="mt-3 rounded-2xl border bg-card">
			<button
				type="button"
				class="flex w-full items-center gap-3 px-4 py-2.5 text-left"
				aria-expanded={o}
				onclick={() => fold(g.name, o)}
			>
				<ChevronDown
					class="size-4 shrink-0 text-muted-foreground transition-transform duration-200 ease-out motion-reduce:transition-none {o
						? ''
						: '-rotate-90'}"
				/>
				<h2 class="flex-1 text-base font-semibold">{g.name}</h2>
				<span class="tnum text-sm text-muted-foreground"
					>{up(g.entries)} of {g.entries.length} up</span
				>
				<StatusDot state={worst(g.entries.map((e) => e.state))} />
			</button>
			<!-- Folding animates the height through the grid row (0fr ↔ 1fr), so no row is measured and any length folds
			     smoothly; folded rows are inert, out of the tab order and hidden from screen readers. -->
			<div
				class="grid transition-[grid-template-rows] duration-200 ease-out motion-reduce:transition-none {o
					? 'grid-rows-[1fr]'
					: 'grid-rows-[0fr]'}"
				inert={!o}
				ontransitionend={(ev) => ev.target === ev.currentTarget && (moving[g.name] = false)}
			>
				<div class="min-h-0 {!o || moving[g.name] ? 'overflow-hidden' : ''}">
					<ul class="divide-y border-t">
						{#each g.entries as e (e.slug)}
							<li>
								<a
									href="/s/{e.slug}"
									class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1.5 px-4 py-2 hover:bg-surface-2/60 sm:grid-cols-[11rem_minmax(0,1fr)_4.5rem_4.5rem]"
								>
									<span class="flex min-w-0 items-center gap-2 text-md font-medium">
										<AppIcon
											name={e.name}
											icon={e.icon}
											light={e.icon_light}
											class="size-6 rounded-md text-xs"
										/>
										<span class="truncate">{e.name}</span>
										{#if e.pending_dns}<span
												class="shrink-0 text-xs font-normal text-muted-foreground"
												>DNS updating</span
											>{/if}
									</span>
									<!-- Word then dot, the dot on the same right edge as the group's (07 §7.1). -->
									<PublicState state={e.state} class="sm:order-last" />
									{#if span === 'days'}
										<DayStrip
											samples={days(e)}
											detail={(i) => dayDetail(e.days[i]!, i === e.days.length - 1, now)}
											labels={false}
											class="col-span-2 sm:col-span-1"
										/>
									{:else}
										<DayStrip
											samples={minuteHealth(e.minutes)}
											detail={(i) => minuteDetail(e.minutes, i)}
											labels={false}
											unit="check"
											class="col-span-2 sm:col-span-1"
										/>
									{/if}
									<span class="tnum hidden text-sm text-muted-foreground sm:block sm:text-right"
										>{uptimeText(span === 'days' ? e.uptime_90d : e.uptime_24h)}</span
									>
								</a>
							</li>
						{/each}
					</ul>
				</div>
			</div>
		</section>
	{/each}
</main>
