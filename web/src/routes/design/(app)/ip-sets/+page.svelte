<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import IpItemEditor from '$lib/components/hashi/security/IpItemEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import { ipSets, containsAddress, type SetKind } from '$lib/design/security';
	import { Plus, Search, Lock, Rss } from '@lucide/svelte';

	let q = $state('');
	let kind = $state<'all' | SetKind>('all');
	let addOpen = $state(false);
	const kindLabel = { user: 'Yours', system: 'System', feed: 'Feed', dynamic: 'Dynamic' };
	const looksLikeAddress = $derived(/^[0-9a-f:.]+(\/\d+)?$/i.test(q.trim()) && q.includes('.'));
	const rows = $derived(
		ipSets.filter(
			(s) =>
				(!kind || kind === 'all' || s.kind === kind) &&
				(looksLikeAddress
					? containsAddress(s, q.trim())
					: (s.slug + s.description).toLowerCase().includes(q.toLowerCase()))
		)
	);
</script>

<PageHeader
	title="IP sets"
	description="Named groups of addresses. Firewall allow and block, rules, DNS values, upstreams and SSH targets all point at a set, so one change reaches every place that uses it."
>
	{#snippet actions()}
		<Button variant="outline"><Rss />Add feed</Button>
		<Button onclick={() => (addOpen = true)}><Plus />Add set</Button>
	{/snippet}
</PageHeader>

<div class="rounded-2xl border bg-card">
	<div class="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
		<div class="relative w-full max-w-80">
			<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
			<Input
				bind:value={q}
				placeholder="Filter by name, or type an address"
				class="pl-8"
				aria-label="Filter IP sets"
			/>
		</div>
		<ToggleGroup.Root type="single" bind:value={kind} variant="outline" size="sm">
			<ToggleGroup.Item value="all">All</ToggleGroup.Item>
			{#each Object.entries(kindLabel) as [k, l] (k)}<ToggleGroup.Item value={k}
					>{l}</ToggleGroup.Item
				>{/each}
		</ToggleGroup.Root>
		<span class="ml-auto text-[12.5px] text-muted-foreground">{rows.length} of {ipSets.length}</span
		>
	</div>
	{#if looksLikeAddress}
		<div class="flex flex-wrap items-center gap-2 border-b bg-surface-2/40 px-4 py-2.5 text-[13px]">
			<span>Sets that contain <span class="font-mono">{q}</span> are listed below.</span>
			<a class="font-medium underline" href="/design/ip-sets/address?ip={encodeURIComponent(q)}"
				>Open the address page</a
			>
		</div>
	{/if}
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head class="pl-4">Set</Table.Head>
				<Table.Head class="hidden md:table-cell">Kind</Table.Head>
				<Table.Head class="hidden text-right sm:table-cell">Size</Table.Head>
				<Table.Head class="hidden lg:table-cell">Used by</Table.Head>
				<Table.Head>Status</Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each rows as s (s.id)}
				<Table.Row>
					<Table.Cell class="max-w-[28rem] pl-4">
						<a href="/design/ip-sets/{s.id}" class="group block">
							<span class="flex items-center gap-1.5 font-medium group-hover:underline"
								>{s.slug}{#if s.locked}<Lock
										class="size-3.5 text-muted-foreground"
										aria-label="Not editable"
									/>{/if}</span
							>
							<span class="block truncate text-[12.5px] text-muted-foreground">{s.description}</span
							>
						</a>
					</Table.Cell>
					<Table.Cell class="hidden md:table-cell">
						<Tag>{kindLabel[s.kind]}</Tag>
						{#if s.addressSet}<Tag tone="info">addresses only</Tag>{/if}
					</Table.Cell>
					<Table.Cell class="tnum hidden text-right sm:table-cell">
						{#if s.kind === 'dynamic'}{s.entries?.length} entries{:else if s.kind === 'feed'}{(
								s.v4 + s.v6
							).toLocaleString()} entries{:else}{s.items.length} items{/if}
						<span class="block text-[12px] text-muted-foreground"
							>{s.v4.toLocaleString()} v4 · {s.v6.toLocaleString()} v6</span
						>
					</Table.Cell>
					<Table.Cell class="hidden max-w-[22rem] lg:table-cell">
						{#if s.usedBy.length}
							<span class="block truncate text-[13px]">{s.usedBy[0]?.label}</span>
							{#if s.usedBy.length > 1}<span class="text-[12px] text-muted-foreground"
									>and {s.usedBy.length - 1} more</span
								>{/if}
						{:else}<span class="text-muted-foreground">Not used</span>{/if}
					</Table.Cell>
					<Table.Cell>
						{#if s.status === 'partial'}<Tag tone="warn">partial</Tag
							>{:else if s.status === 'empty'}<Tag tone="bad">empty</Tag>{:else}<Tag tone="ok"
								>ok</Tag
							>{/if}
						<span class="block text-[12px] text-muted-foreground">{s.changed}</span>
					</Table.Cell>
				</Table.Row>
			{/each}
		</Table.Body>
	</Table.Root>
</div>

<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Add IP set</Sheet.Title>
			<Sheet.Description
				>A new set is used nowhere until you pick it in a rule, the firewall, DNS or a service.</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-5 overflow-y-auto px-4 pb-4">
			<div class="space-y-1.5">
				<Label for="sn">Name</Label><Input id="sn" class="font-mono" value="family-devices" />
				<p class="text-[12.5px] text-muted-foreground">Lowercase letters, digits and dashes.</p>
			</div>
			<div class="space-y-1.5">
				<Label for="sd">Description</Label><Textarea id="sd" rows={2} />
			</div>
			<div class="rounded-xl border p-3">
				<div class="mb-3 text-[13px] font-medium">First item</div>
				<IpItemEditor type="pulse" />
			</div>
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button href="/design/ip-sets/s-trusted">Create set</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
