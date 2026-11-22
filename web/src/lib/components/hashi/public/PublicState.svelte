<script lang="ts">
	// A state on the public pages (07 §7): always the word, then the dot, so every state reads the same way and the
	// dots end on one right edge. Visitors' words: Slow, not Degraded; No data, not Unknown.
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import { health } from '#lib/components/hashi/status.ts';
	import type { Health } from '#lib/design/mock.ts';
	import { stateWord } from '#lib/public.ts';
	import { cn } from '#lib/utils.ts';

	let { state, class: cls = '' }: { state: string; class?: string } = $props();
	const h = $derived((state in health ? state : 'unknown') as Health);
</script>

<span
	class={cn(
		'inline-flex items-center justify-end gap-1.5 text-md font-medium whitespace-nowrap',
		health[h].text,
		cls
	)}
>
	{stateWord(state)}<StatusDot state={h} />
</span>
