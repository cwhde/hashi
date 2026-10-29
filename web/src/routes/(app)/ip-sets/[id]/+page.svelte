<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import IpItemView from '$lib/components/hashi/ipsets/IpItemView.svelte';
	import IpItemEditor from '$lib/components/hashi/ipsets/IpItemEditor.svelte';
	import AddressLink from '$lib/components/hashi/ipsets/AddressLink.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import {
		ChevronLeft,
		CircleAlert,
		Lock,
		MoreHorizontal,
		Plus,
		Search,
		TriangleAlert
	} from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { dateTime } from '$lib/format';
	import {
		count,
		elementInput,
		emptyItemForm,
		formFromElement,
		kindLabel,
		listSets,
		lockReason,
		type IPSet,
		type IPSetElement
	} from '$lib/ipsets';

	// One IP set (05a): its items, what it resolves to now, who uses it and its audit history.
	const id = $derived(page.params.id ?? '');
	let s = $state<IPSet | null>(null);
	let sets = $state<IPSet[]>([]);
	let elements = $state<IPSetElement[]>([]);
	let total = $state(0);
	let resolved = $state<Schemas['IPSetResolved'] | null>(null);
	let consumers = $state<Schemas['IPSetConsumer'][]>([]);
	let history = $state<Schemas['AuditEvent'][]>([]);
	let loadError = $state('');
	let outcome = $state<{ text: string; bad: boolean } | null>(
		page.url.searchParams.get('first') === 'failed'
			? {
					text: 'The set was created, but its first item was refused. Add it again below.',
					bad: true
				}
			: null
	);

	let q = $state('');
	const usedAsTarget = $derived(consumers.some((c) => c.position === 'target'));

	let nextCursor = $state('');
	// more = true appends the next page (05a §6: large sets are paginated).
	async function loadElements(more = false) {
		const query: { q?: string; cursor?: string } = q.trim() ? { q: q.trim() } : {};
		if (more) query.cursor = nextCursor;
		const r = await api.GET('/api/ip-sets/{id}/elements', { params: { path: { id }, query } });
		if (r.error || !r.data) throw r.error;
		elements = more ? [...elements, ...r.data.elements] : r.data.elements;
		total = r.data.total;
		nextCursor = r.data.next_cursor;
	}

	async function load() {
		const path = { params: { path: { id } } };
		const [one, res, cons, all, hist] = await Promise.all([
			api.GET('/api/ip-sets/{id}', path),
			api.GET('/api/ip-sets/{id}/resolved', path),
			api.GET('/api/ip-sets/{id}/consumers', path),
			listSets(),
			api.GET('/api/activity/audit', { params: { query: { target: `ip_set:${id}`, limit: 50 } } })
		]);
		if (one.error || !one.data) throw one.error;
		s = one.data;
		resolved = res.data ?? null;
		consumers = cons.data?.consumers ?? [];
		sets = all;
		history = hist.data?.events ?? [];
		await loadElements();
	}
	onMount(() => load().catch((e) => (loadError = problemText(e))));

	$effect(() => {
		void q;
		if (!s) return;
		const t = setTimeout(() => loadElements().catch((e) => (loadError = problemText(e))), 200);
		return () => clearTimeout(t);
	});

	// ---- add / edit an item ----
	let itemOpen = $state(false);
	let editing = $state<IPSetElement | null>(null);
	let item = $state(emptyItemForm());
	let itemError = $state('');
	let busy = $state(false);

	function openItem(e: IPSetElement | null) {
		editing = e;
		item = e ? formFromElement(e) : emptyItemForm();
		itemError = '';
		itemOpen = true;
	}

	async function saveItem() {
		busy = true;
		itemError = '';
		try {
			const body = elementInput(item);
			const r = editing
				? await api.PATCH('/api/ip-sets/{id}/elements/{elementId}', {
						params: { path: { id, elementId: editing.id } },
						body
					})
				: await api.POST('/api/ip-sets/{id}/elements', { params: { path: { id } }, body });
			if (r.error) throw r.error;
			itemOpen = false;
			outcome = { text: editing ? 'Item saved.' : 'Item added.', bad: false };
			await load();
		} catch (e) {
			itemError = problemText(e);
		} finally {
			busy = false;
		}
	}

	async function removeItem(e: IPSetElement) {
		outcome = null;
		const r = await api.DELETE('/api/ip-sets/{id}/elements/{elementId}', {
			params: { path: { id, elementId: e.id } }
		});
		if (r.error) {
			outcome = { text: problemText(r.error), bad: true };
			return;
		}
		outcome = { text: `Removed ${e.label}.`, bad: false };
		await load();
	}

	// ---- rename / delete ----
	let renameOpen = $state(false);
	let meta = $state({ slug: '', description: '' });
	let metaError = $state('');
	function openRename() {
		if (!s) return;
		meta = { slug: s.slug, description: s.description };
		metaError = '';
		renameOpen = true;
	}
	async function saveMeta() {
		busy = true;
		metaError = '';
		try {
			const slug = meta.slug.trim().toLowerCase();
			const r = await api.PUT('/api/ip-sets/{id}', {
				params: { path: { id } },
				body: { slug, name: slug, description: meta.description }
			});
			if (r.error) throw r.error;
			renameOpen = false;
			await load();
		} catch (e) {
			metaError = problemText(e);
		} finally {
			busy = false;
		}
	}

	let delOpen = $state(false);
	async function deleteSet() {
		const r = await api.DELETE('/api/ip-sets/{id}', { params: { path: { id } } });
		delOpen = false;
		if (r.error) {
			outcome = { text: problemText(r.error), bad: true };
			return;
		}
		await goto('/ip-sets');
	}

	const historyText: Record<string, string> = {
		'ip_set.created': 'Created',
		'ip_set.updated': 'Renamed or described',
		'ip_set.element.added': 'Item added',
		'ip_set.element.updated': 'Item changed',
		'ip_set.element.removed': 'Item removed',
		'ip_set.element.expired': 'Item expired'
	};
