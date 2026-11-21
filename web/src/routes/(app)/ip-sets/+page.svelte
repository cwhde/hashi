<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import IpItemEditor from '#lib/components/hashi/ipsets/IpItemEditor.svelte';
	import AddFeedSheet from '#lib/components/hashi/ipsets/AddFeedSheet.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Textarea } from '#lib/components/ui/textarea/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as Table from '#lib/components/ui/table/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { CircleAlert, Lock, Plus, Rss, Search } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { dateTime } from '#lib/format.ts';
	import {
		addressHref,
		count,
		elementInput,
		emptyItemForm,
		kindLabel,
		listSets,
		looksLikeAddress,
		type IPSet
	} from '#lib/ipsets.ts';

	// 05a: every place that needs an address points at a set; blocklists (06 §8) are sets of kind feed.
	let feedOpen = $state(false);
	let sets = $state<IPSet[]>([]);
	let usedBy = $state<Record<string, Schemas['IPSetConsumer'][]>>({});
	let loaded = $state(false);
	let loadError = $state('');

	let q = $state('');
	let kind = $state<string>('all');
	let containing = $state<string[] | null>(null);
	const isAddress = $derived(looksLikeAddress(q) && !q.includes('/'));
	// Sorted by kind, then name (bug 121): Hashi's own sets, feeds, dynamic sets, then yours.
	const kindOrder: Record<string, number> = { system: 0, feed: 1, dynamic: 2, user: 3 };
	const rows = $derived(
		sets
			.filter(
				(s) =>
					(kind === 'all' || s.kind === kind) &&
					(isAddress && containing
						? containing.includes(s.id)
						: (s.slug + ' ' + s.name + ' ' + s.description).toLowerCase().includes(q.toLowerCase()))
			)
			.toSorted(
				(a, b) =>
					(kindOrder[a.kind] ?? 9) - (kindOrder[b.kind] ?? 9) ||
					a.slug.localeCompare(b.slug, undefined, { sensitivity: 'base' })
			)
	);

	async function load() {
		sets = await listSets();
		const entries = await Promise.all(
			sets.map(async (s) => {
				const r = await api.GET('/api/ip-sets/{id}/consumers', { params: { path: { id: s.id } } });
				return [s.id, r.data?.consumers ?? []] as const;
			})
		);
		usedBy = Object.fromEntries(entries);
		loaded = true;
	}
	onMount(() => load().catch((e) => (loadError = problemText(e))));

	// Typing an address lists the sets that contain it (the address page has the details).
	$effect(() => {
		const a = q.trim();
		if (!isAddress) {
			containing = null;
			return;
		}
		const t = setTimeout(async () => {
			const r = await api.GET('/api/addresses/{address}', { params: { path: { address: a } } });
			containing = r.data ? r.data.sets.map((m) => m.set.id) : [];
		}, 250);
		return () => clearTimeout(t);
	});

	// ---- add ----
	let addOpen = $state(false);
	let form = $state({ slug: '', description: '', withItem: true });
	let item = $state(emptyItemForm());
	let formError = $state('');
	let busy = $state(false);
	// An error belongs to the input that caused it: any change to the form clears it (bug 67).
	$effect(() => {
		JSON.stringify([form, item]);
		untrack(() => (formError = ''));
	});

	function openAdd() {
		form = { slug: '', description: '', withItem: true };
		item = emptyItemForm();
		formError = '';
		addOpen = true;
	}

	async function create() {
		busy = true;
		formError = '';
		try {
			const slug = form.slug.trim().toLowerCase();
			// One step: a refused first item creates nothing and its reason shows here (bug 66).
			const r = await api.POST('/api/ip-sets', {
				body: {
					slug,
					name: slug,
					description: form.description,
					first_item: form.withItem ? elementInput(item) : undefined
				}
			});
			if (r.error || !r.data) throw r.error;
			await goto(`/ip-sets/${r.data.id}`);
		} catch (e) {
			formError = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>IP sets · Hashi</title></svelte:head>

<PageHeader title="IP sets">
	{#snippet actions()}
		<Button variant="outline" onclick={() => (feedOpen = true)}><Rss />Add blocklist</Button>
		<Button onclick={openAdd}><Plus />Add set</Button>
	{/snippet}
</PageHeader>

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-md text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !loaded}
	<Skeleton class="h-80 rounded-2xl" />
{:else}
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
				<ToggleGroup.Item value="user">{kindLabel.user}</ToggleGroup.Item>
				<ToggleGroup.Item value="system">{kindLabel.system}</ToggleGroup.Item>
				<ToggleGroup.Item value="feed">{kindLabel.feed}</ToggleGroup.Item>
				<ToggleGroup.Item value="dynamic">{kindLabel.dynamic}</ToggleGroup.Item>
			</ToggleGroup.Root>
			<span class="ml-auto text-sm text-muted-foreground">{rows.length} of {sets.length}</span>
		</div>
		{#if isAddress}
			<div class="flex flex-wrap items-center gap-2 border-b bg-surface-2/40 px-4 py-2.5 text-md">
				<span>Sets that contain <span class="font-mono">{q.trim()}</span> are listed below.</span>
				<a class="font-medium underline" href={addressHref(q.trim())}>Open the address page</a>
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
					{@const used = usedBy[s.id] ?? []}
					<Table.Row>
						<Table.Cell class="max-w-[28rem] pl-4">
							<a href="/ip-sets/{s.id}" class="group block">
								<span class="flex items-center gap-1.5 font-medium group-hover:underline"
									>{s.slug}{#if !s.editable}<Lock
											class="size-3.5 text-muted-foreground"
											aria-label="Not editable"
										/>{/if}</span
								>
								<span class="block truncate text-sm text-muted-foreground">{s.description}</span>
							</a>
						</Table.Cell>
						<Table.Cell class="hidden md:table-cell">
							<Tag>{kindLabel[s.kind]}</Tag>
							{#if s.address_set}<Tag tone="info">addresses only</Tag>{/if}
						</Table.Cell>
						<Table.Cell class="tnum hidden text-right sm:table-cell">
							{count(s.element_count, 'item')}
							<span class="block text-xs text-muted-foreground"
								>{s.v4_count.toLocaleString()} v4 · {s.v6_count.toLocaleString()} v6</span
							>
						</Table.Cell>
						<Table.Cell class="hidden max-w-[22rem] lg:table-cell">
							{#if used.length}
								<span class="block truncate text-md">{used[0]?.name}</span>
								{#if used.length > 1}<span class="text-xs text-muted-foreground"
										>and {used.length - 1} more</span
									>{/if}
							{:else}<span class="text-muted-foreground">Not used</span>{/if}
						</Table.Cell>
						<Table.Cell>
							{#if s.resolution_status === 'partial'}<Tag tone="warn">partial</Tag
								>{:else if s.resolution_status === 'empty'}<Tag tone="bad">empty</Tag>{:else}<Tag
									tone="ok">ok</Tag
								>{/if}
							<span class="block text-xs text-muted-foreground">{dateTime(s.updated_at)}</span>
						</Table.Cell>
					</Table.Row>
				{:else}
					<Table.Row>
						<Table.Cell colspan={5} class="py-10 text-center text-md text-muted-foreground">
							{#if isAddress}No set contains {q.trim()}.{:else}No set matches “{q}”.{/if}
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</div>
{/if}

<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Add IP set</Sheet.Title>
		</Sheet.Header>
		<div class="space-y-5 overflow-y-auto px-4 pb-4">
			<div class="space-y-1.5">
				<Label for="sn">Name</Label><Input
					id="sn"
					class="font-mono"
					bind:value={form.slug}
					placeholder="family-devices"
				/>
				{#if form.slug && !/^[a-z0-9-]+$/.test(form.slug)}<p class="text-sm text-bad-text">
						Lowercase letters, digits and dashes only.
					</p>{/if}
			</div>
			<div class="space-y-1.5">
				<Label for="sd">Description</Label><Textarea
					id="sd"
					rows={2}
					bind:value={form.description}
				/>
			</div>
			<div class="rounded-xl border p-3">
				<div class="mb-3 flex items-center justify-between gap-2">
					<Label for="fi" class="text-md font-medium">First item</Label>
					<Switch id="fi" bind:checked={form.withItem} />
				</div>
				{#if form.withItem}<IpItemEditor bind:form={item} {sets} self="" />{/if}
			</div>
			{#if formError}<p role="alert" class="text-md text-bad-text">{formError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button onclick={create} disabled={busy || !form.slug.trim()}>Create set</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

<AddFeedSheet
	bind:open={feedOpen}
	onadded={(id) => {
		feedOpen = false;
		goto(`/ip-sets/${id}`);
	}}
/>
