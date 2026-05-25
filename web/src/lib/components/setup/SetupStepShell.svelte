<script lang="ts">
	import type { SetupStepDef } from '$lib/setup/steps';
	import type { Snippet } from 'svelte';

	let {
		step,
		children
	}: {
		step: SetupStepDef;
		children: Snippet;
	} = $props();

	const Icon = $derived(step.icon);
</script>

<div class="space-y-4">
	<div class="flex items-start gap-3 border-b border-border pb-4">
		<div
			class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-hashi-hover/20 text-hashi-contrast"
		>
			<Icon class="size-5" aria-hidden="true" />
		</div>
		<div class="min-w-0">
			<h2 class="text-lg font-semibold text-white">{step.title}</h2>
			<p class="text-sm text-muted-foreground">{step.description}</p>
		</div>
	</div>
	{@render children()}
</div>
