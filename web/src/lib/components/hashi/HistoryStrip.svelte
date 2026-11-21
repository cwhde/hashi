<script lang="ts">
	import type { Health } from '#lib/design/mock.ts';
	import { cn } from '#lib/utils.ts';
	import { health } from './status';
	import { stripSummary } from '#lib/monitor.ts';

	// Gatus-style history: one bar per sample, oldest left. Labels sit at both ends (10a §5.5).
	let {
		samples,
		from = '60 min ago',
		to = 'now',
		labels = true,
		class: className = ''
	}: { samples: Health[]; from?: string; to?: string; labels?: boolean; class?: string } = $props();

	const summary = $derived(stripSummary(samples));
	// One path per state, each sample a bar of it: at most five elements whatever the length.
	const paths = $derived.by(() => {
		const w = samples.length > 1 ? 0.8 : 1;
		const out: Partial<Record<Health, string>> = {};
		samples.forEach((s, i) => (out[s] = (out[s] ?? '') + `M${i} 0h${w}v1h-${w}z`));
		return Object.entries(out) as [Health, string][];
	});
</script>

<div class={cn('min-w-0', className)}>
	<!-- One SVG per strip, not one element per sample: the public status page had 5,500 elements (13.11). -->
	<svg
		class="block h-6 w-full"
		viewBox="0 0 {Math.max(samples.length, 1)} 1"
		preserveAspectRatio="none"
		role="img"
		aria-label={summary}
	>
		{#each paths as [s, d] (s)}
			<path {d} class={cn(health[s].fill, s === 'up' && 'opacity-80')} />
		{/each}
	</svg>
	{#if labels}
		<div class="mt-1 flex justify-between text-2xs text-muted-foreground">
			<span>{from}</span><span>{to}</span>
		</div>
	{/if}
</div>
