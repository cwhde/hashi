<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import RuleEntryList from '$lib/components/hashi/security/RuleEntryList.svelte';
	import RuleItemEditor from '$lib/components/hashi/security/RuleItemEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Sheet from '$lib/components/ui/sheet';
	import { ChevronLeft, Lock, Plus, Layers, TriangleAlert } from '@lucide/svelte';

	let { data } = $props();
	const rs = $derived(data.rs);
	let editOpen = $state(false);
</script>

<a
	href="/design/rule-sets"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Rule sets</a
>

<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
	<div class="min-w-64 flex-1">
		<h1 class="flex items-center gap-2 text-[22px] leading-tight font-semibold">
			<span class="font-mono tracking-[-0.02em]">{rs.slug}</span>
			{#if rs.kind === 'global'}<Tag><Lock class="size-3" />global · cannot be deleted</Tag>{/if}
		</h1>
		<p class="mt-1 text-[13.5px] text-muted-foreground">{rs.description}</p>
	</div>
	<div class="flex gap-2">
		<Button variant="outline" size="sm"><Layers />Attach rule set</Button>
		<Button size="sm" onclick={() => (editOpen = true)}><Plus />Add rule</Button>
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
		<RuleEntryList entries={rs.entries} onedit={() => (editOpen = true)} />
	</Panel>
	<Panel title="Used by">
		<ul class="space-y-2 text-[13px]">
			{#each rs.usedBy as u (u.label)}<li>
					<a href={u.href} class="font-medium hover:underline">{u.label}</a>
				</li>{/each}
		</ul>
		<p class="mt-3 text-[12.5px] text-muted-foreground">
			{rs.kind === 'global'
				? 'Runs after each service’s own rules and before its fallback.'
				: 'A rule set that is attached somewhere cannot be deleted.'}
		</p>
		{#if rs.kind === 'user'}<Button
				variant="ghost"
				size="sm"
				class="mt-2 -ml-2 text-bad-text"
				disabled={rs.usedBy.length > 0}>Delete rule set</Button
			>{/if}
	</Panel>
</div>

<Sheet.Root bind:open={editOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Rule in {rs.slug}</Sheet.Title>
			<Sheet.Description
				>Takes effect within seconds on every service that uses this rule set.</Sheet.Description
			>
		</Sheet.Header>
		<div class="overflow-y-auto px-4 pb-4"><RuleItemEditor /></div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (editOpen = false)}>Cancel</Button>
			<Button>Save rule</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
