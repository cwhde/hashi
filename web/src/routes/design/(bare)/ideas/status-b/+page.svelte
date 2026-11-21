<script lang="ts">
	// Idea B: one tile per group on top (where is the trouble?), then compact rows with a 90-day bar (UptimeRobot,
	// OpenStatus); a click opens the service's own page with a link you can share. When the browser is signed in to
	// Hashi, one "Edit layout" link appears (the toggle only shows both views in this mock).
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { Pencil } from '@lucide/svelte';
	import { entries, grouped, worst } from '../../../ideas.ts';

	const pub = entries.filter((e) => e.status);
	const groups = grouped(pub);
	let admin = $state(false);
</script>

<svelte:head><title>Status · example.com (idea B)</title></svelte:head>
<div class="mx-auto max-w-5xl px-4 py-10">
	<header class="flex flex-wrap items-center gap-3">
		<h1 class="flex-1 text-title font-semibold">example.com status</h1>
		<label
			class="flex items-center gap-2 rounded-full border border-dashed px-3 py-1 text-xs text-muted-foreground"
			><Switch bind:checked={admin} />mock: signed in to Hashi</label
		>
		{#if admin}<a
				href="/design/ideas/layout-a"
				class="inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-md hover:bg-surface-2"
				><Pencil class="size-4" />Edit layout</a
			>{/if}
	</header>

	<div class="mt-5 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
		{#each groups as g (g.name)}
			{@const w = worst(g.items)}
			<button
				type="button"
				onclick={() =>
					document.getElementById(`g-${g.name}`)?.scrollIntoView({ behavior: 'smooth' })}
				class="rounded-2xl border bg-card p-3 text-left hover:border-muted-foreground/40 {w ===
				'down'
					? 'border-bad/40'
					: w === 'degraded'
						? 'border-warn/40'
						: ''}"
			>
				<span class="flex items-center gap-2 text-md font-medium"
					><StatusDot state={w} />{g.name}</span
				>
				<span class="tnum mt-1 block text-sm text-muted-foreground"
					>{g.items.filter((e) => e.state === 'up').length} of {g.items.length} up</span
				>
			</button>
		{/each}
	</div>

	{#each groups as g (g.name)}
		<section id="g-{g.name}" class="mt-8">
			<h2 class="mb-2 text-base font-semibold">{g.name}</h2>
			<ul class="divide-y rounded-2xl border bg-card">
				{#each g.items as e (e.id)}
					<li>
						<a
							href="/design/ideas/status-b/{e.id}"
							class="grid gap-2 px-4 py-3 hover:bg-surface-2/60 sm:grid-cols-[11rem_minmax(0,1fr)_5rem_6.5rem] sm:items-center"
						>
							<span class="text-md font-medium">{e.name}</span>
							<HistoryStrip samples={e.days} labels={false} />
							<span class="tnum text-sm text-muted-foreground sm:text-right"
								>{e.uptime.d90.toFixed(2)}%</span
							>
							<span class="sm:text-right"><StatusBadge state={e.state} /></span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
	<p class="mt-4 text-sm text-muted-foreground">Bars: one per day, last 90 days.</p>
</div>
