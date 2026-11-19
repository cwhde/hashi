<script lang="ts">
	// Thin trace, sparse grid, current value top-right (10a §5.6). With `labels`, a pointer over the chart shows that
	// moment's time and value in the corner instead of the latest (bug 119).
	let {
		title,
		values,
		unit = '',
		current,
		labels,
		format
	}: {
		title: string;
		values: number[];
		unit?: string;
		current?: string;
		labels?: string[];
		format?: (v: number) => string;
	} = $props();
	let at = $state<number | null>(null);
	function move(e: PointerEvent) {
		const r = (e.currentTarget as SVGElement).getBoundingClientRect();
		const i = Math.round(((e.clientX - r.left) / r.width) * (values.length - 1));
		at = Math.max(0, Math.min(values.length - 1, i));
	}
	const show = (v: number) => (format ? format(v) : v + unit);
	const x = (i: number) => (i / Math.max(1, values.length - 1)) * w;
	const w = 600;
	const h = 140;
	const max = $derived(Math.max(1, ...values) * 1.15);
	const pts = $derived(
		values
			.map(
				(v, i) =>
					`${((i / Math.max(1, values.length - 1)) * w).toFixed(1)},${(h - (v / max) * h).toFixed(1)}`
			)
			.join(' ')
	);
</script>

<figure class="min-w-0 rounded-2xl border bg-card p-4">
	<figcaption class="mb-2 flex items-baseline justify-between gap-3">
		<span class="text-md font-medium">{title}</span>
		{#if at !== null && labels}<span class="tnum text-lg font-semibold"
				><span class="text-sm font-normal text-muted-foreground">{labels[at]}</span>
				{show(values[at] ?? 0)}</span
			>{:else}<span class="tnum text-lg font-semibold">{current ?? values.at(-1) + unit}</span>{/if}
	</figcaption>
	<svg
		viewBox="0 0 {w} {h}"
		class="h-36 w-full text-primary"
		preserveAspectRatio="none"
		role="img"
		aria-label="{title}, latest {current ?? values.at(-1) + unit}"
		onpointermove={labels ? move : undefined}
		onpointerleave={() => (at = null)}
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
		{#if at !== null && labels}<line
				x1={x(at)}
				x2={x(at)}
				y1="0"
				y2={h}
				class="stroke-muted-foreground"
				stroke-width="1"
				vector-effect="non-scaling-stroke"
			/>{/if}
	</svg>
</figure>
