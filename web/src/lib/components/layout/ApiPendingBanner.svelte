<script lang="ts" module>
	export function apiUnavailable(feature: string): string {
		return `Waiting for ${feature} API — UI is wired and will activate when the endpoint ships.`;
	}
</script>

<script lang="ts">
	import { Clock } from 'lucide-svelte';
	import { cn } from '$lib/utils';

	let {
		message = 'Backend API not available yet.',
		detail,
		class: className
	}: {
		message?: string;
		detail?: string;
		class?: string;
	} = $props();
</script>

<div
	class={cn(
		'flex items-start gap-2 rounded-md border border-hashi-highlight/30 bg-hashi-highlight/10 px-3 py-2 text-xs',
		className
	)}
	role="status"
>
	<Clock class="mt-0.5 size-3.5 shrink-0 text-hashi-highlight" aria-hidden="true" />
	<div class="min-w-0">
		<p class="font-medium text-hashi-contrast">{message}</p>
		{#if detail}
			<p class="mt-0.5 text-muted-foreground">{detail}</p>
		{/if}
	</div>
</div>
