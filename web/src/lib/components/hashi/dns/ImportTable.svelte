<script lang="ts">
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import { Search } from '@lucide/svelte';
	import {
		leaveOnlyReason,
		offeredDecisions,
		rrsetKey,
		ttlText,
		type Decision,
		type ImportRow
	} from '$lib/dns';

	// 03-dns.md §4: one row per RRSet; leave (default) / import / remove. Rows without a stored decision are new since
	// the last import. `decisions` holds the choice per rrsetKey and is what the page saves.
	let {
		rows,
		decisions = $bindable()
	}: { rows: ImportRow[]; decisions: Record<string, Decision> } = $props();

	let q = $state('');
	// A re-import starts with only the new rows (03-dns.md §4); a first import shows everything anyway.
	// svelte-ignore state_referenced_locally
	let newOnly = $state(rows.some((r) => r.decision) && rows.some((r) => !r.decision));
	let expanded = $state<Record<string, boolean>>({});

	const labels: Record<Decision, string> = { leave: 'Leave', import: 'Import', prune: 'Remove' };
	const order: Decision[] = ['leave', 'import', 'prune'];

	const newCount = $derived(rows.filter((r) => !r.decision).length);
	const shown = $derived(
		rows.filter((r) => {
			if (newOnly && newCount > 0 && r.decision) return false;
			const needle = q.trim().toLowerCase();
			if (!needle) return true;
			const hay = `${r.rrset.name} ${r.rrset.type} ${r.rrset.values.join(' ')}`.toLowerCase();
			return hay.includes(needle);
		})
	);
	const counts = $derived(
		Object.values(decisions).reduce(
			(acc, d) => ((acc[d] = (acc[d] ?? 0) + 1), acc),
			{} as Partial<Record<Decision, number>>
		)
	);
</script>

<div class="rounded-2xl border bg-card">
	<div class="flex flex-wrap items-center gap-3 border-b px-3 py-2.5">
		<div class="relative w-full max-w-72">
			<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
			<Input bind:value={q} placeholder="Filter records" class="pl-8" aria-label="Filter records" />
		</div>
		<label class="flex items-center gap-2 text-[13px]"
			><Switch bind:checked={newOnly} disabled={newCount === 0} />New only
			<span class="tnum text-muted-foreground">({newCount})</span></label
		>
		<p class="tnum ml-auto text-[12.5px] text-muted-foreground" aria-live="polite">
			{rows.length} records · {counts.import ?? 0} to import · {counts.prune ?? 0} to remove
		</p>
	</div>
	<ul class="divide-y" aria-label="Import decisions">
		{#each shown as row (rrsetKey(row.rrset))}
			{@const r = row.rrset}
			{@const k = rrsetKey(r)}
			{@const offered = offeredDecisions(row)}
			{@const many = r.values.length > 1}
			<li
				class="grid items-center gap-x-4 gap-y-2 px-4 py-2.5 text-[13px] md:grid-cols-[minmax(0,1fr)_auto]"
			>
				<div class="min-w-0">
					<div class="flex flex-wrap items-center gap-2">
						<Mono class="font-medium">{r.name}</Mono>
						<span class="font-medium">{r.type}</span>
						<span class="tnum text-[12px] text-muted-foreground">TTL {ttlText(r)}</span>
						{#if !row.decision}<Tag tone="info">new</Tag>{/if}
						{#if r.origin !== 'unmanaged'}<Tag tone="accent">{r.origin}</Tag>{/if}
					</div>
					{#if expanded[k]}
						<ul class="mt-1 space-y-0.5 text-muted-foreground">
							{#each r.values as v, i (i)}<li><Mono class="break-all">{v}</Mono></li>{/each}
						</ul>
					{:else}
						<Mono class="block truncate text-muted-foreground">{r.values.join(', ')}</Mono>
					{/if}
					{#if many || r.values.join(', ').length > 80}
						<button
							type="button"
							class="mt-0.5 text-[12px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
							aria-expanded={!!expanded[k]}
							onclick={() => (expanded[k] = !expanded[k])}
							>{expanded[k]
								? 'Collapse'
								: `Show ${many ? `all ${r.values.length} values` : 'value'}`}</button
						>
					{/if}
				</div>
				{#if offered.length <= 1}
					<span class="text-[12.5px] text-muted-foreground md:max-w-72 md:text-right"
						>{leaveOnlyReason(r)}</span
					>
				{:else}
					<div
						class="inline-flex w-fit rounded-[10px] border bg-surface-2 p-0.5"
						role="radiogroup"
						aria-label="Decision for {r.name} {r.type}"
					>
						{#each order.filter((d) => offered.includes(d)) as d (d)}
							{@const on = (decisions[k] ?? 'leave') === d}
							<button
								type="button"
								role="radio"
								aria-checked={on}
								onclick={() => (decisions[k] = d)}
								class="h-7 rounded-lg px-2.5 text-[12.5px] {on
									? d === 'prune'
										? 'bg-bad/15 text-bad-text'
										: 'bg-card text-foreground shadow-sm'
									: 'text-muted-foreground hover:text-foreground'}">{labels[d]}</button
							>
						{/each}
					</div>
				{/if}
			</li>
		{:else}
			<li class="px-4 py-6 text-center text-[13px] text-muted-foreground">
				{rows.length === 0 ? 'The zone has no records yet.' : 'No record matches the filter.'}
			</li>
		{/each}
	</ul>
</div>
