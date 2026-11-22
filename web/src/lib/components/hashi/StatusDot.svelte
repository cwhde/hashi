<script lang="ts">
	import type { Health } from '#lib/design/mock.ts';
	import { cn } from '#lib/utils.ts';
	import { health } from './status';

	// `label` names the dot in other words (the public pages' "Slow", "No data"); '' hides it from screen readers where
	// the state is already written beside it (bug 176).
	let {
		state,
		label,
		class: className = ''
	}: { state: Health; label?: string; class?: string } = $props();
</script>

{#if label === ''}
	<span
		class={cn('inline-block size-2 shrink-0 rounded-full', health[state].dot, className)}
		aria-hidden="true"
	></span>
{:else}
	<span
		class={cn('inline-block size-2 shrink-0 rounded-full', health[state].dot, className)}
		role="img"
		aria-label={label ?? health[state].label}
	></span>
{/if}
