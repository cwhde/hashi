<script lang="ts">
	import ApiPendingBanner from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Search } from 'lucide-svelte';

	let search = $state('');
	let sort = $state('name');
</script>

<section class="space-y-6">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<h1 class="text-xl font-semibold text-white">Homelab Dashboard</h1>
			<p class="text-sm text-muted-foreground">Public service tiles on port 8081.</p>
		</div>
		<div class="flex items-center gap-2 text-xs text-muted-foreground">
			<span>0 / 0 hosts online</span>
			<span aria-hidden="true">·</span>
			<span>0 / 0 firewall hosts available</span>
		</div>
	</div>

	<ApiPendingBanner
		message="Waiting for public dashboard API"
		detail="Cards will populate from /api/public/dashboard when backend publishes resource tile endpoints."
	/>

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
				<option value="status">Status</option>
				<option value="group">Group</option>
			</select>
		</label>
	</div>

	<div
		class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
		aria-label="Service tiles placeholder"
	>
		{#each Array(6) as _, index}
			<div class="rounded-lg border border-dashed border-border bg-card/30 p-4 text-center">
				<p class="text-xs text-muted-foreground">Tile slot {index + 1}</p>
			</div>
		{/each}
	</div>
</section>
