<script lang="ts">
	import type { Snippet } from 'svelte';

	// Small title, an optional help icon (10c §4.7), one muted line, actions right. Never a hero (10-ui.md §1.1).
	let {
		title,
		description,
		help,
		actions
	}: { title: string; description?: string; help?: Snippet; actions?: Snippet } = $props();
</script>

<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
	<div class="min-w-0 flex-1">
		<h1 class="flex items-center gap-1.5 text-title leading-tight font-semibold tracking-[-0.01em]">
			{title}{@render help?.()}
		</h1>
		{#if description}<p class="mt-1 max-w-[70ch] text-md text-muted-foreground">
				{description}
			</p>{/if}
	</div>
	{#if actions}<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>{/if}
</div>
