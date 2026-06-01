<script lang="ts">
	import { api } from '$lib/api/client';
	import type { PublicDashboard, PublicDashboardItem } from '$lib/api/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Search, X } from 'lucide-svelte';

	let { subtitle = 'Public homelab services.' }: { subtitle?: string } = $props();

	let dashboard = $state<PublicDashboard | null>(null);
	let search = $state('');
	let sort = $state('name');
	let loading = $state(true);
	let searchOpen = $state(false);

	$effect(() => {
		void (async () => {
			try {
				dashboard = await api.getPublicApps();
			} catch {
				dashboard = null;
			} finally {
				loading = false;
			}
		})();
	});

	const apps = $derived((dashboard?.items ?? []) as PublicDashboardItem[]);
	const filtered = $derived(
		apps
			.filter((a) => a.displayName.toLowerCase().includes(search.toLowerCase()))
			.sort((a, b) => a.displayName.localeCompare(b.displayName))
	);
</script>

<section class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<h1 class="text-xl font-semibold text-white">Homelab Dashboard</h1>
			<p class="text-sm text-muted-foreground">{subtitle}</p>
		</div>
		<div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
			<span>{dashboard?.hostsOnline ?? 0} / {dashboard?.totalHosts ?? 0} hosts online</span>
			<span class="text-border">|</span>
			<span>
				{dashboard?.linuxFirewallHostsAvailable ?? 0} /
				{dashboard?.totalLinuxFirewallHosts ?? 0} Linux firewall hosts available
			</span>
		</div>
	</div>

	<div class="flex flex-wrap items-center gap-3">
		<Button variant="outline" size="icon" onclick={() => (searchOpen = !searchOpen)} aria-label="Search">
			{#if searchOpen}
				<X class="size-4" />
			{:else}
				<Search class="size-4" />
			{/if}
		</Button>
		{#if searchOpen}
			<div class="relative min-w-[12rem] flex-1">
				<Search class="absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
				<Input bind:value={search} placeholder="Search services..." class="pl-9" />
			</div>
		{/if}
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
		<p class="text-sm text-muted-foreground">Loading services...</p>
	{:else if filtered.length === 0}
		<p class="text-sm text-muted-foreground">No public dashboard tiles configured.</p>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each filtered as app (app.id)}
				<a
					href={app.publicUrl}
					class="rounded-lg border border-border bg-card/50 p-4 transition-colors hover:border-hashi-hover/50"
				>
					<p class="font-medium text-white">{app.displayName}</p>
					<p class="mt-1 truncate font-mono text-xs text-muted-foreground">
						{app.domain ?? app.publicUrl}
					</p>
					<p class="mt-2 text-[11px] text-emerald-300">{app.status}</p>
				</a>
			{/each}
		</div>
	{/if}
</section>
