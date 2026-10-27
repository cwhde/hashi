<script lang="ts">
	import type { PlanItem, Risk } from '$lib/design/mock';
	import { Button } from '$lib/components/ui/button';
	import { cn } from '$lib/utils';
	import Mono from './Mono.svelte';
	import Tag from './Tag.svelte';
	import { ShieldAlert, Fingerprint } from '@lucide/svelte';

	// The exact preview of a sync plan (01-platform.md §5, principle 6). What is shown here is byte-for-byte what is
	// applied. High-risk items are marked per row; approving a high-risk plan needs a fresh passkey.
	let {
		title,
		trigger,
		risk,
		items,
		compact = false
	}: {
		title: string;
		trigger: string;
		risk: Risk;
		items: PlanItem[];
		compact?: boolean;
	} = $props();

	const verbs: Record<PlanItem['action'], string> = {
		create: 'Create',
		update: 'Change',
		delete: 'Delete',
		conflict: 'Conflict',
		'write file': 'Write',
		restart: 'Restart'
	};
	const highCount = $derived(items.filter((i) => i.risk === 'high').length);
</script>

<article class="overflow-hidden rounded-2xl border bg-card">
	<header class="flex flex-wrap items-center gap-x-4 gap-y-2 border-b px-4 py-3">
		<div class="min-w-0 flex-1">
			<h3 class="text-[14.5px] font-semibold">{title}</h3>
			<p class="text-[12.5px] text-muted-foreground">{trigger} · {items.length} changes</p>
		</div>
		{#if risk === 'high'}
			<Tag tone="warn"><ShieldAlert class="size-3" />{highCount} high-risk</Tag>
		{:else}
			<Tag tone="ok">Low risk · applies automatically</Tag>
		{/if}
	</header>

	<ol class="divide-y">
		{#each items as item, i (i)}
			<li
				class={cn(
					'grid gap-x-4 gap-y-1 px-4 py-2.5 sm:grid-cols-[6.5rem_9rem_1fr]',
					item.risk === 'high' && 'bg-warn/[0.04]'
				)}
			>
				<div class="flex items-center gap-2">
					<span
						class={cn(
							'text-[13px] font-medium',
							item.action === 'delete' && 'text-bad-text',
							item.action === 'create' && 'text-ok-text',
							item.action === 'conflict' && 'text-warn-text'
						)}>{verbs[item.action]}</span
					>
					{#if item.risk === 'high'}<ShieldAlert
							class="size-3.5 text-warn-text"
							aria-label="High risk"
						/>{/if}
				</div>
				<div class="min-w-0 text-[12.5px] text-muted-foreground">
					<span class="sm:block">{item.subsystem}</span><span class="sm:hidden"> · </span>
					<span class="truncate sm:block">{item.target}</span>
				</div>
				<div class="min-w-0 space-y-0.5">
					<div class="text-[13px] font-medium">{item.object}</div>
					{#if item.before && !compact}
						<div class="flex gap-2 text-[12.5px]">
							<span class="w-10 shrink-0 text-muted-foreground">before</span>
							<Mono class="break-all text-muted-foreground line-through decoration-1"
								>{item.before}</Mono
							>
						</div>
					{/if}
					{#if item.after && !compact}
						<div class="flex gap-2 text-[12.5px]">
							<span class="w-10 shrink-0 text-muted-foreground">after</span>
							<Mono class="break-all">{item.after}</Mono>
						</div>
					{/if}
					{#if item.note}<p class="text-[12.5px] text-muted-foreground">{item.note}</p>{/if}
				</div>
			</li>
		{/each}
	</ol>

	{#if compact}
		<footer class="flex items-center justify-end border-t px-4 py-2.5">
			<Button variant="outline" size="sm" href="/design/changes">Review changes</Button>
		</footer>
	{:else}
		<footer
			class="flex flex-wrap items-center justify-end gap-2 border-t bg-surface-2/50 px-4 py-3"
		>
			<p class="mr-auto text-[12.5px] text-muted-foreground">
				{#if risk === 'high'}Approving asks for your passkey. Nothing changes until you approve.{:else}Will
					apply on the next sync.{/if}
			</p>
			<Button variant="ghost" size="sm">Discard</Button>
			<Button size="sm">
				{#if risk === 'high'}<Fingerprint />Approve and apply{:else}Apply now{/if}
			</Button>
		</footer>
	{/if}
</article>
