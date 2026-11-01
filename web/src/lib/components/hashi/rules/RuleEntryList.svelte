<script lang="ts">
	import Mono from '../Mono.svelte';
	import ActionTag from './ActionTag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { ArrowDown, ArrowUp, Layers, Lock, MoreHorizontal } from '@lucide/svelte';
	import type { IPSet } from '$lib/ipsets';
	import type { RuleEntry } from '$lib/rules';

	// The editable entries of one rule set in order: rule items and attached rule sets (06 §4.2). A service root also
	// shows its locked fallback last, where only the action can change. Saving is the page's job.
	let {
		entries = $bindable(),
		fallback = $bindable(undefined),
		sets,
		readonly = false,
		onedit,
		onchange
	}: {
		entries: RuleEntry[];
		fallback?: 'none' | 'deny';
		sets: IPSet[];
		readonly?: boolean;
		onedit?: (index: number) => void;
		onchange?: () => void;
	} = $props();
	const slug = (id: string) => sets.find((s) => s.id === id)?.slug ?? id;
	function move(i: number, by: number) {
		const next = [...entries];
		next.splice(i + by, 0, ...next.splice(i, 1));
		entries = next;
		onchange?.();
	}
	function remove(i: number) {
		entries = entries.filter((_, n) => n !== i);
		onchange?.();
	}
	const row =
		'grid grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-4 py-2.5 md:grid-cols-[1.5rem_minmax(0,1fr)_minmax(0,0.8fr)_11rem_auto]';
</script>

<ol class="divide-y text-[13px]">
	{#each entries as e, i (i + e.id)}
		<li class={row}>
			<span class="tnum text-muted-foreground">{i + 1}</span>
			{#if e.type === 'set'}
				<span class="flex min-w-0 items-center gap-2"
					><Layers class="size-4 shrink-0 text-muted-foreground" /><a
						href="/rule-sets/{e.child_id}"
						class="truncate font-medium hover:underline">{e.child_slug}</a
					></span
				>
				<span class="hidden text-muted-foreground md:inline">Attached rule set</span>
				<span class="hidden md:inline"></span>
			{:else}
				<span class="min-w-0"
					><a href="/ip-sets/{e.applies_to}" class="truncate font-medium hover:underline"
						>{slug(e.applies_to)}</a
					>{#if e.note}<span class="block truncate text-[12px] text-muted-foreground">{e.note}</span
						>{/if}</span
				>
				<span class="hidden md:inline"
					>{#if e.match_type}<span class="text-muted-foreground">{e.match_type}</span>
						<Mono>{e.pattern}</Mono>{:else}<span class="text-muted-foreground">any path</span
						>{/if}</span
				>
				<span class="hidden md:inline"><ActionTag action={e.action} groups={e.groups} /></span>
			{/if}
			<span class="flex items-center gap-1">
				{#if e.type === 'item'}<Switch
						checked={e.enabled}
						disabled={readonly}
						aria-label="Rule enabled"
						onCheckedChange={(v) => {
							entries = entries.map((x, n) => (n === i ? { ...x, enabled: v } : x));
							onchange?.();
						}}
					/>{/if}
				{#if !readonly}
					<DropdownMenu.Root>
						<DropdownMenu.Trigger>
							{#snippet child({ props })}<Button
									{...props}
									variant="ghost"
									size="icon-sm"
									aria-label="Actions for entry {i + 1}"><MoreHorizontal /></Button
								>{/snippet}
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="end">
							{#if e.type === 'item'}<DropdownMenu.Item onclick={() => onedit?.(i)}
									>Edit</DropdownMenu.Item
								>{/if}
							<DropdownMenu.Item disabled={i === 0} onclick={() => move(i, -1)}
								><ArrowUp />Move up</DropdownMenu.Item
							>
							<DropdownMenu.Item disabled={i === entries.length - 1} onclick={() => move(i, 1)}
								><ArrowDown />Move down</DropdownMenu.Item
							>
							<DropdownMenu.Separator />
							<DropdownMenu.Item class="text-bad-text" onclick={() => remove(i)}
								>{e.type === 'set' ? 'Detach' : 'Remove'}</DropdownMenu.Item
							>
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				{/if}
			</span>
		</li>
	{/each}
	{#if fallback}
		<li class="{row} bg-surface-2/40">
			<Lock class="size-3.5 text-muted-foreground" aria-label="Locked" />
			<span class="min-w-0"
				><span class="font-medium">Fallback</span><span
					class="block text-[12px] text-muted-foreground"
					>Everyone else, any path, after the global rules</span
				></span
			>
			<span class="hidden md:inline"></span>
			<span class="col-span-1 flex gap-1 md:col-span-2" role="radiogroup" aria-label="Fallback">
				{#each [['none', 'Open (None)'], ['deny', 'Closed (Deny)']] as const as [v, label] (v)}
					<Button
						size="xs"
						disabled={readonly}
						variant={fallback === v ? 'secondary' : 'ghost'}
						role="radio"
						aria-checked={fallback === v}
						onclick={() => {
							fallback = v;
							onchange?.();
						}}>{label}</Button
					>
				{/each}
			</span>
		</li>
	{/if}
</ol>
