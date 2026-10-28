<script lang="ts">
	import { services } from '$lib/design/mock';
	import StatusDot from '$lib/components/hashi/StatusDot.svelte';
	import { Search } from '@lucide/svelte';
	const apps = services.filter((r) => r.dashboard);
	const groups = Object.entries(Object.groupBy(apps, (a) => a.group));
	const online = apps.filter((a) => a.health === 'up' || a.health === 'degraded').length;
</script>

<svelte:head><title>Apps · example.com</title></svelte:head>
<!-- Port 8081: no admin controls, only names, links and state (07 §8). -->
<div class="mx-auto max-w-6xl px-4 py-10">
	<header class="mb-8 flex flex-wrap items-end gap-4">
		<div class="flex-1">
			<h1 class="text-[24px] font-semibold">example.com</h1>
			<p class="text-[13.5px] text-muted-foreground">
				{online} / {apps.length} apps online · 2 / 2 edge hosts available
			</p>
		</div>
		<button
			class="flex h-9 items-center gap-2 rounded-[10px] border bg-card px-3 text-[13px] text-muted-foreground"
			><Search class="size-4" />Search</button
		>
	</header>
	{#each groups as [g, list] (g)}
		<section class="mb-8">
			<h2 class="mb-3 text-[13px] font-medium text-muted-foreground">{g}</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
				{#each list ?? [] as a (a.id)}
					<a
						href="https://{a.domain}"
						class="flex items-center gap-3 rounded-2xl border bg-card p-3.5 transition-colors hover:border-muted-foreground/40"
					>
						<span
							class="grid size-10 shrink-0 place-items-center rounded-xl border bg-surface-2 text-[15px] font-semibold"
							>{a.name[0]}</span
						>
						<span class="min-w-0 flex-1"
							><span class="block truncate text-[14px] font-medium">{a.name}</span><span
								class="block truncate text-[12.5px] text-muted-foreground">{a.domain}</span
							></span
						>
						<StatusDot state={a.health} />
					</a>
				{/each}
			</div>
		</section>
	{/each}
</div>
