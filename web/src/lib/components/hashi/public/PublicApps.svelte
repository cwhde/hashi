<script lang="ts">
	import { onMount } from 'svelte';
	import StatusDot from '$lib/components/hashi/StatusDot.svelte';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import { Search } from '@lucide/svelte';
	import { api, type Schemas } from '$lib/api/client';
	import { stateOrder } from '$lib/monitor';
	import { theme, type ThemeId } from '$lib/theme.svelte';

	// Public port: only names, links and state, no admin controls (07 §8).
	let data = $state<Schemas['PublicApps'] | null>(null);
	let failed = $state(false);
	let searching = $state(false);
	let query = $state('');
	let sort = $state('group');

	async function load() {
		try {
			const r = await api.GET('/api/public/apps');
			if (!r.data) throw new Error('no data');
			data = r.data;
			failed = false;
			theme.set(r.data.theme as ThemeId);
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
		<div class="min-w-48 flex-1">
			<h1 class="text-[24px] font-semibold">{data?.title ?? ''}</h1>
			{#if data}
				<p class="text-[13.5px] text-muted-foreground">
					{data.hosts_online} / {data.hosts_total} hosts online · {data.firewall_online} / {data.firewall_total}
					Linux firewall hosts available
				</p>
			{/if}
		</div>
		{#if searching}
			<Input bind:value={query} autofocus placeholder="Search" aria-label="Search" class="w-48" />
		{:else}
			<button
				onclick={() => (searching = true)}
				class="flex h-9 items-center gap-2 rounded-[10px] border bg-card px-3 text-[13px] text-muted-foreground"
				><Search class="size-4" />Search</button
			>
		{/if}
		<Select.Root type="single" bind:value={sort}>
			<Select.Trigger class="w-36" aria-label="Sort">Sort: {sortNames[sort]}</Select.Trigger>
			<Select.Content>
				{#each Object.entries(sortNames) as [v, l] (v)}<Select.Item value={v}>{l}</Select.Item
					>{/each}
			</Select.Content>
		</Select.Root>
	</header>
	{#if failed}
		<p role="alert" class="mb-4 text-[13.5px] text-warn-text">
			Could not refresh. What you see may be out of date.
		</p>
	{/if}
	{#if data && data.apps.length === 0}
		<p class="text-[13.5px] text-muted-foreground">No apps are listed here yet.</p>
	{/if}
	{#each groups as [g, list] (g)}
		<section class="mb-8">
			{#if g}<h2 class="mb-3 text-[13px] font-medium text-muted-foreground">{g}</h2>{/if}
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
				{#each list ?? [] as a (a.url)}
					<a
						href={a.url}
						class="flex items-center gap-3 rounded-2xl border bg-card p-3.5 transition-colors hover:border-muted-foreground/40"
					>
						<span
							class="grid size-10 shrink-0 place-items-center rounded-xl border bg-surface-2 text-[15px] font-semibold"
							>{a.name[0]}</span
						>
						<span class="min-w-0 flex-1"
							><span class="block truncate text-[14px] font-medium">{a.name}</span><span
								class="block truncate text-[12.5px] text-muted-foreground">{domain(a.url)}</span
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
