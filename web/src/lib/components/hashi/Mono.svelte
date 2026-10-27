<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	// Literal technical values only (IPs, records, hashes, paths) — never labels.
	let { children, class: className = '' }: { children: Snippet; class?: string } = $props();
</script>

<span class={cn('font-mono text-[12.5px] tracking-[-0.01em]', className)}>{@render children()}</span
>
