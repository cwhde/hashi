<script lang="ts">
	// Approved mock (07 §7.1, 2026-11-22): status page from the owner's picks of 2026-11-21. Group tiles on top
	// (idea B), each group in a panel that folds (idea A), dense rows with the app icon, 90 days by default. Hovering a
	// bar says what happened then; a row opens the service page. Always grouped, always with icons (owner, 2026-11-22).
	// Public fields only: no targets, ports, protocols, codes or error text.
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import DayStrip from '../../DayStrip.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { ChevronDown } from '@lucide/svelte';
	import { dayDetail, entries, grouped, minuteDetail, slug, worst } from '../../ideas.ts';

	const pub = entries.filter((e) => e.status);
	const groups = grouped(pub);
	const down = pub.filter((e) => e.state === 'down');
	const slow = pub.filter((e) => e.state === 'degraded');
	let span = $state('days');
	// A group with trouble starts open; the rest as the visitor left them (open the first time).
	let open = $state<Record<string, boolean>>({});
	const isOpen = (name: string) => open[name] ?? true;
	function jump(name: string) {
		open[name] = true;
		document.getElementById(`g-${name}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}
	const up = (list: typeof pub) => list.filter((e) => e.state === 'up').length;
</script>

<svelte:head><title>Status · example.com</title></svelte:head>
<div class="mx-auto max-w-4xl px-4 py-10">
	<h1 class="text-title font-semibold">example.com status</h1>
	<div
		class="mt-4 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 rounded-2xl border px-4 py-3 {down.length
			? 'border-bad/30 bg-bad/10 text-bad-text'
			: slow.length
				? 'border-warn/30 bg-warn/10 text-warn-text'
				: 'border-ok/30 bg-ok/10 text-ok-text'}"
	>
		<span class="text-base font-medium"
			>{down.length
				? `${down.length} ${down.length === 1 ? 'service is' : 'services are'} down.`
				: slow.length
					? `${slow.length} slow.`
					: 'Everything works.'}{down.length && slow.length ? ` ${slow.length} slow.` : ''}</span
		>
		<span class="text-sm text-muted-foreground">Updated 13:42</span>
	</div>

	{#if groups.length > 1}
		<nav
			aria-label="Groups"
			class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-[repeat(auto-fit,minmax(8.5rem,1fr))]"
		>
			{#each groups as g (g.name)}
				{@const w = worst(g.items)}
				<button
					type="button"
					onclick={() => jump(g.name)}
					class="rounded-xl border bg-card px-3 py-2.5 text-left transition-colors hover:border-muted-foreground/40 {w ===
					'down'
						? 'border-bad/40'
						: w === 'degraded'
							? 'border-warn/40'
							: ''}"
				>
					<span class="flex items-center gap-2 text-md font-medium"
						><StatusDot state={w} /><span class="truncate">{g.name}</span></span
					>
					<span class="tnum mt-0.5 block text-sm text-muted-foreground"
						>{up(g.items)} of {g.items.length} up</span
					>
				</button>
			{/each}
		</nav>
	{/if}

	<div class="mt-6 flex justify-end">
		<ToggleGroup.Root type="single" bind:value={span} variant="outline" size="sm">
			<ToggleGroup.Item value="days">90 days</ToggleGroup.Item>
			<ToggleGroup.Item value="hour">Last hour</ToggleGroup.Item>
		</ToggleGroup.Root>
	</div>

	{#each groups as g (g.name)}
		{@const o = isOpen(g.name)}
		<section id="g-{g.name}" class="mt-3 scroll-mt-4 rounded-2xl border bg-card">
			<button
				type="button"
				class="flex w-full items-center gap-3 px-4 py-2.5 text-left"
				aria-expanded={o}
				onclick={() => (open[g.name] = !o)}
			>
				<ChevronDown
					class="size-4 shrink-0 text-muted-foreground transition-transform {o ? '' : '-rotate-90'}"
				/>
				<h2 class="flex-1 text-base font-semibold">{g.name}</h2>
				<span class="tnum text-sm text-muted-foreground">{up(g.items)} of {g.items.length} up</span>
				<StatusDot state={worst(g.items)} />
			</button>
			{#if o}
				<ul class="divide-y border-t">
					{#each g.items as e (e.id)}
						<li>
							<a
								href="/design/public-status/s/{slug(e.name)}"
								class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1.5 px-4 py-2 hover:bg-surface-2/60 sm:grid-cols-[11rem_minmax(0,1fr)_4.5rem_5.5rem]"
							>
								<span class="flex min-w-0 items-center gap-2 text-md font-medium">
									<AppIcon name={e.name} class="size-6 rounded-md text-xs" />
									<span class="truncate">{e.name}</span>
								</span>
								<span class="sm:order-last sm:text-right"><StatusBadge state={e.state} /></span>
								<DayStrip
									samples={span === 'days' ? e.days : e.history}
									detail={(i) => (span === 'days' ? dayDetail(e, i) : minuteDetail(e, i))}
									labels={false}
									class="col-span-2 sm:col-span-1"
								/>
								<span class="tnum hidden text-sm text-muted-foreground sm:block sm:text-right"
									>{span === 'days' ? e.uptime.d90.toFixed(2) : e.uptime.h24}%</span
								>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/each}
</div>
