<script lang="ts">
	// Mock chart geometry for S0b: thin trace, sparse grid, current value top-right (10a §5.6). The real pages use uPlot.
	let {
		title,
		values,
		unit = '',
		current
	}: { title: string; values: number[]; unit?: string; current?: string } = $props();
	const w = 600;
	const h = 140;
	const max = $derived(Math.max(...values) * 1.15);
	const pts = $derived(
		values
			.map(
				(v, i) => `${((i / (values.length - 1)) * w).toFixed(1)},${(h - (v / max) * h).toFixed(1)}`
			)
			.join(' ')
	);
</script>

<figure class="min-w-0 rounded-2xl border bg-card p-4">
	<figcaption class="mb-2 flex items-baseline justify-between gap-3">
		<span class="text-[13px] font-medium">{title}</span>
		<span class="tnum text-[15px] font-semibold">{current ?? values.at(-1) + unit}</span>
	</figcaption>
	<svg
		viewBox="0 0 {w} {h}"
		class="h-36 w-full text-primary"
		preserveAspectRatio="none"
		role="img"
		aria-label="{title}, latest {current ?? values.at(-1) + unit}"
	>
		{#each [0.25, 0.5, 0.75] as g (g)}<line
				x1="0"
				x2={w}
				y1={h * g}
				y2={h * g}
				class="stroke-border"
				stroke-width="1"
				vector-effect="non-scaling-stroke"
				stroke-dasharray="2 4"
			/>{/each}
		<polyline
			points={pts}
			fill="none"
			stroke="currentColor"
			stroke-width="1.5"
			vector-effect="non-scaling-stroke"
			stroke-linejoin="round"
		/>
	</svg>
</figure>
