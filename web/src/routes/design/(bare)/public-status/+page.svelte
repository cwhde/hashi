<script lang="ts">
	import { monitors } from '$lib/design/mock';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	const pub = monitors.filter((m) => m.public);
	const groups = Object.entries(Object.groupBy(pub, (m) => m.group));
	const down = pub.filter((m) => m.state === 'down').length;
</script>

<svelte:head><title>Status · example.com</title></svelte:head>
<!-- Port 8082: names, groups, state, 60-minute strips, uptime. Never hosts, IPs or errors (07 §7). -->
<div class="mx-auto max-w-4xl px-4 py-10">
	<h1 class="text-[24px] font-semibold">example.com status</h1>
	<div
		class="{down
			? 'border-bad/30 bg-bad/10 text-bad-text'
			: 'border-ok/30 bg-ok/10 text-ok-text'} mt-4 rounded-2xl border px-4 py-3 text-[14px] font-medium"
	>
		{down ? `${down} service is down. Others are working.` : 'All services are working.'}
	</div>
	<p class="mt-2 text-[12.5px] text-muted-foreground">
		Updated 13:42. If this stops updating, the page shows how old the data is.
	</p>
	{#each groups as [g, list] (g)}
		<section class="mt-6 rounded-2xl border bg-card">
			<h2 class="border-b px-4 py-2.5 text-[14px] font-semibold">{g}</h2>
			<ul class="divide-y">
				{#each list ?? [] as m (m.id)}
					<li class="grid gap-2 px-4 py-3 sm:grid-cols-[12rem_minmax(0,1fr)_6rem] sm:items-center">
						<span class="text-[13.5px] font-medium">{m.name}</span><HistoryStrip
							samples={m.history}
						/><span class="sm:text-right"><StatusBadge state={m.state} /></span>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
</div>
