<script lang="ts">
	import Mono from '../Mono.svelte';
	import Tag from '../Tag.svelte';
	import SetBadge from './SetBadge.svelte';
	import { Activity, Server, Globe, Hash, Layers, MapPin } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { pulseAgents } from '#lib/pulseAgents.svelte.ts';
	import { firewallHosts } from '#lib/firewallHosts.svelte.ts';
	import { itemValue, type IPSet, type IPItem } from '#lib/ipsets.ts';

	// One IP item in a list: type icon and the value in its natural form (05a §2). A set reference shows the set's
	// badge; a host shows its name when known.
	let { item, sets }: { item: IPItem; sets: IPSet[] } = $props();
	const icons = {
		ip: Hash,
		cidr: Hash,
		host: Server,
		pulse: Activity,
		dns: Globe,
		geo: MapPin,
		set: Layers
	};
	const typeLabel = {
		ip: 'IP',
		cidr: 'CIDR',
		host: 'Host',
		pulse: 'Pulse',
		dns: 'DNS',
		geo: 'Geo',
		set: 'Set'
	};
	onMount(() => {
		if (item.type === 'pulse') void pulseAgents.load();
		if (item.type === 'host') void firewallHosts.load();
	});
	const Icon = $derived(icons[item.type]);
	const ref = $derived(item.type === 'set' ? sets.find((s) => s.id === item.set_id) : undefined);
	// What a host or agent item stands for right now (bug 122): its current addresses, next to its name.
	const now = $derived.by(() => {
		if (item.type === 'pulse') {
			const a = pulseAgents.list.find((x) => x.id === item.agent_id);
			if (!a) return '';
			const pub = a.public_ip;
			const internal = a.selected_ip;
			return item.scope === 'public'
				? pub
				: item.scope === 'internal'
					? internal
					: [internal, pub].filter(Boolean).join(', ');
		}
		if (item.type === 'host') {
			const h = firewallHosts.list.find((x) => x.id === item.host_id);
			if (!h) return '';
			return item.scope === 'public'
				? h.public_ip
				: item.scope === 'internal'
					? h.subnets.join(', ')
					: [h.public_ip, ...h.subnets].filter(Boolean).join(', ');
		}
		return '';
	});
</script>

<div class="flex min-w-0 items-start gap-2.5">
	<Icon class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-label={typeLabel[item.type]} />
	<div class="flex min-w-0 flex-wrap items-center gap-1.5 text-md">
		{#if item.type === 'host'}
			<span class="font-medium">{firewallHosts.name(item.host_id) ?? 'Firewall host'}</span><Tag
				>{item.scope}</Tag
			>{#if now}<Mono class="text-muted-foreground">{now}</Mono>{/if}
		{:else if item.type === 'pulse'}
			<span class="font-medium"
				>{pulseAgents.list.find((a) => a.id === item.agent_id)?.name ?? 'Pulse agent'}</span
			><Tag>{item.scope}</Tag>{#if item.selector}<Mono>{item.selector}</Mono>{/if}{#if now}<Mono
					class="text-muted-foreground">{now}</Mono
				>{/if}
		{:else if item.type === 'set'}
			{#if ref}<SetBadge set={ref} />{:else}<span class="text-muted-foreground">missing set</span
				>{/if}
		{:else}
			<Mono>{itemValue(item)}</Mono>
		{/if}
	</div>
</div>
