<script lang="ts">
	// Approved mock (07 §8, 2026-11-22): the apps page in the shared layout's order. Groups as headings in the
	// owner's order, cards in the owner's order inside each; search always shown; sort: group, name or status.
	// No up/down counts (bug 147), no admin controls.
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import { Input } from '#lib/components/ui/input/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import { Search } from '@lucide/svelte';
	import { entries, grouped } from '../../ideas.ts';

	const apps = entries.filter((e) => e.apps);
	let q = $state('');
	let sort = $state('order');
	const sorts = { order: 'Group', name: 'Name', status: 'Status' } as const;
	const rank = { down: 0, degraded: 1, up: 2, unknown: 3, paused: 4 } as const;
	const domain = (name: string) => `${name.toLowerCase().replace(/[^a-z0-9]+/g, '')}.example.com`;
	const shown = $derived.by(() => {
		const hit = apps.filter((e) => e.name.toLowerCase().includes(q.trim().toLowerCase()));
		if (sort === 'order') return grouped(hit);
		const list = [...hit].sort((a, b) =>
			sort === 'name'
				? a.name.localeCompare(b.name)
				: rank[a.state] - rank[b.state] || a.name.localeCompare(b.name)
		);
		return [{ name: '', items: list }];
	});
</script>

<svelte:head><title>Apps · example.com</title></svelte:head>
<div class="mx-auto max-w-6xl px-4 py-10">
	<header class="mb-8 flex flex-wrap items-center gap-3">
		<h1 class="flex-1 text-title font-semibold">example.com</h1>
		<label class="relative w-full sm:w-64">
			<span class="sr-only">Search apps</span>
			<Search
				class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
			/>
			<Input bind:value={q} placeholder="Search" class="pl-8" />
		</label>
		<Select.Root type="single" bind:value={sort}>
			<Select.Trigger class="w-36" aria-label="Sort"
				>Sort: {sorts[sort as keyof typeof sorts]}</Select.Trigger
			>
			<Select.Content>
				{#each Object.entries(sorts) as [v, l] (v)}<Select.Item value={v}>{l}</Select.Item>{/each}
			</Select.Content>
		</Select.Root>
	</header>
	{#each shown as g (g.name)}
		<section class="mb-8">
			{#if g.name}<h2 class="mb-3 text-md font-medium text-muted-foreground">{g.name}</h2>{/if}
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
				{#each g.items as a (a.id)}
					<a
						href="https://{domain(a.name)}"
						class="flex items-center gap-3 rounded-2xl border bg-card p-3.5 transition-colors hover:border-muted-foreground/40"
					>
						<AppIcon name={a.name} class="size-10 rounded-xl text-lg" />
						<span class="min-w-0 flex-1"
							><span class="block truncate text-base font-medium">{a.name}</span><span
								class="block truncate text-sm text-muted-foreground">{domain(a.name)}</span
							></span
						>
						{#if a.check}<StatusDot state={a.state} />{/if}
					</a>
				{/each}
			</div>
		</section>
	{:else}
		<p class="text-md text-muted-foreground">No app matches “{q}”.</p>
	{/each}
</div>
