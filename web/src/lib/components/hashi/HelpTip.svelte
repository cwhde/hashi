<script lang="ts">
	// One help icon per panel for a concept that needs explaining (10c §4.7): at most three sentences, in a popover
	// instead of a paragraph on the page.
	import type { Snippet } from 'svelte';
	import * as Popover from '#lib/components/ui/popover/index.ts';
	import { CircleHelp } from '@lucide/svelte';

	let { topic, children }: { topic: string; children: Snippet } = $props();
</script>

<Popover.Root>
	<Popover.Trigger
		class="inline-flex size-6 items-center justify-center rounded-full align-middle text-muted-foreground hover:text-foreground"
		aria-label="About {topic}"><CircleHelp class="size-4" /></Popover.Trigger
	>
	<Popover.Content class="w-[min(20rem,calc(100vw-2rem))] space-y-2 text-md" align="start"
		>{@render children()}</Popover.Content
	>
</Popover.Root>
