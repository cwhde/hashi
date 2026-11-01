<script lang="ts">
	import Mono from '../Mono.svelte';
	import ActionTag from './ActionTag.svelte';
	import { ArrowRight, Lock } from '@lucide/svelte';
	import type { IPSet } from '$lib/ipsets';
	import type { FlatRule } from '$lib/rules';

	// All rules a request goes through, as one list; first match wins (06 §4.3). `decides` marks the row that decided a
	// traced request; rows after it are dimmed.
	let {
		rows,
		sets,
		decides = -1
	}: { rows: FlatRule[]; sets: IPSet[]; decides?: number } = $props();
	const slug = (id: string) => sets.find((s) => s.id === id)?.slug ?? id;
</script>

<ol class="divide-y text-[13px]">
	{#each rows as r, i (i)}
		<li
			class="grid grid-cols-[2rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-0.5 px-4 py-2.5 md:grid-cols-[2rem_minmax(0,1fr)_minmax(0,0.8fr)_13rem_minmax(0,1fr)]
				{i === decides ? 'outline-2 -outline-offset-2 outline-primary/60' : ''}
				{decides >= 0 && i > decides ? 'opacity-45' : ''}"
		>
			<span class="tnum text-muted-foreground"
				>{#if i === decides}<ArrowRight
						class="size-4 text-primary"
						aria-label="Decides"
					/>{:else if r.entry.fallback}<Lock class="size-3.5" aria-label="Locked" />{:else}{i +
						1}{/if}</span
			>
			<span class="truncate font-medium"
				>{r.entry.fallback ? 'Fallback: everyone' : slug(r.entry.applies_to)}</span
			>
			<span class="hidden md:inline"
				>{#if r.entry.match_type}<span class="text-muted-foreground">{r.entry.match_type}</span>
					<Mono>{r.entry.pattern}</Mono>{:else}<span class="text-muted-foreground">any path</span
					>{/if}</span
			>
			<span class="flex items-center gap-1.5"
				><ActionTag action={r.entry.action} groups={r.entry.groups} /></span
			>
			<span class="col-span-2 truncate text-[12.5px] text-muted-foreground md:col-span-1"
				>{r.path.join(' → ')}</span
			>
		</li>
	{/each}
</ol>
