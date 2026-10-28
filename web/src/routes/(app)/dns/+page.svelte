<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import ImportTable from '$lib/components/hashi/dns/ImportTable.svelte';
	import RecordEditor, { type EntryForm } from '$lib/components/hashi/dns/RecordEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tabs from '$lib/components/ui/tabs';
	import { CircleAlert, FlaskConical, Lock, Pencil, Plus, RefreshCw, Search } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { pendingPlans } from '$lib/sync-pending.svelte';
	import { outcomeText } from '$lib/sync';
	import {
		decisionBody,
		initialDecisions,
		rrsetKey,
		ttlText,
		type Decision,
		type ImportRow,
		type RRSet
	} from '$lib/dns';

	// 03-dns.md §3–7, 10b §3: Records (observed RRSets with Hashi's ownership, manual entries edited inline; every save
	// creates a plan) and Import.
	type Zone = Schemas['DNSZone'];
	let zone = $state<Zone | null>(null);
	let rrsets = $state<RRSet[]>([]);
	let importRows = $state<ImportRow[]>([]);
	let decisions = $state<Record<string, Decision>>({});
	let loaded = $state(false);
	let loadError = $state('');
	let error = $state('');
	let saved = $state('');
	let busy = $state<'' | 'refresh' | 'save' | 'entry' | 'probe'>('');
	// Plain-language outcome of the last action, with a link when something waits for approval.
	let outcome = $state<{ text: string; review: boolean } | null>(null);
	// The row being edited: an RRSet key, or "new" for the add form.
	let editing = $state<string | null>(null);

	let q = $state('');
	let showGenerated = $state(false);

	// The tab lives in the URL (?tab=import) so a reload or a shared link opens the same tool.
	let tab = $state(page.url.searchParams.get('tab') === 'import' ? 'import' : 'records');
	const setTab = (t: string) =>
		goto(t === 'records' ? '?' : `?tab=${t}`, {
			replaceState: true,
			keepFocus: true,
			noScroll: true
		});

	const newCount = $derived(importRows.filter((r) => !r.decision).length);
	const dirty = $derived(
		importRows.some((r) => (r.decision ?? 'leave') !== decisions[rrsetKey(r.rrset)] || !r.decision)
	);
	const rows = $derived(
		rrsets.filter((r) => {
			if (!showGenerated && r.origin === 'system') return false;
			const needle = q.trim().toLowerCase();
			return !needle || `${r.name} ${r.type} ${r.values.join(' ')}`.toLowerCase().includes(needle);
		})
	);

	function setImport(t: Schemas['DNSImportTable']) {
		importRows = t.rows;
		decisions = initialDecisions(t.rows);
	}

	async function load() {
		const zs = await api.GET('/api/dns/zones');
		if (zs.error || !zs.data) throw zs.error;
		zone = zs.data.zones[0] ?? null;
		if (zone) {
			const [rs, imp] = await Promise.all([
				api.GET('/api/dns/zones/{id}/rrsets', { params: { path: { id: zone.id } } }),
				api.GET('/api/dns/import/{zoneId}', { params: { path: { zoneId: zone.id } } })
			]);
			if (rs.error || !rs.data) throw rs.error;
			if (imp.error || !imp.data) throw imp.error;
			zone = rs.data.zone;
			rrsets = rs.data.rrsets;
			setImport(imp.data);
		}
		loaded = true;
	}
	onMount(() => load().catch((e) => (loadError = problemText(e))));

	async function refresh() {
		if (!zone) return;
		error = '';
		busy = 'refresh';
		try {
			const res = await api.POST('/api/dns/zones/{id}/refresh', {
				params: { path: { id: zone.id } }
			});
			if (res.error) throw res.error;
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = '';
		}
	}

	async function save() {
		if (!zone) return;
		error = saved = '';
		busy = 'save';
		try {
			const res = await api.PUT('/api/dns/import/{zoneId}/decisions', {
				params: { path: { zoneId: zone.id } },
				body: decisionBody(importRows, decisions)
			});
			if (res.error || !res.data) throw res.error;
			setImport(res.data);
			saved = 'Decisions saved.';
			// Removals are their own high-risk plan (03 §4); imports go out with the next records plan.
			const prune = await api.POST('/api/dns/prune', { body: { zone_id: zone.id } });
			if (prune.error) throw prune.error;
			const plan = await api.POST('/api/dns/plan', { body: { zone_id: zone.id } });
			if (plan.error) throw plan.error;
			if (prune.data?.plan) {
				saved = 'Decisions saved.';
				outcome = {
					text: `${prune.data.plan.items.length} records are marked for removal. Removing them needs your approval.`,
					review: true
				};
			}
			await pendingPlans.refresh();
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = '';
		}
	}

	// What the admin sees after saving an entry: applied, waiting for approval, or why planning failed.
	function describe(res: Schemas['DNSRecordSaved'], verb: string) {
		const p = res.plan;
		if (res.plan_error) {
			outcome = {
				text: `${verb}, but the change is not applied yet: ${res.plan_error} The next sync tries again.`,
				review: false
			};
		} else if (!p) {
			outcome = { text: `${verb}. Nothing changes at the provider.`, review: false };
		} else if (p.status === 'pending') {
			outcome = { text: `${verb}. Deleting it at the provider needs your approval.`, review: true };
		} else {
			outcome = { text: `${verb}. ${outcomeText(p)}`, review: false };
		}
	}

	async function saveEntry(f: EntryForm, entry?: Schemas['DNSRecord']) {
		if (!zone) return;
		error = '';
		outcome = null;
		busy = 'entry';
		try {
			const body = {
				ttl: f.ttl,
				values: f.values,
				display_name: f.display_name,
				dashboard: f.dashboard,
				monitoring: f.monitoring
			};
			const res = entry
				? await api.PATCH('/api/dns/records/{id}', { params: { path: { id: entry.id } }, body })
				: await api.POST('/api/dns/records', {
						body: { ...body, zone_id: zone.id, name: f.name, type: f.type }
					});
			if (res.error || !res.data) throw res.error;
			describe(res.data, `${res.data.record.name} ${res.data.record.type} saved`);
			editing = null;
			await pendingPlans.refresh();
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = '';
		}
	}

	async function deleteEntry(entry: Schemas['DNSRecord']) {
		error = '';
		outcome = null;
		busy = 'entry';
		try {
			const res = await api.DELETE('/api/dns/records/{id}', { params: { path: { id: entry.id } } });
			if (res.error || !res.data) throw res.error;
			describe(res.data, `${entry.name} ${entry.type} removed from Hashi`);
			editing = null;
			await pendingPlans.refresh();
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = '';
		}
	}

	// Write test (02 §1.3, invariant 4): plans a probe TXT; nothing is written before approval on Pending changes.
	async function probe() {
		if (!zone) return;
		error = '';
		outcome = null;
		busy = 'probe';
		try {
			const res = await api.POST('/api/dns/probe', { body: { zone_id: zone.id } });
			if (res.error) throw res.error;
			outcome = {
				text: 'Write test planned: once you approve, Hashi creates a _hashi-test TXT record, reads it back and deletes it again.',
				review: true
			};
			await pendingPlans.refresh();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = '';
		}
	}

	const editable = (r: RRSet) =>
		!!r.entry && (r.entry.origin === 'user' || r.entry.origin === 'imported') && !r.protected;

	const fmtTime = (iso?: string) =>
		iso
			? new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
			: 'never';
