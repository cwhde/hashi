<script lang="ts">
	import Mono from '../Mono.svelte';
	import Tag from '../Tag.svelte';
	import SetBadge from './SetBadge.svelte';
	import ActionTag from './ActionTag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { ruleSetById, type RuleEntry } from '$lib/design/security';
	import { GripVertical, Layers, Lock, MoreHorizontal } from '@lucide/svelte';

	// The editable entries of one rule set, in priority order: rule items and attached rule sets (06 §4.2). A
	// service root also shows its locked fallback as the last row, where only the action can change.
	let {
		entries,
		fallback = $bindable<'none' | 'deny' | undefined>(undefined),
		onedit
	}: { entries: RuleEntry[]; fallback?: 'none' | 'deny'; onedit?: () => void } = $props();
	const sorted = $derived([...entries].sort((a, b) => a.priority - b.priority));
</script>

<ol class="divide-y text-[13px]">
	{#each sorted as e (e.priority)}
		<li
			class="grid grid-cols-[1rem_1.5rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-4 py-2.5 md:grid-cols-[1rem_1.5rem_minmax(0,1fr)_minmax(0,0.8fr)_11rem_auto]"
		>
			<GripVertical class="size-4 cursor-grab text-muted-foreground" aria-label="Drag to reorder" />
			<span class="tnum text-muted-foreground">{e.priority}</span>
			{#if e.kind === 'ref'}
				{@const rs = ruleSetById(e.ruleSet)}
				<span class="flex min-w-0 items-center gap-2"
					><Layers class="size-4 shrink-0 text-muted-foreground" /><a
						href="/design/rule-sets/{e.ruleSet}"
						class="truncate font-medium hover:underline">{rs?.slug}</a
					></span
				>
				<span class="hidden truncate text-muted-foreground md:inline">{rs?.description}</span>
				<span class="hidden md:inline"
					><Tag>{rs?.entries.length} {rs?.entries.length === 1 ? 'rule' : 'rules'} inside</Tag
					></span
				>
			{:else}
				<span class="min-w-0"
					><SetBadge slug={e.appliesTo} />{#if e.note}<span
							class="block truncate text-[12px] text-muted-foreground">{e.note}</span
						>{/if}</span
				>
				<span class="hidden md:inline"
					>{#if e.condition}<span class="text-muted-foreground">{e.condition.type}</span>
						<Mono>{e.condition.pattern}</Mono>{:else}<span class="text-muted-foreground"
							>any path</span
						>{/if}</span
				>
				<span class="hidden md:inline"><ActionTag action={e.action} groups={e.auth?.groups} /></span
				>
			{/if}
			<span class="flex items-center gap-1">
				{#if e.kind === 'item'}<Switch checked={e.enabled} aria-label="Rule enabled" />{/if}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}<Button
								{...props}
								variant="ghost"
								size="icon-sm"
								aria-label="Actions for entry {e.priority}"><MoreHorizontal /></Button
							>{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end">
						{#if e.kind === 'item'}<DropdownMenu.Item onclick={onedit}>Edit</DropdownMenu.Item>{/if}
						<DropdownMenu.Item>Move up</DropdownMenu.Item>
						<DropdownMenu.Item>Move down</DropdownMenu.Item>
						<DropdownMenu.Separator />
						<DropdownMenu.Item class="text-bad-text"
							>{e.kind === 'ref' ? 'Detach' : 'Remove'}</DropdownMenu.Item
						>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			</span>
		</li>
	{/each}
	{#if fallback}
		<li
			class="grid grid-cols-[1rem_1.5rem_minmax(0,1fr)_auto] items-center gap-x-3 bg-surface-2/40 px-4 py-2.5 md:grid-cols-[1rem_1.5rem_minmax(0,1fr)_minmax(0,0.8fr)_11rem_auto]"
		>
			<span></span>
			<Lock class="size-3.5 text-muted-foreground" aria-label="Locked" />
			<span class="min-w-0"
				><span class="font-medium">Fallback</span><span
					class="block text-[12px] text-muted-foreground"
					>Everyone else, any path · after the global rules</span
				></span
			>
			<span class="hidden md:inline"></span>
			<span class="col-span-1 flex gap-1 md:col-span-2" role="radiogroup" aria-label="Fallback">
				<Button
					size="xs"
					variant={fallback === 'none' ? 'secondary' : 'ghost'}
					role="radio"
					aria-checked={fallback === 'none'}
					onclick={() => (fallback = 'none')}>Open (None)</Button
				>
				<Button
					size="xs"
					variant={fallback === 'deny' ? 'secondary' : 'ghost'}
					role="radio"
					aria-checked={fallback === 'deny'}
					onclick={() => (fallback = 'deny')}>Closed (Deny)</Button
				>
			</span>
		</li>
	{/if}
</ol>
