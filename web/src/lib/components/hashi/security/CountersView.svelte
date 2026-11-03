<script lang="ts">
	// Security → Counters (06 §13.1): every address with a ban or challenge counter, active or not.
	import { onMount } from 'svelte';
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
	import { Search, MoreHorizontal, KeyRound } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { dateTime } from '$lib/format';
	import { sourceLabel } from './labels';

	type Row = Schemas['CounterRow'];
	type Source = Schemas['SecuritySource'];
	const sources: Source[] = ['l7_rate', 'l7_waf', 'l7_challenge_failed', 'l4_meter', 'manual'];

	let view = $state<'banned' | 'challenged' | 'active' | 'all'>('all');
	let search = $state('');
	let minBan = $state(0);
	let minChallenge = $state(0);
	let source = $state<Source | 'any'>('any');
	let rows = $state<Row[]>([]);
	let total = $state(0);
	let next = $state<string | undefined>();
	let selected = $state<string[]>([]);
	let editing = $state<Row | null>(null);
	let editBan = $state(0);
	let editChallenge = $state(0);
	let bulkOpen = $state(false);
	let busy = $state(false);
	let error = $state('');

	// The search box takes an address or CIDR, an ASN ("AS3320" or "3320") or a country code ("DE").
	function filter(): Schemas['CounterFilter'] {
		const s = search.trim();
		const f: Schemas['CounterFilter'] = { state: view };
		if (minBan > 0) f.min_ban = minBan;
		if (minChallenge > 0) f.min_challenge = minChallenge;
		if (source !== 'any') f.source = source;
		if (/^(as)?\d+$/i.test(s)) f.asn = Number(s.replace(/^as/i, ''));
		else if (/^[a-z]{2}$/i.test(s)) f.country = s.toUpperCase();
		else if (s) f.q = s;
		return f;
	}

	async function load(more = false) {
		error = '';
		const r = await api.GET('/api/security/counters', {
			params: { query: { ...filter(), cursor: more ? next : undefined, limit: 100 } }
		});
		if (r.error || !r.data) {
			error = problemText(r.error);
			return;
		}
		rows = more ? [...rows, ...r.data.rows] : r.data.rows;
		total = r.data.total;
		next = r.data.next_cursor;
		if (!more) selected = [];
	}

	let timer: ReturnType<typeof setTimeout> | undefined;
	// Typing waits a moment before asking again; the other filters reload at once.
	function reload(delay = 0) {
		clearTimeout(timer);
		timer = setTimeout(() => load(), delay);
	}
	onMount(() => load());

	async function act(f: () => Promise<{ data?: unknown; error?: unknown }>) {
		busy = true;
		error = '';
		try {
			await withReauth(f);
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}

	const allSelected = $derived(rows.length > 0 && rows.every((r) => selected.includes(r.address)));
	const toggle = (a: string) =>
		(selected = selected.includes(a) ? selected.filter((x) => x !== a) : [...selected, a]);
	const openEdit = (r: Row) => (
		(editing = r),
		(editBan = r.ban_count),
		(editChallenge = r.challenge_count)
	);
	// A /64 row stands for its network; its first address addresses the same counter.
	const addr = (k: string) => k.split('/')[0] ?? k;

	const saveEdit = () =>
		editing &&
		act(() =>
			api.PATCH('/api/addresses/{address}/counters', {
				params: { path: { address: addr(editing!.address) } },
				body: { ban_count: editBan, challenge_count: editChallenge }
			})
		).then(() => (editing = null));
	const resetOne = (r: Row) =>
		act(() => api.POST('/api/security/counters/reset', { body: { addresses: [r.address] } }));
	const resetBulk = () =>
		act(() =>
			api.POST('/api/security/counters/reset', {
				body: selected.length ? { addresses: selected } : { all_matching: filter() }
			})
		).then(() => (bulkOpen = false));
	const enter = (r: Row, set: 'sys-dynamic-blocked' | 'sys-dynamic-challenge') =>
		act(() =>
			api.POST('/api/ip-sets/{id}/entries', {
				params: { path: { id: set } },
				body: { address: addr(r.address) }
			})
		);
	const remove = (r: Row) =>
		act(() =>
			api.DELETE('/api/ip-sets/{id}/entries/{address}', {
				params: { path: { id: 'sys-dynamic-blocked', address: addr(r.address) } }
			})
		);
	const when = (t?: string) => (t ? dateTime(t) : '—');
	const stateDetail = (r: Row) =>
		r.state === 'banned'
			? r.until
				? `until ${dateTime(r.until)}`
				: 'permanent'
			: `${r.failures} of ${r.max_failures} unsolved${r.until ? ` · ends ${dateTime(r.until)}` : ''}`;
</script>

{#if error}<p
		role="alert"
		class="mb-3 rounded-2xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
	>
		{error}
	</p>{/if}
<div class="rounded-2xl border bg-card">
	<div class="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
		<div class="relative w-full sm:max-w-64">
			<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
			<Input
				bind:value={search}
				oninput={() => reload(300)}
				placeholder="Address, CIDR, ASN or country"
				class="pl-8"
				aria-label="Search counters"
			/>
		</div>
		<ToggleGroup.Root
			type="single"
			bind:value={view}
			onValueChange={(v) => v && reload()}
			variant="outline"
			size="sm"
			class="max-w-full overflow-x-auto"
		>
			<ToggleGroup.Item value="banned">Banned now</ToggleGroup.Item>
			<ToggleGroup.Item value="challenged">Challenged now</ToggleGroup.Item>
			<ToggleGroup.Item value="active">Any active</ToggleGroup.Item>
			<ToggleGroup.Item value="all">All</ToggleGroup.Item>
		</ToggleGroup.Root>
		<Select.Root type="single" bind:value={source} onValueChange={() => reload()}>
			<Select.Trigger size="sm" class="w-44" aria-label="Last source"
				>{source === 'any' ? 'Any source' : sourceLabel(source)}</Select.Trigger
			>
			<Select.Content>
				<Select.Item value="any">Any source</Select.Item>
				{#each sources as s (s)}<Select.Item value={s}>{sourceLabel(s)}</Select.Item>{/each}
			</Select.Content>
		</Select.Root>
		<label class="flex items-center gap-1.5 text-[12.5px] text-muted-foreground"
			>Bans ≥<Input
				type="number"
				min="0"
				bind:value={minBan}
				onchange={() => reload()}
				class="tnum h-7 w-14"
			/></label
		>
		<label class="flex items-center gap-1.5 text-[12.5px] text-muted-foreground"
			>Challenges ≥<Input
				type="number"
				min="0"
				bind:value={minChallenge}
				onchange={() => reload()}
				class="tnum h-7 w-14"
			/></label
		>
		<span class="ml-auto text-[12.5px] text-muted-foreground"
			>{total} {total === 1 ? 'address' : 'addresses'}</span
		>
	</div>
	{#if selected.length || (view !== 'all' && total)}
		<div class="flex flex-wrap items-center gap-2 border-b bg-surface-2/40 px-4 py-2 text-[13px]">
			{#if selected.length}<span>{selected.length} selected</span>{:else}<span
					>{total} addresses match the filter</span
				>{/if}
			<Button size="xs" variant="outline" onclick={() => (bulkOpen = true)}
				>Reset counters of {selected.length || 'all ' + total}</Button
			>
			{#if selected.length}<Button size="xs" variant="ghost" onclick={() => (selected = [])}
					>Clear selection</Button
				>{/if}
		</div>
	{/if}
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head class="hidden w-10 pl-4 sm:table-cell"
					><Checkbox
						checked={allSelected}
						onCheckedChange={(v) => (selected = v ? rows.map((r) => r.address) : [])}
						aria-label="Select all"
					/></Table.Head
				>
				<Table.Head>Address</Table.Head>
				<Table.Head class="text-right">Bans</Table.Head>
				<Table.Head class="hidden text-right sm:table-cell">Challenges</Table.Head>
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
					<Table.Cell class="hidden pl-4 sm:table-cell"
						><Checkbox
							checked={selected.includes(c.address)}
							onCheckedChange={() => toggle(c.address)}
							aria-label="Select {c.address}"
						/></Table.Cell
					>
					<Table.Cell>
						<AddressLink address={c.address} />
						{#if c.country || c.asn}<span
								class="block max-w-48 truncate text-[12px] text-muted-foreground"
								>{[c.country, c.asn ? `AS${c.asn}` : ''].filter(Boolean).join(', ')}</span
							>{/if}
					</Table.Cell>
					<Table.Cell class="tnum text-right text-[14px] font-semibold">{c.ban_count}</Table.Cell>
					<Table.Cell class="tnum hidden text-right text-[14px] font-semibold sm:table-cell"
						>{c.challenge_count}</Table.Cell
					>
					<Table.Cell>
						{#if c.state === 'banned'}<Tag tone="bad">banned</Tag
							>{:else if c.state === 'challenged'}<Tag tone="warn">challenged</Tag>{:else}<span
								class="text-muted-foreground">—</span
							>{/if}
						{#if c.state !== 'none'}<span class="block text-[12px] text-muted-foreground"
								>{stateDetail(c)}</span
							>{/if}
					</Table.Cell>
					<Table.Cell class="hidden text-[12.5px] text-muted-foreground lg:table-cell"
						>{when(c.last_ban)} · {when(c.last_challenge)}</Table.Cell
					>
					<Table.Cell class="hidden text-[12.5px] text-muted-foreground xl:table-cell"
						>{when(c.next_ban_decrement ?? c.next_challenge_decrement)}</Table.Cell
					>
					<Table.Cell class="hidden md:table-cell">{sourceLabel(c.last_source)}</Table.Cell>
					<Table.Cell>
						<DropdownMenu.Root>
							<DropdownMenu.Trigger>
								{#snippet child({ props })}<Button
										{...props}
										variant="ghost"
										size="icon-sm"
										disabled={busy}
										aria-label="Actions for {c.address}"><MoreHorizontal /></Button
									>{/snippet}
							</DropdownMenu.Trigger>
							<DropdownMenu.Content align="end">
								<DropdownMenu.Item onclick={() => openEdit(c)}>Edit counters</DropdownMenu.Item>
								<DropdownMenu.Item onclick={() => resetOne(c)}>Reset both</DropdownMenu.Item>
								<DropdownMenu.Separator />
								{#if c.state !== 'banned'}<DropdownMenu.Item
										onclick={() => enter(c, 'sys-dynamic-blocked')}>Ban now</DropdownMenu.Item
									>{/if}
								{#if c.state === 'none'}<DropdownMenu.Item
										onclick={() => enter(c, 'sys-dynamic-challenge')}
										>Challenge now</DropdownMenu.Item
									>{/if}
								{#if c.state !== 'none'}<DropdownMenu.Item
										class="text-bad-text"
										onclick={() => remove(c)}
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
	{#if next}
		<div class="border-t px-4 py-2.5">
			<Button size="sm" variant="ghost" onclick={() => load(true)}>Show more</Button>
		</div>
	{/if}
</div>
<p class="mt-2 text-[12.5px] text-muted-foreground">
	The ban counter sets the length of the next ban; the challenge counter is for information. Each
	drops by one per quiet period without a new ban or challenge. Solving a CAPTCHA resets neither.
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
					bind:value={editBan}
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="ec">Challenge counter</Label><Input
					id="ec"
					type="number"
					min="0"
					bind:value={editChallenge}
				/>
			</div>
		</div>
		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (editing = null)}>Cancel</Button>
			<Button disabled={busy} onclick={saveEdit}><KeyRound />Confirm with passkey and save</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={bulkOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Reset counters of {selected.length || total} addresses?</Dialog.Title>
			<Dialog.Description
				>Both counters go to 0. Running bans and challenges stay as they are; the next ban starts
				again at the shortest length. Recorded in the audit log.</Dialog.Description
			>
		</Dialog.Header>
		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (bulkOpen = false)}>Cancel</Button>
			<Button variant="destructive" disabled={busy} onclick={resetBulk}
				><KeyRound />Confirm with passkey and reset</Button
			>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
