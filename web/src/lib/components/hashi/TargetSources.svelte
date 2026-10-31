<script lang="ts">
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { api, type Schemas } from '$lib/api/client';
	import { dateTime } from '$lib/format';

	// 08 §5: where each address of a connection's target comes from.
	let { setId }: { setId: string } = $props();
	let sources = $state<Schemas['IPSetSources']['sources']>([]);
	$effect(() => {
		const id = setId;
		if (!id) {
			sources = [];
			return;
		}
		api
			.GET('/api/ip-sets/{id}/sources', { params: { path: { id } } })
			.then((r) => (sources = r.data?.sources ?? []));
	});
</script>

{#if sources.length}
	<ul
		class="space-y-1 text-[12px] text-muted-foreground"
		aria-label="Where the addresses come from"
	>
		{#each sources as s, i (i)}
			<li class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
				<span>{s.source}:</span>
				<Mono>{s.addresses.join(', ') || 'no address yet'}</Mono>
				{#if s.agent}
					{#if s.agent.status === 'stale'}<Tag tone="warn">stale</Tag>{/if}
					{#if s.agent.last_seen_at}<span>seen {dateTime(s.agent.last_seen_at)}</span>{/if}
					{#if s.agent.public_ip}<span>public <Mono>{s.agent.public_ip}</Mono></span>{/if}
					{#if s.agent.reachable === true}<Tag tone="ok">reachable from Hashi</Tag>
					{:else if s.agent.reachable === false}<Tag tone="warn">not reachable from Hashi</Tag>{/if}
				{/if}
			</li>
		{/each}
	</ul>
{/if}
