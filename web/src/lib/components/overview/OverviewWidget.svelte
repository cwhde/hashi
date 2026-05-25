<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		description,
		children
	}: {
		title: string;
		description?: string;
		children?: Snippet;
	} = $props();
</script>

<div class="rounded-lg border border-border bg-card/40 p-4">
	<div class="mb-2">
		<h3 class="text-sm font-medium text-white">{title}</h3>
		{#if description}
			<p class="text-xs text-muted-foreground">{description}</p>
		{/if}
	</div>
	{#if children}
		{@render children()}
	{/if}
</div>