</script>

{#snippet failure(msg: string)}
	<p
		role="alert"
		class="mb-4 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{msg}
	</p>
{/snippet}

<svelte:head><title>DNS · Hashi</title></svelte:head>

<PageHeader
	title="DNS"
	description="Hashi manages one zone. Records it does not own are shown but never changed outside the import and prune flow."
>
	{#snippet actions()}
		{#if zone}
			<span class="text-[12.5px] text-muted-foreground"
				><Mono>{zone.name}</Mono> · Hetzner · read {fmtTime(zone.last_refreshed_at)}</span
			>
			<Button variant="outline" size="sm" onclick={probe} disabled={!!busy}
				><FlaskConical />Test write access</Button
			>
			<Button variant="outline" size="sm" onclick={refresh} disabled={!!busy}
				><RefreshCw class={busy === 'refresh' ? 'animate-spin' : ''} />{busy === 'refresh'
					? 'Reading…'
					: 'Refresh'}</Button
			>
		{/if}
	{/snippet}
</PageHeader>

{#if loadError}
	{@render failure(loadError)}
{:else if !loaded}
	<Skeleton class="mb-4 h-9 w-56" /><Skeleton class="h-96 rounded-2xl" />
{:else if !zone}
	<Panel title="No zone yet">
		<p class="text-[13px] text-muted-foreground">
			The DNS provider is connected in setup step 3. Until then Hashi has no zone to show.
		</p>
	</Panel>
{:else}
	{#if zone.last_refresh_error}
		{@render failure(
			`The last read of ${zone.name} failed: ${zone.last_refresh_error} The records below are from the last successful read.`
		)}
	{/if}
	{#if error}{@render failure(error)}{/if}
	{#if outcome}
		<p
			role="status"
			class="mb-4 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-2xl border bg-card p-4 text-[13px]"
		>
			<span class="flex-1">{outcome.text}</span>
			{#if outcome.review}<Button size="sm" variant="outline" href="/changes">Review changes</Button
				>{/if}
		</p>
	{/if}

	<Tabs.Root bind:value={tab} onValueChange={setTab}>
		<Tabs.List class="mb-4">
			<Tabs.Trigger value="records">Records</Tabs.Trigger>
			<Tabs.Trigger value="import"
				>Import{#if newCount > 0}<Tag class="ml-1">{newCount} new</Tag>{/if}</Tabs.Trigger
			>
		</Tabs.List>

		<Tabs.Content value="records">
			<div class="rounded-2xl border bg-card">
				<div class="flex flex-wrap items-center gap-3 border-b px-3 py-2.5">
					<div class="relative w-full max-w-72">
						<Search
							class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
						/>
						<Input
							bind:value={q}
							placeholder="Filter records"
							class="pl-8"
							aria-label="Filter records"
						/>
					</div>
					<label class="flex items-center gap-2 text-[13px]"
						><Switch bind:checked={showGenerated} />Show generated</label
					>
					<span class="tnum ml-auto text-[12.5px] text-muted-foreground"
						>{rows.length} of {rrsets.length}</span
					>
					<Button size="sm" onclick={() => (editing = 'new')} disabled={editing === 'new'}
						><Plus />Add record</Button
					>
				</div>
				{#if editing === 'new'}
					<RecordEditor
						zone={zone.name}
						busy={busy === 'entry'}
						onSave={(f) => saveEntry(f)}
						onCancel={() => (editing = null)}
					/>
				{/if}
				<div role="table" aria-label="DNS records" class="text-[13px]">
					<div
						role="row"
						class="hidden grid-cols-[minmax(8rem,1fr)_4.5rem_6rem_minmax(10rem,2fr)_7rem_2.5rem] gap-3 border-b px-4 py-2 text-[12px] font-medium text-muted-foreground md:grid"
					>
						<span role="columnheader">Name</span><span role="columnheader">Type</span><span
							role="columnheader">TTL</span
						><span role="columnheader">Value</span><span role="columnheader">Owner</span><span
							role="columnheader"><span class="sr-only">Edit</span></span
						>
					</div>
					{#each rows as r (rrsetKey(r))}
						{@const locked = r.protected || r.type === 'NS' || r.type === 'SOA'}
						<div
							role="row"
							class="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-x-3 gap-y-0.5 border-b px-4 py-2 last:border-b-0 hover:bg-surface-2/40 md:grid-cols-[minmax(8rem,1fr)_4.5rem_6rem_minmax(10rem,2fr)_7rem_2.5rem] {editing ===
							rrsetKey(r)
								? 'bg-surface-2/40'
								: ''}"
						>
							<span class="min-w-0 truncate" role="cell"><Mono>{r.name}</Mono></span>
							<span class="font-medium" role="cell">{r.type}</span>
							<span class="tnum hidden text-muted-foreground md:block" role="cell"
								>{ttlText(r)}</span
							>
							<span class="col-span-3 min-w-0 md:col-span-1" role="cell"
								>{#each r.values as v, i (i)}<span class="block truncate" title={v}
										><Mono class="text-muted-foreground md:text-foreground">{v}</Mono></span
									>{/each}</span
							>
							<span class="hidden md:block" role="cell">
								{#if locked}<Tag
										><Lock class="size-3" />{r.protected ? 'provider lock' : 'protected'}</Tag
									>{:else}<Tag tone={r.origin === 'unmanaged' ? 'neutral' : 'accent'}
										>{r.origin}</Tag
									>{/if}
								{#if r.pending}<Tag tone="warn">pending</Tag>{/if}
							</span>
							<span role="cell">
								{#if editable(r)}
									<Button
										variant="ghost"
										size="icon-sm"
										aria-label="Edit {r.name} {r.type}"
										onclick={() => (editing = editing === rrsetKey(r) ? null : rrsetKey(r))}
										><Pencil /></Button
									>
								{/if}
							</span>
						</div>
						{#if editing === rrsetKey(r) && r.entry}
							{@const entry = r.entry}
							<RecordEditor
								{entry}
								zone={zone.name}
								busy={busy === 'entry'}
								onSave={(f) => saveEntry(f, entry)}
								onDelete={() => deleteEntry(entry)}
								onCancel={() => (editing = null)}
							/>
						{/if}
					{:else}
						<p class="px-4 py-6 text-center text-muted-foreground">
							{rrsets.length === 0
								? 'The zone has no records yet.'
								: 'No record matches the filter.'}
						</p>
					{/each}
				</div>
			</div>
			<p class="mt-2 text-[12.5px] text-muted-foreground">
				NS and SOA are never changed by Hashi. Records Hashi does not own are shown as the provider
				holds them; import one on the Import tab to edit it here.
			</p>
		</Tabs.Content>

		<Tabs.Content value="import">
			<div class="mb-3 flex flex-wrap items-center gap-3">
				<p class="flex-1 text-[13px] text-muted-foreground">
					Leave records alone (default), take them over as managed entries, or mark them for
					removal.
				</p>
				{#if saved && !dirty}<span class="text-[12.5px] text-ok-text" aria-live="polite"
						>{saved}</span
					>{/if}
				<Button size="sm" onclick={save} disabled={!!busy || !dirty}
					>{busy === 'save' ? 'Saving…' : 'Save decisions'}</Button
				>
			</div>
			<ImportTable rows={importRows} bind:decisions />
			<p class="mt-2 text-[12.5px] text-muted-foreground">
				Removals become a separate high-risk plan that lists every record and asks for your passkey.
			</p>
		</Tabs.Content>
	</Tabs.Root>
{/if}
