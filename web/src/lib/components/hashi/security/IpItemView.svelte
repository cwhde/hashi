<script lang="ts">
	import Mono from '../Mono.svelte';
	import Tag from '../Tag.svelte';
	import SetBadge from './SetBadge.svelte';
	import type { IpItem } from '$lib/design/security';
	import { Radio, Server, Globe, MapPin, Hash, Layers } from '@lucide/svelte';

	// One IP item in a list: type icon, the value in its natural form, what it resolves to now (05a §2).
	let { item }: { item: IpItem } = $props();
	const icons = {
		ip: Hash,
		cidr: Hash,
		pulse: Radio,
		host: Server,
		dns: Globe,
		geo: MapPin,
		set: Layers
	};
	const typeLabel = {
		ip: 'IP',
		cidr: 'CIDR',
		pulse: 'Pulse',
		host: 'Host',
		dns: 'DNS',
		geo: 'Geo',
		set: 'Set'
	};
	const geoLabel = { country: 'Country', region: 'Region', asn: 'ASN' };
	const Icon = $derived(icons[item.type]);
</script>

<div class="flex min-w-0 items-start gap-2.5">
	<Icon class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-label={typeLabel[item.type]} />
	<div class="min-w-0">
		<div class="flex flex-wrap items-center gap-1.5 text-[13px]">
			{#if item.type === 'ip' || item.type === 'cidr' || item.type === 'dns'}
				<Mono>{item.value}</Mono>
			{:else if item.type === 'pulse' || item.type === 'host'}
				<span class="font-medium">{item.value}</span><Tag>{item.scope}</Tag>
				{#if item.label}<span class="text-[12.5px] text-muted-foreground">{item.label}</span>{/if}
			{:else if item.type === 'geo'}
				<span>{geoLabel[item.geoKind ?? 'country']}</span><Mono>{item.value}</Mono><span
					class="text-muted-foreground">{item.label}</span
				>
			{:else}
				<SetBadge slug={item.value} />
			{/if}
		</div>
		{#if item.resolved?.length}
			<div class="truncate text-[12px] text-muted-foreground">
				now <Mono class="text-[12px]">{item.resolved.join(', ')}</Mono>
			</div>
		{/if}
	</div>
</div>
