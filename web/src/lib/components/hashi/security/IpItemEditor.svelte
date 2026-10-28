<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import * as Select from '$lib/components/ui/select';
	import IpSetPicker from './IpSetPicker.svelte';
	import type { ItemType } from '$lib/design/security';

	// Add or edit one IP item: pick the type, then only that type's fields (05a §2). When the set is used as a
	// target, CIDR, Geo and non-address sets cannot be added (05a §5.2).
	let {
		usedAsTarget = false,
		type = $bindable<ItemType>('ip')
	}: { usedAsTarget?: boolean; type?: ItemType } = $props();
	let scope = $state('public');
	let geoKind = $state('country');
	let ref = $state('office');
	const types: { v: ItemType; l: string; target: boolean }[] = [
		{ v: 'ip', l: 'IP', target: true },
		{ v: 'cidr', l: 'CIDR', target: false },
		{ v: 'pulse', l: 'Pulse', target: true },
		{ v: 'host', l: 'Host', target: true },
		{ v: 'dns', l: 'DNS name', target: true },
		{ v: 'geo', l: 'Geo', target: false },
		{ v: 'set', l: 'Other set', target: true }
	];
	const scopes = [
		{ v: 'public', l: 'Public address' },
		{ v: 'internal', l: 'Internal addresses' },
		{ v: 'all', l: 'Both' }
	];
</script>

<div class="space-y-4">
	<div class="space-y-1.5">
		<Label>Type</Label>
		<ToggleGroup.Root type="single" bind:value={type} variant="outline" size="sm" class="flex-wrap">
			{#each types as t (t.v)}
				<ToggleGroup.Item value={t.v} disabled={usedAsTarget && !t.target}>{t.l}</ToggleGroup.Item>
			{/each}
		</ToggleGroup.Root>
		{#if usedAsTarget}<p class="text-[12.5px] text-muted-foreground">
				This set is a DNS value or upstream, so it can only hold single addresses.
			</p>{/if}
	</div>

	{#if type === 'ip' || type === 'cidr'}
		<div class="space-y-1.5">
			<Label for="iv">{type === 'ip' ? 'Address (IPv4 or IPv6)' : 'Prefix'}</Label>
			<Input
				id="iv"
				class="font-mono"
				placeholder={type === 'ip' ? '198.51.100.7' : '192.0.2.0/24'}
			/>
			{#if type === 'cidr'}<p class="text-[12.5px] text-muted-foreground">
					Host bits are cleared on save; a /32 or /128 is saved as an IP.
				</p>{/if}
		</div>
	{:else if type === 'pulse' || type === 'host'}
		<div class="grid gap-3 sm:grid-cols-2">
			<div class="space-y-1.5">
				<Label>{type === 'pulse' ? 'Pulse agent' : 'Firewall or edge host'}</Label>
				<Select.Root type="single" value={type === 'pulse' ? 'phone' : 'edge-fra'}>
					<Select.Trigger class="w-full">{type === 'pulse' ? 'phone' : 'edge-fra'}</Select.Trigger>
					<Select.Content>
						{#each type === 'pulse' ? ['phone', 'laptop', 'home-router', 'home-docker'] : ['edge-fra', 'edge-home'] as n (n)}
							<Select.Item value={n}>{n}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<div class="space-y-1.5">
				<Label>Which addresses</Label>
				<Select.Root type="single" bind:value={scope}>
					<Select.Trigger class="w-full">{scopes.find((s) => s.v === scope)?.l}</Select.Trigger>
					<Select.Content>
						{#each scopes as s (s.v)}<Select.Item value={s.v}>{s.l}</Select.Item>{/each}
					</Select.Content>
				</Select.Root>
			</div>
		</div>
		{#if type === 'pulse' && scope !== 'public'}
			<div class="space-y-1.5">
				<Label for="sel">Interface or CIDR (optional)</Label><Input
					id="sel"
					class="font-mono"
					placeholder="eth0 or 10.20.1.0/24"
				/>
			</div>
		{/if}
		<p class="text-[12.5px] text-muted-foreground">
			Follows the {type === 'pulse' ? 'agent' : 'host'}: when its address changes, every place using
			this set updates.
		</p>
	{:else if type === 'dns'}
		<div class="space-y-1.5">
			<Label for="fq">Name</Label><Input id="fq" class="font-mono" placeholder="vpn.example.org" />
			<p class="text-[12.5px] text-muted-foreground">
				Resolved by Hashi and refreshed at the record TTL (1 min to 1 h).
			</p>
		</div>
	{:else if type === 'geo'}
		<div class="grid gap-3 sm:grid-cols-[10rem_1fr]">
			<div class="space-y-1.5">
				<Label>Match by</Label>
				<Select.Root type="single" bind:value={geoKind}>
					<Select.Trigger class="w-full"
						>{(
							{ country: 'Country', region: 'Region', asn: 'Network (ASN)' } as Record<
								string,
								string
							>
						)[geoKind]}</Select.Trigger
					>
					<Select.Content>
						<Select.Item value="country">Country</Select.Item>
						<Select.Item value="region">Region</Select.Item>
						<Select.Item value="asn">Network (ASN)</Select.Item>
					</Select.Content>
				</Select.Root>
			</div>
			<div class="space-y-1.5">
				<Label for="gv">{geoKind === 'asn' ? 'ASN' : 'Code'}</Label><Input
					id="gv"
					class="font-mono"
					placeholder={geoKind === 'asn' ? 'AS64500' : geoKind === 'region' ? 'CH-ZH' : 'CH'}
				/>
			</div>
		</div>
		<p class="text-[12.5px] text-muted-foreground">
			Uses the GeoIP database (updated 6 days ago). On a firewall it becomes that country’s
			prefixes.
		</p>
	{:else}
		<div class="space-y-1.5">
			<Label for="ref">Set to include</Label><IpSetPicker
				id="ref"
				bind:value={ref}
				addressOnly={usedAsTarget}
			/>
			<p class="text-[12.5px] text-muted-foreground">
				Everything in that set counts as part of this one. Loops are refused when you save.
			</p>
		</div>
	{/if}

	<div class="grid gap-3 sm:grid-cols-[1fr_11rem]">
		<div class="space-y-1.5">
			<Label for="nt">Note</Label><Input id="nt" placeholder="Optional" />
		</div>
		<div class="space-y-1.5">
			<Label for="ex">Expires</Label><Input id="ex" type="date" aria-describedby="exh" />
		</div>
	</div>
	<p id="exh" class="-mt-2 text-[12.5px] text-muted-foreground">
		An expired item drops out at the next update and is recorded in Activity.
	</p>
</div>
