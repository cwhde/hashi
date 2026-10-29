<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import IpItemEditor from '$lib/components/hashi/ipsets/IpItemEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Switch } from '$lib/components/ui/switch';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import { CircleAlert, Lock, Plus, Search } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { dateTime } from '$lib/format';
	import {
		addressHref,
		count,
		elementInput,
		emptyItemForm,
		kindLabel,
		listSets,
		looksLikeAddress,
		type IPSet
	} from '$lib/ipsets';

	// 05a: every place that needs an address points at a set. Feeds arrive with S10, dynamic sets with S9/S10.
	let sets = $state<IPSet[]>([]);
	let usedBy = $state<Record<string, Schemas['IPSetConsumer'][]>>({});
	let loaded = $state(false);
	let loadError = $state('');

	let q = $state('');
	let kind = $state<string>('all');
	let containing = $state<string[] | null>(null);
	const isAddress = $derived(looksLikeAddress(q) && !q.includes('/'));
	const rows = $derived(
		sets.filter(
			(s) =>
				(kind === 'all' || s.kind === kind) &&
				(isAddress && containing
					? containing.includes(s.id)
					: (s.slug + ' ' + s.name + ' ' + s.description).toLowerCase().includes(q.toLowerCase()))
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
			const r = await api.POST('/api/ip-sets', {
				body: { slug, name: slug, description: form.description }
			});
			if (r.error || !r.data) throw r.error;
			if (form.withItem) {
				const e = await api.POST('/api/ip-sets/{id}/elements', {
					params: { path: { id: r.data.id } },
					body: elementInput(item)
				});
				if (e.error) {
					await goto(`/ip-sets/${r.data.id}?first=failed`);
					return;
				}
			}
			await goto(`/ip-sets/${r.data.id}`);
		} catch (e) {
			formError = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<PageHeader
	title="IP sets"
	description="Named groups of addresses. Firewall allow and block, rules, DNS values, upstreams and SSH targets all point at a set, so one change reaches every place that uses it."
>
	{#snippet actions()}
		<Button onclick={openAdd}><Plus />Add set</Button>
	{/snippet}
</PageHeader>

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
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
			</ToggleGroup.Root>
			<span class="ml-auto text-[12.5px] text-muted-foreground">{rows.length} of {sets.length}</span
			>
		</div>
		{#if isAddress}
			<div
				class="flex flex-wrap items-center gap-2 border-b bg-surface-2/40 px-4 py-2.5 text-[13px]"
			>
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
								<span class="block truncate text-[12.5px] text-muted-foreground"
									>{s.description}</span
								>
							</a>
						</Table.Cell>
						<Table.Cell class="hidden md:table-cell">
							<Tag>{kindLabel[s.kind]}</Tag>
							{#if s.address_set}<Tag tone="info">addresses only</Tag>{/if}
						</Table.Cell>
						<Table.Cell class="tnum hidden text-right sm:table-cell">
							{count(s.element_count, 'item')}
							<span class="block text-[12px] text-muted-foreground"
								>{s.v4_count.toLocaleString()} v4 · {s.v6_count.toLocaleString()} v6</span
							>
						</Table.Cell>
						<Table.Cell class="hidden max-w-[22rem] lg:table-cell">
							{#if used.length}
								<span class="block truncate text-[13px]">{used[0]?.name}</span>
								{#if used.length > 1}<span class="text-[12px] text-muted-foreground"
										>and {used.length - 1} more</span
									>{/if}
							{:else}<span class="text-muted-foreground">Not used</span>{/if}
						</Table.Cell>
						<Table.Cell>
							{#if s.resolution_status === 'partial'}<Tag tone="warn">partial</Tag
								>{:else if s.resolution_status === 'empty'}<Tag tone="bad">empty</Tag>{:else}<Tag
									tone="ok">ok</Tag
								>{/if}
							<span class="block text-[12px] text-muted-foreground">{dateTime(s.updated_at)}</span>
						</Table.Cell>
					</Table.Row>
				{:else}
					<Table.Row>
						<Table.Cell colspan={5} class="py-10 text-center text-[13px] text-muted-foreground">
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
			<Sheet.Description
				>A new set is used nowhere until you pick it in a rule, the firewall, DNS or a service.</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-5 overflow-y-auto px-4 pb-4">
			<div class="space-y-1.5">
				<Label for="sn">Name</Label><Input
					id="sn"
					class="font-mono"
					bind:value={form.slug}
					placeholder="family-devices"
				/>
				<p class="text-[12.5px] text-muted-foreground">Lowercase letters, digits and dashes.</p>
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
					<Label for="fi" class="text-[13px] font-medium">First item</Label>
					<Switch id="fi" bind:checked={form.withItem} />
				</div>
				{#if form.withItem}<IpItemEditor bind:form={item} {sets} self="" />{/if}
			</div>
			{#if formError}<p role="alert" class="text-[13px] text-bad-text">{formError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button onclick={create} disabled={busy || !form.slug.trim()}>Create set</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
