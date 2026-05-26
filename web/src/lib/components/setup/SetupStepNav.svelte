<script lang="ts">
	import { SETUP_STEPS, isStepComplete } from '$lib/setup/steps';
	import { cn } from '$lib/utils';
	import { Check } from 'lucide-svelte';

	let {
		currentSlug,
		completedSteps
	}: {
		currentSlug: string;
		completedSteps: string[];
	} = $props();
</script>

<ol class="space-y-1">
	{#each SETUP_STEPS as step, index (step.slug)}
		{@const Icon = step.icon}
		{@const done = isStepComplete(step.slug, completedSteps)}
		{@const current = step.slug === currentSlug}
		<li>
			<div
				class={cn(
					'flex items-center gap-2 rounded-md px-2 py-1.5 text-xs',
					current && 'bg-hashi-hover/25 text-white',
					!current && done && 'text-emerald-300',
					!current && !done && 'text-muted-foreground'
				)}
			>
				<span
					class={cn(
						'flex size-5 shrink-0 items-center justify-center rounded text-[10px] font-semibold',
						done ? 'bg-emerald-500/20' : 'bg-muted'
					)}
				>
					{#if done}
						<Check class="size-3" />
					{:else}
						{index + 1}
					{/if}
				</span>
				<Icon class="size-3.5 shrink-0 opacity-70" aria-hidden="true" />
				<span class="truncate">{step.title}</span>
			</div>
		</li>
	{/each}
</ol>
