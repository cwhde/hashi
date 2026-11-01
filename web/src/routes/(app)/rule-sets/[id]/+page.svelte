<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import RuleEntryList from '$lib/components/hashi/rules/RuleEntryList.svelte';
	import RuleItemEditor from '$lib/components/hashi/rules/RuleItemEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Select from '$lib/components/ui/select';
	import { ChevronLeft, CircleAlert, Layers, Lock, Plus, TriangleAlert } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { pendingPlans } from '$lib/sync-pending.svelte';
	import { listSets, type IPSet } from '$lib/ipsets';
	import {
		allSetId,
		emptyRule,
		listProviders,
		listRuleSets,
		toInput,
		type Provider,
		type RuleEntry,
		type RuleSet
	} from '$lib/rules';

	const id = $derived(page.params.id ?? '');
	let rs = $state<RuleSet | null>(null);
	let entries = $state<RuleEntry[]>([]);
	let sets = $state<IPSet[]>([]);
	let providers = $state<Provider[]>([]);
	let others = $state<RuleSet[]>([]);
	let error = $state('');
	let editing = $state<{ rule: RuleEntry; index: number } | null>(null);
	let attach = $state(false);
	let attachId = $state('');

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
		const r = await api.DELETE('/api/rule-sets/{id}', { params: { path: { id } } });
		if (r.error) {
			error = problemText(r.error);
			return;
		}
		await goto('/rule-sets');
	}
	const attachable = $derived(others.filter((s) => s.kind === 'user' && s.id !== id));
</script>

<a
	href="/rule-sets"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Rule sets</a
>
{#if error}
	<p class="mb-3 flex items-start gap-2 text-[13px] text-bad-text">
		<CircleAlert class="mt-0.5 size-3.5 shrink-0" />{error}
	</p>
{/if}
{#if !rs}
	{#if !error}<Skeleton class="h-40 w-full rounded-2xl" />{/if}
{:else}
	<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
		<div class="min-w-64 flex-1">
			<h1 class="flex items-center gap-2 text-[22px] leading-tight font-semibold">
				<span class="font-mono tracking-[-0.02em]">{rs.slug}</span>
				{#if rs.kind === 'global'}<Tag><Lock class="size-3" />global, cannot be deleted</Tag>{/if}
			</h1>
			<p class="mt-1 text-[13.5px] text-muted-foreground">{rs.description}</p>
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
			class="mb-4 flex items-start gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-[12.5px] text-warn-text"
		>
			<TriangleAlert class="mt-0.5 size-3.5 shrink-0" />{w}
		</p>
	{/each}

	<div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
		<Panel
			flush
			title="Rules"
			description="Checked top to bottom. An attached rule set is checked completely at its position."
		>
			{#if entries.length === 0}<p class="px-4 py-3 text-[13px] text-muted-foreground">
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
				<ul class="space-y-2 text-[13px]">
					{#each rs.used_by as u (u.href)}<li>
							<a href={u.href} class="font-medium hover:underline">{u.label}</a>
						</li>{/each}
				</ul>
			{:else if rs.kind !== 'global'}<p class="text-[13px] text-muted-foreground">
					Not used yet.
				</p>{/if}
			<p class="mt-3 text-[12.5px] text-muted-foreground">
				{rs.kind === 'global'
					? "Runs after each service's own rules and before its fallback."
					: 'A rule set that is attached somewhere cannot be deleted.'}
			</p>
			{#if rs.kind === 'user'}<Button
					variant="ghost"
					size="sm"
					class="mt-2 -ml-2 text-bad-text"
					disabled={rs.used_by.length > 0}
					onclick={remove}>Delete rule set</Button
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
