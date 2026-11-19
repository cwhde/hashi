<script lang="ts">
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { onMount } from 'svelte';
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import { Input } from '#lib/components/ui/input/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import { Search } from '@lucide/svelte';
	import { api, type Schemas } from '#lib/api/client.ts';
	import { stateOrder } from '#lib/monitor.ts';
	import { theme } from '#lib/theme.svelte.ts';

	// Public port: only names, links and state, no admin controls (07 §8).
	let data = $state<Schemas['PublicApps'] | null>(null);
	let failed = $state(false);
	let query = $state('');
	let sort = $state('group');
	let sortChosen = false; // the visitor's choice wins over the default from the Dashboard settings

	async function load() {
		try {
			const r = await api.GET('/api/public/apps');
			if (!r.data) throw new Error('no data');
			data = r.data;
			failed = false;
			if (!sortChosen) sort = r.data.default_sort;
			theme.apply(r.data);
		} catch {
			failed = true;
		}
	}
	onMount(() => {
		load();
		const t = setInterval(load, 30_000);
		return () => clearInterval(t);
	});

	const shown = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const apps = (data?.apps ?? []).filter((a) => !q || a.name.toLowerCase().includes(q));
		if (sort === 'status')
			return [...apps].sort((a, b) => (stateOrder[a.state] ?? 9) - (stateOrder[b.state] ?? 9));
		return apps;
	});
	const groups = $derived(
		sort === 'group'
			? Object.entries(Object.groupBy(shown, (a) => a.group))
			: [['', shown] as const]
	);
	const domain = (url: string) => url.replace(/^https?:\/\//, '').replace(/\/$/, '');
	const sortNames: Record<string, string> = { group: 'Group', name: 'Name', status: 'Status' };
</script>

<svelte:head><title>Apps · {data?.title ?? ''}</title></svelte:head>
<div class="mx-auto max-w-6xl px-4 py-10">
	<header class="mb-8 flex flex-wrap items-end gap-3">
		<!-- Up/down counts are the status page's job, not the launcher's (bug 147). -->
		<h1 class="min-w-48 flex-1 text-title font-semibold">{data?.title ?? ''}</h1>
		<!-- Always the plain field: the button that turned into a field looked broken in Firefox until clicked (bug 146). -->
		<!-- Search and sort stay together: one row under the title on a phone (bug 147). -->
		<div class="flex w-full gap-3 sm:w-auto">
			<div class="relative min-w-0 flex-1 sm:w-48 sm:flex-none">
				<Search
					class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
				/>
				<Input bind:value={query} placeholder="Search" aria-label="Search" class="pl-8" />
			</div>
			<Select.Root type="single" bind:value={sort} onValueChange={() => (sortChosen = true)}>
				<Select.Trigger class="w-36 shrink-0" aria-label="Sort"
					>Sort: {sortNames[sort]}</Select.Trigger
				>
				<Select.Content>
					{#each Object.entries(sortNames) as [v, l] (v)}<Select.Item value={v}>{l}</Select.Item
						>{/each}
				</Select.Content>
			</Select.Root>
		</div>
	</header>
	{#if failed}
		<p role="alert" class="mb-4 text-md text-warn-text">
			Could not refresh. What you see may be out of date.
		</p>
	{/if}
	{#if !data && !failed}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
			{#each [0, 1, 2, 3, 4, 5, 6, 7] as i (i)}<Skeleton class="h-20 rounded-2xl" />{/each}
		</div>
	{/if}
	{#if data && data.apps.length === 0}
		<p class="text-md text-muted-foreground">No apps are listed here yet.</p>
	{/if}
	{#each groups as [g, list] (g)}
		<section class="mb-8">
			{#if g}<h2 class="mb-3 text-md font-medium text-muted-foreground">{g}</h2>{/if}
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
				{#each list ?? [] as a (a.name + a.url)}
					<a
						href={a.url}
						class="flex items-center gap-3 rounded-2xl border bg-card p-3.5 transition-colors hover:border-muted-foreground/40"
					>
						<span
							class="grid size-10 shrink-0 place-items-center rounded-xl border bg-surface-2 text-lg font-semibold"
							>{a.name[0]}</span
						>
						<span class="min-w-0 flex-1"
							><span class="block truncate text-base font-medium">{a.name}</span><span
								class="block truncate text-sm text-muted-foreground">{domain(a.url)}</span
							></span
						>
						<StatusDot state={a.state} />
						<span class="sr-only">{a.state}</span>
					</a>
				{/each}
			</div>
		</section>
	{/each}
</div>
