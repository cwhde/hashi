<script lang="ts">
	import { hrefWithTab, tabFrom } from '#lib/url-tab.ts';
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { consumerOf } from '#lib/consumers.ts';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import IpItemView from '#lib/components/hashi/ipsets/IpItemView.svelte';
	import IpItemEditor from '#lib/components/hashi/ipsets/IpItemEditor.svelte';
	import AddressLink from '#lib/components/hashi/ipsets/AddressLink.svelte';
	import DynamicEntries from '#lib/components/hashi/security/DynamicEntries.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Textarea } from '#lib/components/ui/textarea/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as Tabs from '#lib/components/ui/tabs/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.ts';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.ts';
	import {
		ChevronLeft,
		CircleAlert,
		Lock,
		MoreHorizontal,
		Plus,
		Search,
		TriangleAlert
	} from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { country, dateTime } from '#lib/format.ts';
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
	} from '#lib/ipsets.ts';

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
	let outcome = $state<{ text: string; bad: boolean } | null>(null);

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

	// Large feeds resolve to thousands of ranges; the next page is appended (bug 89).
	async function moreResolved() {
		if (!resolved?.next_cursor) return;
		const r = await api.GET('/api/ip-sets/{id}/resolved', {
			params: { path: { id }, query: { cursor: resolved.next_cursor } }
		});
		if (r.error || !r.data) throw r.error;
		resolved = { ...r.data, prefixes: [...resolved.prefixes, ...r.data.prefixes] };
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
	// An error belongs to the input that caused it: any change to the form clears it (bug 67).
	$effect(() => {
		JSON.stringify(item);
		untrack(() => (itemError = ''));
	});

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
			const target = editing;
			await withReauth(() =>
				target
					? api.PATCH('/api/ip-sets/{id}/elements/{elementId}', {
							params: { path: { id, elementId: target.id } },
							body
						})
					: api.POST('/api/ip-sets/{id}/elements', { params: { path: { id } }, body })
			);
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
		try {
			await withReauth(() =>
				api.DELETE('/api/ip-sets/{id}/elements/{elementId}', {
					params: { path: { id, elementId: e.id } }
				})
			);
		} catch (err) {
			outcome = { text: problemText(err), bad: true };
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
		delOpen = false;
		try {
			await withReauth(() => api.DELETE('/api/ip-sets/{id}', { params: { path: { id } } }));
		} catch (err) {
			outcome = { text: problemText(err), bad: true };
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
	// The open tab lives in the address (bug 41).
	const tabs = ['items', 'resolved', 'used', 'history'];
	let tab = $state(tabFrom(page.url, tabs));
	const setTab = (t: string) =>
		goto(hrefWithTab(page.url, tabs, t), { replace: true, reset: false });
</script>

<svelte:head><title>{s?.slug ?? 'IP set'} · Hashi</title></svelte:head>

<a
	href="/ip-sets"
	class="mb-3 inline-flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />IP sets</a
>

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-md text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !s}
	<Skeleton class="h-80 rounded-2xl" />
{:else}
	{#if outcome}
		<p
			role={outcome.bad ? 'alert' : 'status'}
			class="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-3 text-md {outcome.bad
				? 'border-bad/30 bg-bad/10 text-bad-text'
				: 'bg-card'}"
		>
			{#if outcome.bad}<CircleAlert class="size-4 shrink-0" />{/if}{outcome.text}
		</p>
	{/if}

	<div class="mb-5 rounded-2xl border bg-card p-4">
		<div class="flex flex-wrap items-start gap-x-6 gap-y-3">
			<div class="min-w-0 flex-1">
				<h1 class="flex flex-wrap items-center gap-2 text-title leading-tight font-semibold">
					<span class="font-mono tracking-[-0.02em]">{s.slug}</span>
					{#if !s.editable}<span title={lockReason[s.kind]} class="inline-flex"
							><Tag><Lock class="size-3" />{kindLabel[s.kind].toLowerCase()}</Tag></span
						>{/if}
				</h1>
				{#if (s.kind === 'user' || s.kind === 'feed') && s.description}<p
						class="text-md text-muted-foreground"
					>
						{s.description}
					</p>{/if}
			</div>
			{#if s.editable}
				<div class="flex gap-2">
					<Button variant="outline" size="sm" onclick={openRename}>Rename</Button>
					<Button size="sm" onclick={() => openItem(null)}><Plus />Add item</Button>
				</div>
			{/if}
		</div>
		<dl class="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 text-md sm:grid-cols-4">
			<div>
				<dt class="text-xs text-muted-foreground">Kind</dt>
				<dd>
					{kindLabel[s.kind]}{#if s.address_set}{' '}<span class="text-muted-foreground">
							· addresses only</span
						>{/if}
				</dd>
			</div>
			<div>
				<dt class="text-xs text-muted-foreground">Resolves to</dt>
				<dd class="tnum">
					{s.v4_count.toLocaleString()} IPv4 · {s.v6_count.toLocaleString()} IPv6 ranges
				</dd>
			</div>
			<div>
				<dt class="text-xs text-muted-foreground">Used by</dt>
				<dd>{consumers.length ? count(consumers.length, 'place') : 'Not used'}</dd>
			</div>
			<div>
				<dt class="text-xs text-muted-foreground">Status</dt>
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
				class="mt-3 flex items-start gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-sm text-warn-text"
			>
				<TriangleAlert class="mt-0.5 size-3.5 shrink-0" />{p.path.join(' › ')}: {p.message}
			</p>
		{/each}
	</div>

	<Tabs.Root bind:value={tab} onValueChange={setTab}>
		<Tabs.List class="mb-4">
			<Tabs.Trigger value="items">{s.kind === 'dynamic' ? 'Entries' : 'Items'}</Tabs.Trigger>
			{#if s.kind !== 'dynamic'}<Tabs.Trigger value="resolved">Resolved</Tabs.Trigger>{/if}
			<Tabs.Trigger value="used">Used by</Tabs.Trigger>
			<Tabs.Trigger value="history">History</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="items">
			{#if s.kind === 'dynamic' && (s.id === 'sys-dynamic-challenge' || s.id === 'sys-dynamic-blocked')}
				<DynamicEntries setId={s.id} />
			{:else}
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
						<span class="ml-auto text-sm text-muted-foreground"
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
									<span class="min-w-0"
										><IpItemView item={e.item} {sets} />{#if e.country || e.as_name}<span
												class="block truncate text-sm text-muted-foreground"
												title={e.country}
												>{[e.country && country(e.country), e.as_name]
													.filter(Boolean)
													.join(' ')}</span
											>{/if}</span
									>
									<span class="hidden truncate text-sm text-muted-foreground md:block"
										>{e.note}{#each s.problems.filter((p) => p.element_id === e.id) as p (p.message)}<Tag
												tone="warn">{p.message}</Tag
											>{/each}</span
									>
									<span class="hidden text-sm text-muted-foreground md:block"
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
						<div class="px-4 py-10 text-center text-md text-muted-foreground">
							{#if q}Nothing in this set matches “{q}”.{:else}Empty: matches nobody
								{#if s.editable}<Button
										size="sm"
										variant="outline"
										class="ml-2"
										onclick={() => openItem(null)}>Add item</Button
									>{/if}{/if}
						</div>
					{/if}
				</div>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="resolved">
			<Panel flush title="What this set means right now">
				{#if resolved?.prefixes.length}
					<ul class="divide-y text-md">
						{#each resolved.prefixes as p (p)}
							<li class="px-4 py-2"><Mono>{p}</Mono></li>
						{/each}
					</ul>
					{#if resolved.next_cursor}
						<div class="flex items-center justify-between gap-3 border-t px-4 py-2.5">
							<span class="text-sm text-muted-foreground"
								>Showing {resolved.prefixes.length} of {resolved.v4_count + resolved.v6_count} ranges</span
							>
							<Button
								variant="ghost"
								size="sm"
								onclick={() => moreResolved().catch((e) => (loadError = problemText(e)))}
								>Show more</Button
							>
						</div>
					{/if}
				{:else}
					<p class="px-4 py-6 text-md text-warn-text">
						Resolves to nothing right now, so it matches nobody.
					</p>
				{/if}
			</Panel>
			{#if resolved?.address_set && resolved.addresses.length}
				<Panel flush class="mt-4" title="Addresses for targets">
					<ul class="divide-y text-md">
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
			<Panel flush title="Used by">
				{#if consumers.length}
					<ul class="divide-y text-md">
						{#each consumers as u (u.type + u.id)}
							{@const c = consumerOf(u)}
							<li class="grid grid-cols-[10rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5">
								<span class="text-muted-foreground">{c.kind}</span>
								{#if c.href}<a href={c.href} class="truncate font-medium hover:underline"
										>{u.name}</a
									>{:else}<span class="truncate font-medium">{u.name}</span>{/if}
								<Tag tone={u.position === 'target' ? 'info' : 'neutral'}
									>{u.position === 'target' ? 'sends traffic here' : 'matches requests'}</Tag
								>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="px-4 py-6 text-md text-muted-foreground">Nothing uses this set yet.</p>
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
			<Panel flush title="History">
				{#if history.length}
					<ul class="divide-y text-md">
						{#each history as h (h.id)}
							<li class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3 px-4 py-2.5">
								<span class="text-muted-foreground">{dateTime(h.at)}</span><span
									>{historyText[h.action] ?? h.action}{#if h.detail.note}{' '}<span
											class="text-muted-foreground"
										>
											· {h.detail.note}</span
										>{/if}</span
								>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="px-4 py-6 text-md text-muted-foreground">No changes recorded.</p>
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
			{#if itemError}<p role="alert" class="text-md text-bad-text">{itemError}</p>{/if}
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
				{#if meta.slug && !/^[a-z0-9-]+$/.test(meta.slug)}<p class="text-sm text-bad-text">
						Lowercase letters, digits and dashes only.
					</p>{/if}
			</div>
			<div class="space-y-1.5">
				<Label for="rd">Description</Label><Textarea
					id="rd"
					rows={2}
					bind:value={meta.description}
				/>
			</div>
			{#if metaError}<p role="alert" class="text-md text-bad-text">{metaError}</p>{/if}
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
