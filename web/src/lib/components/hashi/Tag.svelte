<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	// Quiet label for origin, kind, mode. tone="risk" is the only loud one.
	let {
		children,
		tone = 'neutral',
		class: className = ''
	}: {
		children: Snippet;
		tone?: 'neutral' | 'accent' | 'ok' | 'warn' | 'bad' | 'info';
		class?: string;
	} = $props();

	const tones = {
		neutral: 'bg-surface-2 text-muted-foreground border-border',
		accent: 'bg-primary/12 text-foreground border-primary/35',
		ok: 'bg-ok/10 text-ok-text border-ok/25',
		warn: 'bg-warn/10 text-warn-text border-warn/30',
		bad: 'bg-bad/10 text-bad-text border-bad/30',
		info: 'bg-info/10 text-info-text border-info/25'
	};
</script>

<span
	class={cn(
		'inline-flex h-5 items-center gap-1 rounded-md border px-1.5 text-[11.5px] font-medium whitespace-nowrap',
		tones[tone],
		className
	)}>{@render children()}</span
>
