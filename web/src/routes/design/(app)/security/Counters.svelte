<script lang="ts">
	import Tag from '$lib/components/hashi/Tag.svelte';
	import AddressLink from '$lib/components/hashi/security/AddressLink.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Table from '$lib/components/ui/table';
	import * as Select from '$lib/components/ui/select';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { counters, inCidr, type CounterRow } from '$lib/design/security';
	import { Search, MoreHorizontal, KeyRound } from '@lucide/svelte';

	// Security → Counters (06 §13.1): every address with a ban or challenge counter, active or not.
	let view = $state('all');
	let q = $state('');
	let minBan = $state(0);
	let minChallenge = $state(0);
	let source = $state('any');
	let selected = $state<string[]>([]);
	let editing = $state<CounterRow | null>(null);
	let bulkOpen = $state(false);
	const sources = ['any', 'L7 rate', 'WAF', 'challenge failed', 'L4 meter', 'manual'];
	const inRange = (a: string, query: string) =>
		!query || (query.includes('/') ? inCidr(a, query) : a.includes(query));
	const rows = $derived(
		counters.filter(
			(c) =>
				(!view || view === 'all' || (view === 'active' ? c.state !== 'none' : c.state === view)) &&
				c.ban >= minBan &&
				c.challenge >= minChallenge &&
				(source === 'any' || c.source === source) &&
				(inRange(c.address, q.trim()) ||
					c.asn.toLowerCase().includes(q.toLowerCase()) ||
					c.country === q.toUpperCase())
		)
	);
	const allSelected = $derived(rows.length > 0 && rows.every((r) => selected.includes(r.address)));
	const toggle = (a: string) =>
		(selected = selected.includes(a) ? selected.filter((x) => x !== a) : [...selected, a]);
</script>

