<script lang="ts">
	// Thin trace, sparse grid, current value top-right (10a §5.6). With `labels`, a pointer over the chart shows that
	// moment's time and value in a bubble beside it, as the overview's charts do (bugs 119, 155); the corner keeps the
	// latest value. Values may be negative (a public path faster than the internal one, bug 156). With `sync` and
	// `times`, charts of one page share the pointer: the same moment shows in each of them.
	import { nearest, type ChartSync } from '#lib/chart-sync.svelte.ts';

	let {
		title,
		values,
		unit = '',
		current,
		labels,
		format,
		times,
		sync
	}: {
		title: string;
		values: number[];
		unit?: string;
		current?: string;
		labels?: string[];
		format?: (v: number) => string;
		/** Each point's moment (ms); with `sync`, the pointer is shared with the page's other charts. */
		times?: number[];
		sync?: ChartSync;
	} = $props();
	let own = $state<number | null>(null);
	const shared = $derived(!!sync && !!times && times.length === values.length);
	const at = $derived(shared ? (sync!.at === null ? null : nearest(times!, sync!.at)) : own);
	function move(e: PointerEvent) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const i = Math.round(((e.clientX - r.left) / r.width) * (values.length - 1));
		const k = Math.max(0, Math.min(values.length - 1, i));
		if (shared) sync!.at = times![k]!;
		else own = k;
	}
	function leave() {
		if (shared) sync!.at = null;
		else own = null;
	}
	const show = (v: number) => (format ? format(v) : v + unit);
	const w = 600;
	const h = 140;
	const top = $derived(Math.max(1, ...values) * 1.15);
	const bottom = $derived(Math.min(0, ...values) * 1.15);
	const x = (i: number) => (i / Math.max(1, values.length - 1)) * w;
	const y = (v: number) => h - ((v - bottom) / (top - bottom || 1)) * h;
	const pts = $derived(values.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' '));
	// Where the bubble sits, as fractions of the chart: beside the point, kept inside the chart.
	const share = $derived(at === null ? 0 : at / Math.max(1, values.length - 1));
</script>

<figure class="min-w-0 rounded-2xl border bg-card p-4">
	<figcaption class="mb-2 flex items-baseline justify-between gap-3">
		<span class="text-md font-medium">{title}</span>
		<span class="tnum text-lg font-semibold">{current ?? show(values.at(-1) ?? 0)}</span>
	</figcaption>
	<div
		class="relative"
		role="img"
		aria-label="{title}, latest {current ?? show(values.at(-1) ?? 0)}"
		onpointermove={labels ? move : undefined}
		onpointerleave={leave}
	>
		<svg
			viewBox="0 0 {w} {h}"
			class="block h-36 w-full text-primary"
			preserveAspectRatio="none"
			aria-hidden="true"
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
			{#if bottom < 0}<line
					x1="0"
					x2={w}
					y1={y(0)}
					y2={y(0)}
					class="stroke-muted-foreground"
					stroke-width="1"
					vector-effect="non-scaling-stroke"
				/>{/if}
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
		{#if at !== null && labels}
			<!-- The dot is outside the stretched drawing, so it stays round. -->
			<span
				class="pointer-events-none absolute size-2 -translate-1/2 rounded-full bg-primary ring-2 ring-card"
				style="left: {share * 100}%; top: {(y(values[at] ?? 0) / h) * 100}%"
			></span>
			<div
				class="pointer-events-none absolute top-0 z-10 rounded-md border bg-popover px-1.5 py-0.5 text-xs whitespace-nowrap text-foreground shadow-sm"
				style={share > 0.6
					? `right: calc(${(1 - share) * 100}% + 0.5rem)`
					: `left: calc(${share * 100}% + 0.5rem)`}
			>
				<span class="text-muted-foreground">{labels[at]}</span>
				<span class="tnum font-medium">{show(values[at] ?? 0)}</span>
			</div>
		{/if}
	</div>
</figure>
