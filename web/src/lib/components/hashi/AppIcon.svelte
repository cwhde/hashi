<script lang="ts">
	// An app's icon tile (07 §9.1): its icon, the white variant on dark themes when there is one, or its first letter.
	import { cn } from '#lib/utils.ts';

	let {
		name,
		icon,
		light,
		class: className = ''
	}: { name: string; icon?: string; light?: string; class?: string } = $props();
</script>

<span
	class={cn(
		'grid shrink-0 place-items-center overflow-hidden rounded-lg border bg-surface-2 font-semibold',
		className
	)}
>
	{#if icon}
		<!-- Lazy: the status page lists every service, and loading all 86 icons at once tripled its weight (bug 183). -->
		<img
			src={icon}
			alt=""
			loading="lazy"
			decoding="async"
			class={cn('size-3/4 object-contain', light && 'on-light')}
		/>
		{#if light}<img
				src={light}
				alt=""
				loading="lazy"
				decoding="async"
				class="on-dark size-3/4 object-contain"
			/>{/if}
	{:else}{name[0]?.toUpperCase()}{/if}
</span>

<style>
	.on-dark {
		display: none;
	}
	:global(:root:not([data-mode='light'])) .on-light {
		display: none;
	}
	:global(:root:not([data-mode='light'])) .on-dark {
		display: block;
	}
</style>
