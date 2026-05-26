<script lang="ts">
	import { api } from '$lib/api/client';
	import type { Resource } from '$lib/api/types';
	import { Input } from '$lib/components/ui/input';
	import { Search } from 'lucide-svelte';

	let apps = $state<Resource[]>([]);
	let search = $state('');
	let sort = $state('name');
	let loading = $state(true);

	$effect(() => {
		void (async () => {
			try {
				apps = await api.getPublicApps();
			} catch {
				apps = [];
			} finally {
				loading = false;
			}
		})();
	});

	const filtered = $derived(
		apps
			.filter((a) => a.name.toLowerCase().includes(search.toLowerCase()))
			.sort((a, b) => a.name.localeCompare(b.name))
	);

	const online = $derived(apps.filter((a) => a.enabled).length);
</script>

<section class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<h1 class="text-xl font-semibold text-white">Homelab Dashboard</h1>
			<p class="text-sm text-muted-foreground">Public service tiles on port 8081.</p>
		</div>
		<div class="flex items-center gap-2 text-xs text-muted-foreground">
			<span>{online} / {apps.length} services online</span>
		</div>
	</div>

	<div class="flex flex-wrap items-center gap-3">
		<div class="relative min-w-[12rem] flex-1">
			<Search class="absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
			<Input bind:value={search} placeholder="Search services…" class="pl-9" />
		</div>
		<label class="flex items-center gap-2 text-xs text-muted-foreground">
			Sort
			<select
				bind:value={sort}
				class="rounded-md border border-border bg-card px-2 py-1.5 text-xs text-white"
			>
				<option value="name">Name</option>
			</select>
		</label>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading services…</p>
	{:else if filtered.length === 0}
		<p class="text-sm text-muted-foreground">No public dashboard tiles configured.</p>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each filtered as app}
				<a
					href={app.domain ? `https://${app.domain}` : '#'}
					class="rounded-lg border border-border bg-card/50 p-4 transition-colors hover:border-hashi-hover/50"
				>
					<p class="font-medium text-white">{app.name}</p>
					<p class="mt-1 truncate font-mono text-xs text-muted-foreground">
						{app.domain ?? `${app.targetHost}:${app.targetPort}`}
					</p>
					<p class="mt-2 text-[11px] text-emerald-300">{app.enabled ? 'online' : 'offline'}</p>
				</a>
			{/each}
		</div>
	{/if}
</section>
