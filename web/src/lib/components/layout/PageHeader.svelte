<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { LucideIcon } from '$lib/icons';

	let {
		title,
		description,
		icon: Icon,
		actions,
		children
	}: {
		title: string;
		description?: string;
		icon?: LucideIcon;
		actions?: Snippet;
		children?: Snippet;
	} = $props();
</script>

<div class="space-y-4">
	<div class="flex items-start justify-between gap-4">
		<div class="flex min-w-0 items-start gap-3">
			{#if Icon}
				<div
					class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-hashi-hover/20 text-hashi-contrast"
				>
					<Icon class="size-4" aria-hidden="true" />
				</div>
			{/if}
			<div class="min-w-0">
				<h2 class="truncate text-lg font-semibold text-white">{title}</h2>
				{#if description}
					<p class="text-sm text-muted-foreground">{description}</p>
				{/if}
			</div>
		</div>
		{#if actions}
			<div class="flex shrink-0 items-center gap-2">
				{@render actions()}
			</div>
		{/if}
	</div>
	{#if children}
		{@render children()}
	{/if}
</div>
