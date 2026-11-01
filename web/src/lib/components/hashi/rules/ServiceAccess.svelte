<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '../Panel.svelte';
	import RuleEntryList from './RuleEntryList.svelte';
	import RuleItemEditor from './RuleItemEditor.svelte';
	import FlattenedRuleList from './FlattenedRuleList.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Select from '$lib/components/ui/select';
	import { CircleAlert, Layers, Plus, TriangleAlert, Unplug } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
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

	// A service's Access tab (06 §13): its own rules, attached sets, the fallback, the flattened list and "try a request".
	let { serviceId }: { serviceId: string } = $props();
	let access = $state<Schemas['ServiceAccess'] | null>(null);
	let entries = $state<RuleEntry[]>([]);
	let fallback = $state<'none' | 'deny'>('none');
	let sets = $state<IPSet[]>([]);
	let providers = $state<Provider[]>([]);
	let shared = $state<RuleSet[]>([]);
	let error = $state('');
	let notice = $state('');
	let editing = $state<{ rule: RuleEntry; index: number } | null>(null);
	let attach = $state(false);
	let attachId = $state('');
	let tryIp = $state('');
	let tryPath = $state('/');
	let trace = $state<Schemas['TraceResult'] | null>(null);
	let traceError = $state('');

	function adopt(a: Schemas['ServiceAccess']) {
		access = a;
		entries = a.rule_set.entries.filter((e) => !e.fallback);
		fallback = a.fallback;
	}
	onMount(async () => {
		const r = await api.GET('/api/services/{id}/access', { params: { path: { id: serviceId } } });
		if (r.error) {
			error = problemText(r.error);
			return;
		}
		adopt(r.data);
		[sets, providers, shared] = await Promise.all([listSets(), listProviders(), listRuleSets()]);
	});
	async function save() {
		error = notice = '';
		const r = await api.PUT('/api/services/{id}/access', {
			params: { path: { id: serviceId } },
			body: { entries: entries.map(toInput), fallback }
		});
		if (r.error) {
			error = problemText(r.error);
			const again = await api.GET('/api/services/{id}/access', {
				params: { path: { id: serviceId } }
			});
			if (again.data) adopt(again.data);
			return;
		}
		adopt(r.data.access);
		trace = null;
		if (r.data.plans.length) {
			notice = 'What this service does while Hashi is away changed: Traefik got a new plan.';
			await pendingPlans.refresh();
		}
	}
	function saveRule() {
		if (!editing) return;
		if (editing.index < 0) entries = [...entries, editing.rule];
		else entries[editing.index] = editing.rule;
		editing = null;
		void save();
	}
	function attachSet() {
		const rs = shared.find((s) => s.id === attachId);
		if (!rs) return;
		entries = [...entries, { ...emptyRule(''), type: 'set', child_id: rs.id, child_slug: rs.slug }];
		attach = false;
		void save();
	}
	async function runTrace() {
		traceError = '';
		const r = await api.POST('/api/addresses/{address}/trace', {
			params: { path: { address: tryIp.trim() } },
			body: { service_id: serviceId, path: tryPath || '/' }
		});
		if (r.error) {
			trace = null;
			traceError = problemText(r.error);
			return;
		}
		trace = r.data;
	}
	const attachable = $derived(shared.filter((s) => s.kind === 'user'));
</script>

{#if error}
	<p class="mb-3 flex items-start gap-2 text-[13px] text-bad-text">
		<CircleAlert class="mt-0.5 size-3.5 shrink-0" />{error}
	</p>
{/if}
{#if notice}<p class="mb-3 text-[13px] text-muted-foreground">{notice}</p>{/if}
{#if access}
	<div class="space-y-4">
		{#each access.warnings as w (w)}
			<p
				class="flex items-start gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-[12.5px] text-warn-text"
			>
				<TriangleAlert class="mt-0.5 size-3.5 shrink-0" />{w}
			</p>
		{/each}
		<Panel
			flush
			title="This service's rules"
			description={access.editable
				? 'Checked first. Then the global rules, then the fallback.'
				: 'The Hashi admin page has its own passkey login, so rules never apply to it.'}
		>
			{#snippet actions()}
				{#if access?.editable}
					<Button size="sm" variant="outline" onclick={() => (attach = true)}
						><Layers />Attach rule set</Button
					>
					<Button
						size="sm"
						onclick={() => (editing = { rule: emptyRule(allSetId(sets)), index: -1 })}
						><Plus />Add rule</Button
					>
				{/if}
			{/snippet}
			{#if entries.length === 0 && access.editable}
				<p class="border-b px-4 py-3 text-[13px] text-muted-foreground">
					No rules of its own. Everyone not stopped by the global rules gets the fallback.
				</p>
			{/if}
			<RuleEntryList
				bind:entries
				bind:fallback
				{sets}
				readonly={!access.editable}
				onedit={(i) => (editing = { rule: $state.snapshot(entries[i]) as RuleEntry, index: i })}
				onchange={save}
			/>
		</Panel>

		<Panel
			flush
			title="What every request goes through"
			description="All rules in order; the first match decides. Try an address and a path."
		>
			<form
				class="flex flex-wrap items-center gap-2 border-b px-4 py-2.5"
				onsubmit={(e) => {
					e.preventDefault();
					void runTrace();
				}}
			>
				<Input
					bind:value={tryIp}
					class="w-44 font-mono"
					placeholder="203.0.113.7"
					aria-label="Address to try"
				/>
				<Input bind:value={tryPath} class="w-36 font-mono" aria-label="Path to try" />
				<Button size="sm" variant="outline" type="submit" disabled={!tryIp.trim()}
					>Try a request</Button
				>
				{#if traceError}<span class="text-[12.5px] text-bad-text">{traceError}</span>{/if}
			</form>
			<FlattenedRuleList
				rows={trace?.flattened ?? access.flattened}
				{sets}
				decides={trace?.decided ?? -1}
			/>
		</Panel>

		<p class="flex items-start gap-2 text-[12.5px] text-muted-foreground">
			<Unplug class="mt-0.5 size-3.5 shrink-0" />
			{#if access.fail_closed}If Hashi is unreachable this service refuses requests, because its
				rules can deny or ask for sign-in.{:else}If Hashi is unreachable this service stays open.{/if}
		</p>
	</div>
{/if}

<Sheet.Root open={!!editing} onOpenChange={(o) => !o && (editing = null)}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Rule</Sheet.Title>
			<Sheet.Description>Takes effect within seconds.</Sheet.Description>
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
		<Sheet.Header>
			<Sheet.Title>Attach a rule set</Sheet.Title>
			<Sheet.Description>It is checked completely at its position in the list.</Sheet.Description>
		</Sheet.Header>
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
