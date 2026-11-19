<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.ts';

	// The one container: rounded, 1px border, surface fill, no shadow. Optional header row with title, a help icon
	// (10c §4.7) and actions.
	let {
		title,
		description,
		help,
		actions,
		children,
		flush = false,
		class: className = ''
	}: {
		title?: string;
		description?: string;
		help?: Snippet;
		actions?: Snippet;
		children: Snippet;
		flush?: boolean;
		class?: string;
	} = $props();
</script>

<!-- A column: the body fills a card stretched by its grid row, so widgets can use the height (bug 142). -->
<section class={cn('flex min-w-0 flex-col rounded-2xl border bg-card', className)}>
	{#if title || actions}
		<header class="flex min-h-12 items-center gap-3 px-4 py-2.5 {flush ? 'border-b' : ''}">
			<div class="min-w-0 flex-1">
				{#if title}<h2 class="flex items-center gap-1 text-base font-semibold">
						<span class="truncate">{title}</span>{@render help?.()}
					</h2>{/if}
				{#if description}<p class="truncate text-sm text-muted-foreground">
						{description}
					</p>{/if}
			</div>
			{#if actions}<div class="flex shrink-0 items-center gap-1.5">{@render actions()}</div>{/if}
		</header>
	{/if}
	<div class={'flex-1 ' + (flush ? '' : 'px-4 pb-4 ' + (title || actions ? '' : 'pt-4'))}>
		{@render children()}
	</div>
</section>
