<script lang="ts">
	// Thin line, no fill, no axes (10a §5.2 item 11). Colour comes from the parent's text colour. With `labels`, a
	// pointer over the line shows that moment's value (bug 119); nothing reacts until the pointer is on it. With `sync`
	// and `times`, sparklines of one page share the pointer: the same moment shows in each of them.
	import { nearest, type ChartSync } from '#lib/chart-sync.svelte.ts';
	import { scrub } from '#lib/scrub.ts';

	let {
		values,
		width = 120,
		height = 28,
		labels,
		format = (v: number) => v.toLocaleString(),
		class: className = '',
		times,
		sync
	}: {
		values: number[];
		width?: number;
		height?: number;
		labels?: string[];
		format?: (v: number) => string;
		class?: string;
		/** Each point's moment (ms); with `sync`, the pointer is shared with the page's other charts. */
		times?: number[];
		sync?: ChartSync;
	} = $props();

	const max = $derived(Math.max(...values, 1));
	const min = $derived(Math.min(...values, 0));
	const step = $derived(width / Math.max(values.length - 1, 1));
	const y = (v: number) => height - 2 - ((v - min) / (max - min || 1)) * (height - 4);
	const points = $derived(
		values.map((v, i) => `${(i * step).toFixed(1)},${y(v).toFixed(1)}`).join(' ')
	);

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
</script>

{#if labels}
	<div
		class="relative {className}"
		role="img"
		aria-label="Trend"
		use:scrub
		onpointermove={move}
		onpointerdown={move}
		onpointerleave={leave}
	>
		<svg
			class="block w-full"
			viewBox="0 0 {width} {height}"
			{height}
			aria-hidden="true"
			preserveAspectRatio="none"
		>
			<polyline
				{points}
				fill="none"
				stroke="currentColor"
				stroke-width="1.5"
				stroke-linejoin="round"
				vector-effect="non-scaling-stroke"
			/>
			{#if at !== null}<line
					x1={at * step}
					x2={at * step}
					y1="0"
					y2={height}
					class="stroke-border"
					stroke-width="1"
					vector-effect="non-scaling-stroke"
				/>{/if}
		</svg>
		{#if at !== null}
			<div
				class="pointer-events-none absolute -top-7 z-10 rounded-md border bg-popover px-1.5 py-0.5 text-xs whitespace-nowrap text-foreground shadow-sm"
				style="left: clamp(0px, calc({(at / Math.max(1, values.length - 1)) *
					100}% - 3rem), calc(100% - 6rem))"
			>
				<span class="text-muted-foreground">{labels[at]}</span>
				<span class="tnum font-medium">{format(values[at] ?? 0)}</span>
			</div>
		{/if}
	</div>
{:else}
	<svg
		class={className}
		viewBox="0 0 {width} {height}"
		{width}
		{height}
		aria-hidden="true"
		preserveAspectRatio="none"
	>
		<polyline
			{points}
			fill="none"
			stroke="currentColor"
			stroke-width="1.5"
			stroke-linejoin="round"
			vector-effect="non-scaling-stroke"
		/>
	</svg>
{/if}
