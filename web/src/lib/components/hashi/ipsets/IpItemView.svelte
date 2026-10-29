<script lang="ts">
	import Mono from '../Mono.svelte';
	import Tag from '../Tag.svelte';
	import SetBadge from './SetBadge.svelte';
	import { Server, Globe, Hash, Layers } from '@lucide/svelte';
	import { itemValue, type IPSet, type IPItem } from '$lib/ipsets';

	// One IP item in a list: type icon and the value in its natural form (05a §2). A set reference shows the set's
	// badge; a host shows its name when known.
	let {
		item,
		sets,
		hostName
	}: { item: IPItem; sets: IPSet[]; hostName?: (id: string) => string | undefined } = $props();
	const icons = { ip: Hash, cidr: Hash, host: Server, dns: Globe, set: Layers };
	const typeLabel = { ip: 'IP', cidr: 'CIDR', host: 'Host', dns: 'DNS', set: 'Set' };
	const Icon = $derived(icons[item.type]);
	const ref = $derived(item.type === 'set' ? sets.find((s) => s.id === item.set_id) : undefined);
</script>

<div class="flex min-w-0 items-start gap-2.5">
	<Icon class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-label={typeLabel[item.type]} />
	<div class="flex min-w-0 flex-wrap items-center gap-1.5 text-[13px]">
		{#if item.type === 'host'}
			<span class="font-medium">{hostName?.(item.host_id) ?? item.host_id}</span><Tag
				>{item.scope}</Tag
			>
		{:else if item.type === 'set'}
			{#if ref}<SetBadge set={ref} />{:else}<span class="text-muted-foreground">missing set</span
				>{/if}
		{:else}
			<Mono>{itemValue(item)}</Mono>
		{/if}
	</div>
</div>
