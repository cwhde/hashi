<script lang="ts">
	// The history strip with details (07 §7.1, §7.2): one path per state (13.11); the bar under the pointer is one extra
	// rect drawn on top that grows, the rest dim, and a bubble says what happened in that bar: when, how long, down or
	// slow. Arrow keys walk the bars; a tap shows the bubble on touch.
	import type { Health } from '#lib/design/mock.ts';
	import { stripText, type Detail } from '#lib/public.ts';
	import { health } from '#lib/components/hashi/status.ts';
	import { cn } from '#lib/utils.ts';
	import { scrub, mouseOnly } from '#lib/scrub.ts';

	let {
		samples,
		detail,
		from = '90 days ago',
		to = 'today',
		labels = true,
		unit = 'day',
		class: className = ''
	}: {
		samples: Health[];
		/** What one bar is, for the screen reader's summary. */
		unit?: 'day' | 'check';
		detail: (i: number) => Detail;
		from?: string;
		to?: string;
		labels?: boolean;
		class?: string;
	} = $props();

	let at = $state<number | null>(null);
	const n = $derived(Math.max(samples.length, 1));
	const summary = $derived(stripText(samples, unit));
	const paths = $derived.by(() => {
		const out: Partial<Record<Health, string>> = {};
		samples.forEach((s, i) => (out[s] = (out[s] ?? '') + `M${i} 0.15h0.8v0.7h-0.8z`));
		return Object.entries(out) as [Health, string][];
	});
	const d = $derived(at === null ? null : detail(at));
	const share = $derived(at === null ? 0 : (at + 0.4) / n);

	function point(e: PointerEvent) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		at = Math.max(0, Math.min(n - 1, Math.floor(((e.clientX - r.left) / r.width) * n)));
	}
	function key(e: KeyboardEvent) {
		const by = { ArrowLeft: -1, ArrowRight: 1, Home: -n, End: n }[e.key];
		if (!by) return;
		e.preventDefault();
		at = Math.max(0, Math.min(n - 1, (at ?? n - 1) + by));
	}
</script>

<div class={cn('min-w-0', className)}>
	<div
		class="relative rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
		role="slider"
		tabindex="0"
		aria-label={summary}
		use:scrub={() => (at = null)}
		aria-valuemin={0}
		aria-valuemax={n - 1}
		aria-valuenow={at ?? n - 1}
		aria-valuetext={d ? `${d.when}: ${d.text}` : summary}
		onpointermove={point}
		onpointerdown={point}
		onpointerleave={mouseOnly(() => (at = null))}
		onfocus={() => (at ??= n - 1)}
		onblur={() => (at = null)}
		onkeydown={key}
	>
		<svg class="block h-7 w-full" viewBox="0 0 {n} 1" preserveAspectRatio="none" aria-hidden="true">
			<g class="transition-opacity duration-150 {at === null ? '' : 'opacity-40'}">
				{#each paths as [s, p] (s)}
					<path d={p} class={cn(health[s].fill, s === 'up' && 'opacity-80')} />
				{/each}
			</g>
			{#if at !== null}
				{#key at}
					<rect
						x={at - 0.1}
						y="0"
						width="1"
						height="1"
						rx="0.15"
						class="grow {health[samples[at] ?? 'unknown'].fill}"
					/>
				{/key}
			{/if}
		</svg>
		{#if d}
			<div
				class="pointer-events-none absolute bottom-full z-20 mb-2 w-max max-w-64 rounded-lg border bg-popover px-2.5 py-1.5 text-sm shadow-md"
				style="left:{share * 100}%; transform:translateX(-{share * 100}%)"
				aria-hidden="true"
			>
				<span class="block font-medium">{d.when}</span>
				<span class="block">{d.text}</span>
				{#if d.extra}<span class="tnum block text-muted-foreground">{d.extra}</span>{/if}
			</div>
		{/if}
	</div>
	{#if labels}
		<div class="mt-1 flex justify-between text-2xs text-muted-foreground">
			<span>{from}</span><span>{to}</span>
		</div>
	{/if}
</div>

<style>
	.grow {
		transform-box: fill-box;
		transform-origin: center;
		animation: grow 140ms ease-out both;
	}
	@keyframes grow {
		from {
			transform: scale(0.8, 0.7);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.grow {
			animation: none;
		}
	}
</style>
