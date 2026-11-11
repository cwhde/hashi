<script lang="ts">
	import { Check } from '@lucide/svelte';

	// Pick user middlewares (04 §1.1a) by name; they run in the order picked. Names no longer in the file stay visible
	// so they can be removed (the service is not published while it names one).
	let {
		names,
		selected = $bindable([]),
		label,
		disabled = false
	}: { names: string[]; selected?: string[]; label: string; disabled?: boolean } = $props();

	const shown = $derived([...names, ...selected.filter((n) => !names.includes(n))]);
	function toggle(n: string) {
		selected = selected.includes(n) ? selected.filter((x) => x !== n) : [...selected, n];
	}
</script>

<ul class="flex flex-wrap gap-1.5" aria-label={label}>
	{#each shown as n (n)}
		{@const on = selected.includes(n)}
		{@const gone = !names.includes(n)}
		<li>
			<button
				type="button"
				{disabled}
				aria-pressed={on}
				onclick={() => toggle(n)}
				class="inline-flex h-7 items-center gap-1.5 rounded-lg border px-2.5 font-mono text-[12px] transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-60 {on
					? gone
						? 'border-bad/40 bg-bad/10 text-bad-text'
						: 'border-primary/50 bg-primary/12 text-foreground'
					: 'text-muted-foreground hover:bg-surface-2'}"
			>
				{#if on}<Check class="size-3.5" />{/if}{n}{#if on && selected.length > 1}<span
						class="font-sans text-[11px] text-muted-foreground">{selected.indexOf(n) + 1}</span
					>{/if}{#if gone}<span class="font-sans text-[11px]">(not in the file)</span>{/if}
			</button>
		</li>
	{/each}
</ul>
