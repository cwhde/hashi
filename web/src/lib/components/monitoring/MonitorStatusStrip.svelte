<script lang="ts">
	let {
		buckets,
		width = 120
	}: {
		buckets: Array<{ up: boolean }>;
		width?: number;
	} = $props();

	const cells = $derived.by(() => {
		const target = 60;
		if (buckets.length === 0) {
			return Array.from({ length: target }, () => ({ up: false, empty: true }));
		}
		if (buckets.length >= target) {
			return buckets.slice(-target).map((b) => ({ up: b.up, empty: false }));
		}
		const pad = target - buckets.length;
		return [
			...Array.from({ length: pad }, () => ({ up: false, empty: true })),
			...buckets.map((b) => ({ up: b.up, empty: false }))
		];
	});
</script>

<div
	class="flex h-3 overflow-hidden rounded-sm border border-border/60"
	style:width="{width}px"
	role="img"
	aria-label="Last 60 minutes uptime strip"
>
	{#each cells as cell, i (i)}
		<div
			class="min-w-0 flex-1 border-r border-background/20 last:border-r-0 {cell.empty
				? 'bg-muted/40'
				: cell.up
					? 'bg-emerald-500/80'
					: 'bg-red-500/80'}"
		></div>
	{/each}
</div>