<div class="rounded-2xl border bg-card">
	<div class="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
		<div class="relative w-full max-w-64">
			<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
			<Input
				bind:value={q}
				placeholder="Address, CIDR, ASN or country"
				class="pl-8"
				aria-label="Search counters"
			/>
		</div>
		<ToggleGroup.Root type="single" bind:value={view} variant="outline" size="sm">
			<ToggleGroup.Item value="banned">Banned now</ToggleGroup.Item>
			<ToggleGroup.Item value="challenged">Challenged now</ToggleGroup.Item>
			<ToggleGroup.Item value="active">Any active</ToggleGroup.Item>
			<ToggleGroup.Item value="all">All</ToggleGroup.Item>
		</ToggleGroup.Root>
		<Select.Root type="single" bind:value={source}>
			<Select.Trigger size="sm" class="w-40" aria-label="Last source"
				>{source === 'any' ? 'Any source' : source}</Select.Trigger
			>
			<Select.Content>
				{#each sources as s (s)}<Select.Item value={s}>{s === 'any' ? 'Any source' : s}</Select.Item
					>{/each}
			</Select.Content>
		</Select.Root>
		<label class="flex items-center gap-1.5 text-[12.5px] text-muted-foreground"
			>Bans ≥<Input type="number" min="0" bind:value={minBan} class="tnum h-7 w-14" /></label
		>
		<label class="flex items-center gap-1.5 text-[12.5px] text-muted-foreground"
			>Challenges ≥<Input
				type="number"
				min="0"
				bind:value={minChallenge}
				class="tnum h-7 w-14"
			/></label
		>
		<span class="ml-auto text-[12.5px] text-muted-foreground"
			>{rows.length} of {counters.length}</span
		>
	</div>
	{#if selected.length || (rows.length < counters.length && rows.length)}
		<div class="flex flex-wrap items-center gap-2 border-b bg-surface-2/40 px-4 py-2 text-[13px]">
			{#if selected.length}<span>{selected.length} selected</span>{:else}<span
					>{rows.length} addresses match the filter</span
				>{/if}
			<Button size="xs" variant="outline" onclick={() => (bulkOpen = true)}
				>Reset counters of {selected.length || 'all ' + rows.length}</Button
			>
			{#if selected.length}<Button size="xs" variant="ghost" onclick={() => (selected = [])}
					>Clear selection</Button
				>{/if}
		</div>
	{/if}
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head class="w-10 pl-4"
					><Checkbox
						checked={allSelected}
						onCheckedChange={(v) => (selected = v ? rows.map((r) => r.address) : [])}
						aria-label="Select all"
					/></Table.Head
				>
				<Table.Head>Address</Table.Head>
				<Table.Head class="text-right">Bans</Table.Head>
				<Table.Head class="text-right">Challenges</Table.Head>
				<Table.Head>Now</Table.Head>
				<Table.Head class="hidden lg:table-cell">Last ban · challenge</Table.Head>
				<Table.Head class="hidden xl:table-cell">Next decrement</Table.Head>
				<Table.Head class="hidden md:table-cell">Last source</Table.Head>
				<Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each rows as c (c.address)}
				<Table.Row>
					<Table.Cell class="pl-4"
						><Checkbox
							checked={selected.includes(c.address)}
							onCheckedChange={() => toggle(c.address)}
							aria-label="Select {c.address}"
						/></Table.Cell
					>
					<Table.Cell>
						<AddressLink address={c.address} />
						<span class="block max-w-48 truncate text-[12px] text-muted-foreground"
							>{c.country} · {c.asn}</span
						>
					</Table.Cell>
					<Table.Cell class="tnum text-right text-[14px] font-semibold">{c.ban}</Table.Cell>
					<Table.Cell class="tnum text-right text-[14px] font-semibold">{c.challenge}</Table.Cell>
					<Table.Cell>
						{#if c.state === 'banned'}<Tag tone="bad">banned</Tag
							>{:else if c.state === 'challenged'}<Tag tone="warn">challenged</Tag>{:else}<span
								class="text-muted-foreground">—</span
							>{/if}
						{#if c.state !== 'none'}<span class="block text-[12px] text-muted-foreground"
								>{c.stateDetail}</span
							>{/if}
					</Table.Cell>
					<Table.Cell class="hidden text-[12.5px] text-muted-foreground lg:table-cell"
						>{c.lastBan} · {c.lastChallenge}</Table.Cell
					>
					<Table.Cell class="hidden text-[12.5px] text-muted-foreground xl:table-cell"
						>{c.nextDecrement}</Table.Cell
					>
					<Table.Cell class="hidden md:table-cell">{c.source}</Table.Cell>
					<Table.Cell>
						<DropdownMenu.Root>
							<DropdownMenu.Trigger>
								{#snippet child({ props })}<Button
										{...props}
										variant="ghost"
										size="icon-sm"
										aria-label="Actions for {c.address}"><MoreHorizontal /></Button
									>{/snippet}
							</DropdownMenu.Trigger>
							<DropdownMenu.Content align="end">
								<DropdownMenu.Item onclick={() => (editing = c)}>Edit counters</DropdownMenu.Item>
								<DropdownMenu.Item>Reset both</DropdownMenu.Item>
								<DropdownMenu.Separator />
								{#if c.state !== 'banned'}<DropdownMenu.Item>Ban now</DropdownMenu.Item>{/if}
								{#if c.state === 'none'}<DropdownMenu.Item>Challenge now</DropdownMenu.Item>{/if}
								{#if c.state !== 'none'}<DropdownMenu.Item class="text-bad-text"
										>{c.state === 'banned' ? 'Unban' : 'End challenge'}</DropdownMenu.Item
									>{/if}
							</DropdownMenu.Content>
						</DropdownMenu.Root>
					</Table.Cell>
				</Table.Row>
			{:else}
				<Table.Row
					><Table.Cell colspan={9} class="py-10 text-center text-muted-foreground"
						>No address matches. Counters appear the first time an address is challenged or banned.</Table.Cell
					></Table.Row
				>
			{/each}
		</Table.Body>
	</Table.Root>
</div>
<p class="mt-2 text-[12.5px] text-muted-foreground">
	The ban counter sets the length of the next ban; the challenge counter is for information. Each
	drops by one per 7 days without a new ban or challenge. Solving a CAPTCHA resets neither.
</p>

<Dialog.Root open={editing !== null} onOpenChange={(o) => !o && (editing = null)}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Counters for <span class="font-mono">{editing?.address}</span></Dialog.Title>
			<Dialog.Description
				>A running ban keeps its end time; the new ban counter applies to the next ban.</Dialog.Description
			>
		</Dialog.Header>
		<div class="grid grid-cols-2 gap-3">
			<div class="space-y-1.5">
				<Label for="eb">Ban counter</Label><Input
					id="eb"
					type="number"
					min="0"
					value={editing?.ban}
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="ec">Challenge counter</Label><Input
					id="ec"
					type="number"
					min="0"
					value={editing?.challenge}
				/>
			</div>
		</div>
		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (editing = null)}>Cancel</Button>
			<Button onclick={() => (editing = null)}><KeyRound />Confirm with passkey and save</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={bulkOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Reset counters of {selected.length || rows.length} addresses?</Dialog.Title>
			<Dialog.Description
				>Both counters go to 0. Running bans and challenges stay as they are; their next ban starts
				again at 15 min. Recorded in the audit log.</Dialog.Description
			>
		</Dialog.Header>
		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (bulkOpen = false)}>Cancel</Button>
			<Button variant="destructive" onclick={() => (bulkOpen = false)}
				><KeyRound />Confirm with passkey and reset</Button
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
