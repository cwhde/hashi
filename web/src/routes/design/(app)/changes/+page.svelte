<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { plans } from '$lib/design/mock';
	import { RefreshCw } from '@lucide/svelte';

	const history = [
		{
			t: 'Today 13:28',
			title: 'Agent nas changed IP',
			result: 'Applied and verified',
			items: 2,
			tone: 'ok' as const
		},
		{
			t: 'Today 12:59',
			title: 'Extend block for 192.0.2.201',
			result: 'Applied on edge-fra',
			items: 1,
			tone: 'ok' as const
		},
		{
			t: 'Today 09:14',
			title: 'Traefik 3.7.18 on traefik-home',
			result: 'Rolled back: ping timed out after 30 s',
			items: 4,
			tone: 'bad' as const
		},
		{
			t: 'Yesterday 22:01',
			title: 'Global reconcile',
			result: 'No changes',
			items: 0,
			tone: 'neutral' as const
		}
	];
</script>

<PageHeader
	title="Pending changes"
	description="Every change to DNS, Traefik, firewall and AdGuard is planned first. You see the exact values here; what you approve is exactly what gets applied."
>
	{#snippet actions()}
		<Button variant="outline" size="sm"><RefreshCw />Plan everything now</Button>
	{/snippet}
</PageHeader>

<div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
	<div class="space-y-5">
		{#each plans as p (p.id)}
			<PlanReview {...p} />
		{/each}
	</div>
	<Panel title="Recent runs" description="Applied, failed and no-op plans">
		<ul class="space-y-3">
			{#each history as h, i (i)}
				<li class="text-[13px]">
					<div class="flex items-center justify-between gap-2">
						<span class="font-medium">{h.title}</span>
						<Tag tone={h.tone}>{h.items} items</Tag>
					</div>
					<div class="text-[12.5px] text-muted-foreground">{h.t} · {h.result}</div>
				</li>
			{/each}
		</ul>
	</Panel>
</div>
