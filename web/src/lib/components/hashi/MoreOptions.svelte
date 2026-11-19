<script lang="ts">
	// The one disclosure section (10c §2, §3): closed by default, and its label says how many fields inside differ
	// from their defaults, so a changed value is never hidden without a hint. It starts open when a value inside
	// already differs as the form loads, so a stored non-default value is in view (10c §3).
	import type { Snippet } from 'svelte';
	import * as Collapsible from '#lib/components/ui/collapsible/index.ts';
	import { ChevronRight } from '@lucide/svelte';

	let {
		label = 'More options',
		changed = 0,
		open = $bindable(changed > 0),
		class: className = '',
		children
	}: {
		label?: string;
		changed?: number;
		open?: boolean;
		class?: string;
		children: Snippet;
	} = $props();
</script>

<Collapsible.Root bind:open class={className}>
	<Collapsible.Trigger
		class="group flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
		><ChevronRight
			class="size-4 transition-transform group-data-[state=open]:rotate-90"
		/>{label}{changed ? ` · ${changed} changed` : ''}</Collapsible.Trigger
	>
	<Collapsible.Content class="space-y-3 pt-3">{@render children()}</Collapsible.Content>
</Collapsible.Root>
