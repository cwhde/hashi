<script lang="ts">
	import { guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import RuleEntryList from '#lib/components/hashi/rules/RuleEntryList.svelte';
	import RuleItemEditor from '#lib/components/hashi/rules/RuleItemEditor.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import { ChevronLeft, CircleAlert, Layers, Lock, Plus, TriangleAlert } from '@lucide/svelte';
	import { withReauth } from '#lib/auth/reauth.ts';
	import ConfirmDelete from '#lib/components/hashi/ConfirmDelete.svelte';
	import { api, problemText } from '#lib/api/client.ts';
	import { pendingPlans } from '#lib/sync-pending.svelte.ts';
	import { listSets, type IPSet } from '#lib/ipsets.ts';
	import {
		allSetId,
		emptyRule,
		listProviders,
		listRuleSets,
		toInput,
		type Provider,
		type RuleEntry,
		type RuleSet
	} from '#lib/rules.ts';

	const id = $derived(page.params.id ?? '');
	let rs = $state<RuleSet | null>(null);
	let entries = $state<RuleEntry[]>([]);
	let sets = $state<IPSet[]>([]);
	let providers = $state<Provider[]>([]);
	let others = $state<RuleSet[]>([]);
	let error = $state('');
	let confirming = $state(false);
	let editing = $state<{ rule: RuleEntry; index: number } | null>(null);
	let attach = $state(false);
	let attachId = $state('');
	// Every change saves at once; only an open rule editor holds unsaved input (bug 40).
	guardUnsaved(() => editing !== null);

	function adopt(r: RuleSet) {
		rs = r;
		entries = r.entries;
	}
	async function load() {
		const r = await api.GET('/api/rule-sets/{id}', { params: { path: { id } } });
		if (r.error) {
			error = problemText(r.error);
			return;
		}
		adopt(r.data);
	}
	onMount(async () => {
		await load();
		[sets, providers, others] = await Promise.all([listSets(), listProviders(), listRuleSets()]);
	});
	async function save() {
		error = '';
		const r = await api.PUT('/api/rule-sets/{id}/entries', {
			params: { path: { id } },
			body: { entries: entries.map(toInput) }
		});
		if (r.error) {
			error = problemText(r.error);
			await load();
			return;
		}
		adopt(r.data.rule_set);
		if (r.data.plans.length) await pendingPlans.refresh();
	}
	function saveRule() {
		if (!editing) return;
		if (editing.index < 0) entries = [...entries, editing.rule];
		else entries[editing.index] = editing.rule;
		editing = null;
		void save();
	}
	function attachSet() {
		const c = others.find((s) => s.id === attachId);
		if (!c) return;
		entries = [...entries, { ...emptyRule(''), type: 'set', child_id: c.id, child_slug: c.slug }];
		attach = false;
		void save();
	}
	async function remove() {
		try {
			await withReauth(() => api.DELETE('/api/rule-sets/{id}', { params: { path: { id } } }));
		} catch (e) {
			error = problemText(e);
			return;
		}
		await goto('/rule-sets');
	}
	const attachable = $derived(others.filter((s) => s.kind === 'user' && s.id !== id));
</script>

<svelte:head><title>{rs?.slug ?? 'Rule set'} · Hashi</title></svelte:head>

<a
	href="/rule-sets"
	class="mb-3 inline-flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Rule sets</a
>
{#if error}
	<p class="mb-3 flex items-start gap-2 text-md text-bad-text">
		<CircleAlert class="mt-0.5 size-3.5 shrink-0" />{error}
	</p>
{/if}
{#if !rs}
	{#if !error}<Skeleton class="h-40 w-full rounded-2xl" />{/if}
{:else}
	<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
		<div class="min-w-64 flex-1">
			<h1 class="flex items-center gap-2 text-title leading-tight font-semibold">
				<span class="font-mono tracking-[-0.02em]">{rs.slug}</span>
				{#if rs.kind === 'global'}<Tag><Lock class="size-3" />global</Tag>{/if}
			</h1>
			{#if rs.kind === 'user' && rs.description}<p class="mt-1 text-md text-muted-foreground">
					{rs.description}
				</p>{/if}
		</div>
		<div class="flex gap-2">
			<Button variant="outline" size="sm" onclick={() => (attach = true)}
				><Layers />Attach rule set</Button
			>
			<Button size="sm" onclick={() => (editing = { rule: emptyRule(allSetId(sets)), index: -1 })}
				><Plus />Add rule</Button
			>
		</div>
	</div>

	{#each rs.warnings as w (w)}
		<p
			class="mb-4 flex items-start gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-sm text-warn-text"
		>
			<TriangleAlert class="mt-0.5 size-3.5 shrink-0" />{w}
		</p>
	{/each}

	<div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
		<Panel flush title="Rules">
			{#if entries.length === 0}<p class="px-4 py-3 text-md text-muted-foreground">
					No rules yet.
				</p>{/if}
			<RuleEntryList
				bind:entries
				{sets}
				onedit={(i) => (editing = { rule: $state.snapshot(entries[i]) as RuleEntry, index: i })}
				onchange={save}
			/>
		</Panel>
		<Panel title="Used by">
			{#if rs.used_by.length}
				<ul class="space-y-2 text-md">
					{#each rs.used_by as u (u.href)}<li>
							<a href={u.href} class="font-medium hover:underline">{u.label}</a>
						</li>{/each}
				</ul>
			{:else if rs.kind === 'global'}<p class="text-md">Every service</p>
			{:else}<p class="text-md text-muted-foreground">Unused</p>{/if}
			{#if rs.kind === 'user'}<Button
					variant="ghost"
					size="sm"
					class="mt-2 -ml-2 text-bad-text"
					disabled={rs.used_by.length > 0}
					title={rs.used_by.length ? 'Detach it everywhere first' : undefined}
					onclick={() => (confirming = true)}>Delete rule set</Button
				>{/if}
		</Panel>
	</div>
{/if}

<Sheet.Root open={!!editing} onOpenChange={(o) => !o && (editing = null)}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Rule in {rs?.slug}</Sheet.Title>
			<Sheet.Description
				>Takes effect within seconds on every service that uses this rule set.</Sheet.Description
			>
		</Sheet.Header>
		{#if editing}
			<div class="overflow-y-auto px-4 pb-4">
				<RuleItemEditor bind:rule={editing.rule} {sets} {providers} />
			</div>
		{/if}
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (editing = null)}>Cancel</Button>
			<Button onclick={saveRule}>Save rule</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

<Sheet.Root bind:open={attach}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header><Sheet.Title>Attach a rule set</Sheet.Title></Sheet.Header>
		<div class="px-4 pb-4">
			<Select.Root type="single" bind:value={attachId}>
				<Select.Trigger class="w-full"
					>{attachable.find((s) => s.id === attachId)?.slug ?? 'Choose a rule set'}</Select.Trigger
				>
				<Select.Content>
					{#each attachable as s (s.id)}<Select.Item value={s.id}>{s.slug}</Select.Item>{/each}
				</Select.Content>
			</Select.Root>
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (attach = false)}>Cancel</Button>
			<Button disabled={!attachId} onclick={attachSet}>Attach</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

{#if rs}<ConfirmDelete
		bind:open={confirming}
		title="Delete {rs.slug}?"
		body="Its rules are deleted with it; this cannot be undone. Hashi asks for your passkey."
		verb="Delete rule set"
		onconfirm={remove}
	/>{/if}
