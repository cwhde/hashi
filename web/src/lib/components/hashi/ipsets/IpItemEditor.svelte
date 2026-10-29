<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import * as Select from '$lib/components/ui/select';
	import IpSetPicker from './IpSetPicker.svelte';
	import type { IPSet, ItemForm, ItemType } from '$lib/ipsets';

	// Add or edit one IP item: pick the type, then only that type's fields (05a §2). When the set is used as a
	// target, CIDR, Geo and non-address sets cannot be added (05a §5.2). Pulse and Geo items arrive with their
	// slices and show why they are not available yet; host items need a registered host.
	let {
		form = $bindable(),
		sets,
		self,
		hosts = [],
		usedAsTarget = false,
		lockType = false
	}: {
		form: ItemForm;
		sets: IPSet[];
		self: string;
		hosts?: { id: string; name: string }[];
		usedAsTarget?: boolean;
		lockType?: boolean;
	} = $props();
	type T = ItemType | 'pulse' | 'geo';
	const types: { v: T; l: string; target: boolean; why?: string }[] = $derived([
		{ v: 'ip', l: 'IP', target: true },
		{ v: 'cidr', l: 'CIDR', target: false },
		{ v: 'pulse', l: 'Pulse', target: true, why: 'Pulse agents arrive in a later version.' },
		{
			v: 'host',
			l: 'Host',
			target: true,
			why: hosts.length ? undefined : 'Add a firewall host first.'
		},
		{ v: 'dns', l: 'DNS name', target: true },
		{ v: 'geo', l: 'Geo', target: false, why: 'Countries and networks arrive with GeoIP.' },
		{ v: 'set', l: 'Other set', target: true }
	]);
	const unavailable = $derived(types.filter((t) => t.why).map((t) => t.why));
	const scopes = [
		{ v: 'public', l: 'Public address' },
		{ v: 'internal', l: 'Internal addresses' },
		{ v: 'all', l: 'Both' }
	] as const;
</script>

<div class="space-y-4">
	<div class="space-y-1.5">
		<Label>Type</Label>
		<ToggleGroup.Root
			type="single"
			bind:value={() => form.type, (v: string) => v && (form.type = v as ItemType)}
			variant="outline"
			size="sm"
			class="flex-wrap"
		>
			{#each types as t (t.v)}
				<ToggleGroup.Item
					value={t.v}
					title={t.why}
					disabled={!!t.why || (usedAsTarget && !t.target) || (lockType && t.v !== form.type)}
					>{t.l}</ToggleGroup.Item
				>
			{/each}
		</ToggleGroup.Root>
		{#if usedAsTarget}<p class="text-[12.5px] text-muted-foreground">
				This set is a DNS value or upstream, so it can only hold single addresses.
			</p>{/if}
		{#if unavailable.length}<p class="text-[12.5px] text-muted-foreground">
				{unavailable.join(' ')}
			</p>{/if}
	</div>

	{#if form.type === 'ip' || form.type === 'cidr'}
		<div class="space-y-1.5">
			<Label for="iv">{form.type === 'ip' ? 'Address (IPv4 or IPv6)' : 'Prefix'}</Label>
			<Input
				id="iv"
				class="font-mono"
				bind:value={form.value}
				placeholder={form.type === 'ip' ? '198.51.100.7' : '192.0.2.0/24'}
			/>
			{#if form.type === 'cidr'}<p class="text-[12.5px] text-muted-foreground">
					Host bits are cleared on save; a /32 or /128 is saved as an IP.
				</p>{/if}
		</div>
	{:else if form.type === 'host'}
		<div class="grid gap-3 sm:grid-cols-2">
			<div class="space-y-1.5">
				<Label>Firewall or edge host</Label>
				<Select.Root type="single" bind:value={form.hostId}>
					<Select.Trigger class="w-full"
						>{hosts.find((h) => h.id === form.hostId)?.name ?? 'Choose a host'}</Select.Trigger
					>
					<Select.Content>
						{#each hosts as h (h.id)}<Select.Item value={h.id}>{h.name}</Select.Item>{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<div class="space-y-1.5">
				<Label>Which addresses</Label>
				<Select.Root type="single" bind:value={form.scope}>
					<Select.Trigger class="w-full">{scopes.find((s) => s.v === form.scope)?.l}</Select.Trigger
					>
					<Select.Content>
						{#each scopes as s (s.v)}<Select.Item value={s.v}>{s.l}</Select.Item>{/each}
					</Select.Content>
				</Select.Root>
			</div>
		</div>
		<p class="text-[12.5px] text-muted-foreground">
			Follows the host: when its address changes, every place using this set updates.
		</p>
	{:else if form.type === 'dns'}
		<div class="space-y-1.5">
			<Label for="fq">Name</Label><Input
				id="fq"
				class="font-mono"
				bind:value={form.value}
				placeholder="vpn.example.org"
			/>
			<p class="text-[12.5px] text-muted-foreground">
				Resolved by Hashi and looked up again every 5 minutes.
			</p>
		</div>
	{:else}
		<div class="space-y-1.5">
			<Label for="ref">Set to include</Label><IpSetPicker
				id="ref"
				{sets}
				bind:value={form.setId}
				addressOnly={usedAsTarget}
				exclude={[self]}
			/>
			<p class="text-[12.5px] text-muted-foreground">
				Everything in that set counts as part of this one. Loops are refused when you save.
			</p>
		</div>
	{/if}

	<div class="grid gap-3 sm:grid-cols-[1fr_11rem]">
		<div class="space-y-1.5">
			<Label for="nt">Note</Label><Input id="nt" bind:value={form.note} placeholder="Optional" />
		</div>
		<div class="space-y-1.5">
			<Label for="ex">Expires</Label><Input
				id="ex"
				type="date"
				bind:value={form.expires}
				aria-describedby="exh"
			/>
		</div>
	</div>
	<p id="exh" class="-mt-2 text-[12.5px] text-muted-foreground">
		An expired item drops out at the next update and is recorded in Activity.
	</p>
</div>
