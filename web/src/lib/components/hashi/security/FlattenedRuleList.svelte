<script lang="ts">
	import Mono from '../Mono.svelte';
	import SetBadge from './SetBadge.svelte';
	import ActionTag from './ActionTag.svelte';
	import type { FlatRow } from '$lib/design/security';
	import { Lock, ArrowRight } from '@lucide/svelte';

	// The whole path of a request as one list: Stage A (locked engine steps) then the flattened Stage B items, first
	// match wins (06 §4.3). `decides` marks the row that decided a traced request; rows after it are dimmed.
	let { rows, decides = -1 }: { rows: FlatRow[]; decides?: number } = $props();
	const numbered = $derived.by(() => {
		let n = 0;
		return rows.map((r) => ({ ...r, n: r.stage === 'B' ? ++n : 0 }));
	});
</script>

<ol class="divide-y text-[13px]">
	{#each numbered as r, i (i)}
		<li
			class="grid grid-cols-[2rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-0.5 px-4 py-2.5 md:grid-cols-[2rem_minmax(0,1fr)_minmax(0,0.8fr)_13rem_minmax(0,1fr)]
				{r.stage === 'A' ? 'bg-surface-2/40' : ''}
				{i === decides ? 'outline-2 -outline-offset-2 outline-primary/60' : ''}
				{decides >= 0 && i > decides ? 'opacity-45' : ''}"
		>
			<span class="tnum text-muted-foreground"
				>{#if i === decides}<ArrowRight
						class="size-4 text-primary"
						aria-label="Decides"
					/>{:else if r.locked}<Lock class="size-3.5" aria-label="Locked" />{:else}{r.n}{/if}</span
			>
			{#if r.stage === 'A'}
				<span class="font-medium">{r.label}</span>
				<span class="col-span-2 text-[12.5px] text-muted-foreground md:col-span-3">{r.note}</span>
			{:else}
				<span class="flex min-w-0 items-center gap-2">
					<SetBadge slug={r.appliesTo ?? 'all'} />
				</span>
				<span class="hidden md:inline"
					>{#if r.condition}<span class="text-muted-foreground">{r.condition.type}</span>
						<Mono>{r.condition.pattern}</Mono>{:else}<span class="text-muted-foreground"
							>any path</span
						>{/if}</span
				>
				<span class="flex items-center gap-1.5"
					><ActionTag action={r.action ?? 'none'} groups={r.auth?.groups} /></span
				>
				<span class="col-span-2 truncate text-[12.5px] text-muted-foreground md:col-span-1"
					>{r.origin}{#if r.note}<span class="hidden lg:inline"> · {r.note}</span>{/if}</span
				>
			{/if}
		</li>
	{/each}
</ol>
