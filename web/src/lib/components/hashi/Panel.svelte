<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	// The one container: rounded, 1px border, surface fill, no shadow. Optional header row with title + actions.
	let {
		title,
		description,
		actions,
		children,
		flush = false,
		class: className = ''
	}: {
		title?: string;
		description?: string;
		actions?: Snippet;
		children: Snippet;
		flush?: boolean;
		class?: string;
	} = $props();
</script>

<section class={cn('min-w-0 rounded-2xl border bg-card', className)}>
	{#if title || actions}
		<header class="flex min-h-12 items-center gap-3 px-4 py-2.5 {flush ? 'border-b' : ''}">
			<div class="min-w-0 flex-1">
				{#if title}<h2 class="truncate text-[14px] font-semibold">{title}</h2>{/if}
				{#if description}<p class="truncate text-[12.5px] text-muted-foreground">
						{description}
					</p>{/if}
			</div>
			{#if actions}<div class="flex shrink-0 items-center gap-1.5">{@render actions()}</div>{/if}
		</header>
	{/if}
	<div class={flush ? '' : 'px-4 pb-4 ' + (title || actions ? '' : 'pt-4')}>
		{@render children()}
	</div>
</section>