</script>

<a
	href="/ip-sets"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />IP sets</a
>

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !s}
	<Skeleton class="h-80 rounded-2xl" />
{:else}
	{#if outcome}
		<p
			role={outcome.bad ? 'alert' : 'status'}
			class="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-3 text-[13px] {outcome.bad
				? 'border-bad/30 bg-bad/10 text-bad-text'
				: 'bg-card'}"
		>
			{#if outcome.bad}<CircleAlert class="size-4 shrink-0" />{/if}{outcome.text}
		</p>
	{/if}

	<div class="mb-5 rounded-2xl border bg-card p-4">
		<div class="flex flex-wrap items-start gap-x-6 gap-y-3">
			<div class="min-w-0 flex-1">
				<h1 class="flex flex-wrap items-center gap-2 text-[22px] leading-tight font-semibold">
					<span class="font-mono tracking-[-0.02em]">{s.slug}</span>
					{#if !s.editable}<Tag><Lock class="size-3" />{kindLabel[s.kind].toLowerCase()}</Tag>{/if}
				</h1>
				<p class="text-[13.5px] text-muted-foreground">{s.description}</p>
			</div>
			{#if s.editable}
				<div class="flex gap-2">
					<Button variant="outline" size="sm" onclick={openRename}>Rename</Button>
					<Button size="sm" onclick={() => openItem(null)}><Plus />Add item</Button>
				</div>
			{/if}
		</div>
		{#if !s.editable}<p class="mt-2 text-[12.5px] text-muted-foreground">
				{lockReason[s.kind]}
			</p>{/if}
		<dl class="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 text-[13px] sm:grid-cols-4">
			<div>
				<dt class="text-[12px] text-muted-foreground">Kind</dt>
				<dd>
					{kindLabel[s.kind]}{#if s.address_set}<span class="text-muted-foreground">
							· addresses only</span
						>{/if}
				</dd>
			</div>
			<div>
				<dt class="text-[12px] text-muted-foreground">Resolves to</dt>
				<dd class="tnum">
					{s.v4_count.toLocaleString()} IPv4 · {s.v6_count.toLocaleString()} IPv6 ranges
				</dd>
			</div>
			<div>
				<dt class="text-[12px] text-muted-foreground">Used by</dt>
				<dd>{consumers.length ? count(consumers.length, 'place') : 'Not used'}</dd>
			</div>
			<div>
				<dt class="text-[12px] text-muted-foreground">Status</dt>
				<dd>
					{#if s.resolution_status === 'partial'}<Tag tone="warn">partial</Tag
						>{:else if s.resolution_status === 'empty'}<Tag tone="bad">empty</Tag>{:else}<Tag
							tone="ok">ok</Tag
						>{/if}
				</dd>
			</div>
		</dl>
		{#each s.problems as p (p.element_id + p.message)}
			<p
				class="mt-3 flex items-start gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-[12.5px] text-warn-text"
			>
				<TriangleAlert class="mt-0.5 size-3.5 shrink-0" />{p.path.join(' › ')}: {p.message}
			</p>
		{/each}
	</div>

	<Tabs.Root value="items">
		<Tabs.List class="mb-4">
			<Tabs.Trigger value="items">Items</Tabs.Trigger>
			<Tabs.Trigger value="resolved">Resolved</Tabs.Trigger>
			<Tabs.Trigger value="used">Used by</Tabs.Trigger>
			<Tabs.Trigger value="history">History</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="items">
			<div class="rounded-2xl border bg-card">
				<div class="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
					<div class="relative w-full max-w-80">
						<Search
							class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
						/>
						<Input
							bind:value={q}
							placeholder="Address, prefix or note"
							class="pl-8"
							aria-label="Search items"
						/>
					</div>
					<span class="ml-auto text-[12.5px] text-muted-foreground"
						>{#if total > elements.length}Showing {elements.length} of {total}{:else}{count(
								total,
								'item'
							)}{/if}</span
					>
				</div>
				{#if elements.length}
					<ul class="divide-y">
						{#each elements as e (e.id)}
							<li
								class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-4 py-2.5 md:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_9rem_auto]"
							>
								<IpItemView item={e.item} {sets} />
								<span class="hidden truncate text-[12.5px] text-muted-foreground md:block"
									>{e.note}{#each s.problems.filter((p) => p.element_id === e.id) as p (p.message)}<Tag
											tone="warn">{p.message}</Tag
										>{/each}</span
								>
								<span class="hidden text-[12.5px] text-muted-foreground md:block"
									>{e.expires_at
										? 'expires ' + dateTime(e.expires_at)
										: e.origin === 'system'
											? ''
											: 'added ' + dateTime(e.created_at)}</span
								>
								{#if s.editable}
									<DropdownMenu.Root>
										<DropdownMenu.Trigger>
											{#snippet child({ props })}<Button
													{...props}
													variant="ghost"
													size="icon-sm"
													aria-label="Actions for {e.label}"><MoreHorizontal /></Button
												>{/snippet}
										</DropdownMenu.Trigger>
										<DropdownMenu.Content align="end">
											<DropdownMenu.Item onclick={() => openItem(e)}>Edit</DropdownMenu.Item>
											<DropdownMenu.Separator />
											<DropdownMenu.Item class="text-bad-text" onclick={() => removeItem(e)}
												>Remove</DropdownMenu.Item
											>
										</DropdownMenu.Content>
									</DropdownMenu.Root>
								{:else}<span></span>{/if}
							</li>
						{/each}
					</ul>
					{#if nextCursor}
						<div class="border-t px-4 py-2.5 text-center">
							<Button
								variant="ghost"
								size="sm"
								onclick={() => loadElements(true).catch((e) => (loadError = problemText(e)))}
								>Show more</Button
							>
						</div>
					{/if}
				{:else}
					<div class="px-4 py-10 text-center text-[13px] text-muted-foreground">
						{#if q}Nothing in this set matches “{q}”.{:else}This set is empty, so it matches nobody.
							{#if s.editable}<Button
									size="sm"
									variant="outline"
									class="ml-2"
									onclick={() => openItem(null)}>Add item</Button
								>{/if}{/if}
					</div>
				{/if}
			</div>
			{#if usedAsTarget}<p class="mt-2 text-[12.5px] text-muted-foreground">
					Used as a DNS value, upstream or SSH target, so only addresses can be added. If it ever
					resolves to nothing, those places keep their last working address.
				</p>{/if}
		</Tabs.Content>

		<Tabs.Content value="resolved">
			<Panel
				flush
				title="What this set means right now"
				description="Merged ranges after resolving hosts, DNS names and nested sets"
			>
				{#if resolved?.prefixes.length}
					<ul class="divide-y text-[13px]">
						{#each resolved.prefixes as p (p)}
							<li class="px-4 py-2"><Mono>{p}</Mono></li>
						{/each}
					</ul>
					{#if resolved.next_cursor}<p class="px-4 py-2 text-[12.5px] text-muted-foreground">
							Showing the first {resolved.prefixes.length} of {resolved.v4_count +
								resolved.v6_count} ranges.
						</p>{/if}
				{:else}
					<p class="px-4 py-6 text-[13px] text-muted-foreground">
						Resolves to nothing right now, so it matches nobody.
					</p>
				{/if}
			</Panel>
			{#if resolved?.address_set && resolved.addresses.length}
				<Panel
					flush
					class="mt-4"
					title="Addresses for targets"
					description="In item order: the first is used first when this set is a connection target"
				>
					<ul class="divide-y text-[13px]">
						{#each resolved.addresses as a, i (a)}
							<li class="flex items-center gap-3 px-4 py-2">
								<AddressLink address={a} />{#if i === 0}<Tag>primary</Tag>{/if}
							</li>
						{/each}
					</ul>
				</Panel>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="used">
			<Panel flush title="Used by" description="A set cannot be deleted while anything uses it">
				{#if consumers.length}
					<ul class="divide-y text-[13px]">
						{#each consumers as u (u.type + u.id)}
							<li class="grid grid-cols-[10rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5">
								<span class="text-muted-foreground"
									>{u.type === 'ip_set' ? 'Included in set' : u.type}</span
								>
								{#if u.type === 'ip_set'}<a
										href="/ip-sets/{u.id}"
										class="truncate font-medium hover:underline">{u.name}</a
									>{:else}<span class="truncate font-medium">{u.name}</span>{/if}
								<Tag tone={u.position === 'target' ? 'info' : 'neutral'}
									>{u.position === 'target' ? 'sends traffic here' : 'matches requests'}</Tag
								>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="px-4 py-6 text-[13px] text-muted-foreground">
						Nothing uses this set yet. Pick it in a rule, the firewall policy, a DNS record or a
						service.
					</p>
				{/if}
			</Panel>
			{#if s.editable}
				<div class="mt-4 flex justify-end">
					<Button
						variant="ghost"
						class="text-bad-text"
						disabled={consumers.length > 0}
						onclick={() => (delOpen = true)}>Delete set</Button
					>
				</div>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="history">
			<Panel flush title="History" description="From the audit log">
				{#if history.length}
					<ul class="divide-y text-[13px]">
						{#each history as h (h.id)}
							<li class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3 px-4 py-2.5">
								<span class="text-muted-foreground">{dateTime(h.at)}</span><span
									>{historyText[h.action] ?? h.action}{#if h.detail.note}<span
											class="text-muted-foreground"
										>
											· {h.detail.note}</span
										>{/if}</span
								>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="px-4 py-6 text-[13px] text-muted-foreground">No changes recorded.</p>
				{/if}
			</Panel>
		</Tabs.Content>
	</Tabs.Root>
{/if}

<Sheet.Root bind:open={itemOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>{editing ? 'Edit item in' : 'Add item to'} {s?.slug}</Sheet.Title>
			<Sheet.Description>Takes effect within seconds everywhere this set is used.</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-3 overflow-y-auto px-4 pb-4">
			<IpItemEditor bind:form={item} {sets} self={id} {usedAsTarget} />
			{#if itemError}<p role="alert" class="text-[13px] text-bad-text">{itemError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (itemOpen = false)}>Cancel</Button>
			<Button onclick={saveItem} disabled={busy}>{editing ? 'Save item' : 'Add item'}</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

<Sheet.Root bind:open={renameOpen}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>Rename {s?.slug}</Sheet.Title>
			<Sheet.Description>Places that use the set follow the new name.</Sheet.Description>
		</Sheet.Header>
		<div class="space-y-4 px-4 pb-4">
			<div class="space-y-1.5">
				<Label for="rs">Name</Label><Input id="rs" class="font-mono" bind:value={meta.slug} />
				<p class="text-[12.5px] text-muted-foreground">Lowercase letters, digits and dashes.</p>
			</div>
			<div class="space-y-1.5">
				<Label for="rd">Description</Label><Textarea
					id="rd"
					rows={2}
					bind:value={meta.description}
				/>
			</div>
			{#if metaError}<p role="alert" class="text-[13px] text-bad-text">{metaError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (renameOpen = false)}>Cancel</Button>
			<Button onclick={saveMeta} disabled={busy}>Save</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

<AlertDialog.Root bind:open={delOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete {s?.slug}?</AlertDialog.Title>
			<AlertDialog.Description
				>Nothing uses it. Its items are deleted with it; this cannot be undone.</AlertDialog.Description
			>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action onclick={deleteSet}>Delete set</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
