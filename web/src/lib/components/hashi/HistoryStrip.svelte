<script lang="ts">
	import type { Health } from '$lib/design/mock';
	import { cn } from '$lib/utils';
	import { health } from './status';

	// Gatus-style history: one bar per sample, oldest left. Labels sit at both ends (10a §5.5).
	let {
		samples,
		from = '60 min ago',
		to = 'now',
		labels = true,
		class: className = ''
	}: { samples: Health[]; from?: string; to?: string; labels?: boolean; class?: string } = $props();

	const summary = $derived.by(() => {
		const bad = samples.filter((s) => s === 'down').length;
		const slow = samples.filter((s) => s === 'degraded').length;
		return `${samples.length} checks: ${bad} down, ${slow} degraded`;
	});
</script>

<div class={cn('min-w-0', className)}>
	<div class="flex h-6 items-stretch gap-px" role="img" aria-label={summary}>
		{#each samples as s, i (i)}
			<span class={cn('min-w-px flex-1 rounded-[1px]', health[s].bar, s === 'up' && 'opacity-80')}
			></span>
		{/each}
	</div>
	{#if labels}
		<div class="mt-1 flex justify-between text-[11px] text-muted-foreground">
			<span>{from}</span><span>{to}</span>
		</div>
	{/if}
</div>
