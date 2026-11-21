<script lang="ts">
	// Idea A: groups fold open and shut (Uptime Kuma, Better Stack); a click on a service opens its detail in a side
	// panel, so the list stays where it was.
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { ChevronDown } from '@lucide/svelte';
	import IdeaDetail from '../../../IdeaDetail.svelte';
	import { entries, grouped, worst, type Entry } from '../../../ideas.ts';

	const pub = entries.filter((e) => e.status);
	const groups = grouped(pub);
	let span = $state('hour');
	let shut = $state<Record<string, boolean>>({});
	let open = $state<Entry | null>(null);
	const down = pub.filter((e) => e.state === 'down');
</script>

<svelte:head><title>Status · example.com (idea A)</title></svelte:head>
<div class="mx-auto max-w-4xl px-4 py-10">
	<h1 class="text-title font-semibold">example.com status</h1>
	<div
		class="mt-4 rounded-2xl border px-4 py-3 text-base font-medium {down.length
			? 'border-bad/30 bg-bad/10 text-bad-text'
			: 'border-ok/30 bg-ok/10 text-ok-text'}"
	>
		{down.length
			? `${down.map((e) => e.name).join(', ')} is down. Everything else works.`
			: 'Everything works.'}
	</div>
	<div class="mt-4 flex items-center justify-between gap-3">
		<p class="text-sm text-muted-foreground">
			Updated 13:42 · bars: {span === 'hour'
				? 'one per minute, last 60 minutes'
				: 'one per day, last 90 days'}
		</p>
		<ToggleGroup.Root type="single" bind:value={span} variant="outline" size="sm">
			<ToggleGroup.Item value="hour">Last hour</ToggleGroup.Item>
			<ToggleGroup.Item value="days">90 days</ToggleGroup.Item>
		</ToggleGroup.Root>
	</div>

	{#each groups as g (g.name)}
		{@const w = worst(g.items)}
		<section class="mt-4 rounded-2xl border bg-card">
			<button
				type="button"
				class="flex w-full items-center gap-3 px-4 py-3 text-left"
				aria-expanded={!shut[g.name]}
				onclick={() => (shut[g.name] = !shut[g.name])}
			>
				<ChevronDown class="size-4 transition-transform {shut[g.name] ? '-rotate-90' : ''}" />
				<h2 class="flex-1 text-base font-semibold">{g.name}</h2>
				<span class="tnum text-sm text-muted-foreground"
					>{g.items.filter((e) => e.state === 'up').length} of {g.items.length} up</span
				>
				<StatusBadge state={w} />
			</button>
			{#if !shut[g.name]}
				<ul class="divide-y border-t">
					{#each g.items as e (e.id)}
						<li>
							<button
								type="button"
								onclick={() => (open = e)}
								class="grid w-full gap-2 px-4 py-3 text-left hover:bg-surface-2/60 sm:grid-cols-[12rem_minmax(0,1fr)_7rem] sm:items-center"
							>
								<span class="flex items-center gap-2 text-md font-medium">
									<span
										class="grid size-6 place-items-center rounded-md text-xs font-semibold text-white"
										style="background:hsl({e.hue} 45% 42%)">{e.letter}</span
									>{e.name}
								</span>
								<HistoryStrip samples={span === 'hour' ? e.history : e.days} labels={false} />
								<span class="flex flex-col items-end gap-0.5 max-sm:items-start">
									<StatusBadge state={e.state} />
									<span class="tnum text-xs text-muted-foreground"
										>{span === 'hour'
											? e.uptime.h24 + '% 24 h'
											: e.uptime.d90.toFixed(2) + '% 90 d'}</span
									>
								</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/each}
</div>

<Sheet.Root open={!!open} onOpenChange={(v) => !v && (open = null)}>
	<Sheet.Content side="right" class="w-full overflow-y-auto p-5 sm:max-w-xl">
		<Sheet.Title class="sr-only">{open?.name}</Sheet.Title>
		{#if open}<IdeaDetail e={open} />{/if}
	</Sheet.Content>
</Sheet.Root>
