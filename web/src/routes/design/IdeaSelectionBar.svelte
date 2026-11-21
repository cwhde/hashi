<script lang="ts">
	// Idea mock: with entries selected, move them all at once ("these 10 go to Media") instead of dragging each.
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { FolderInput, X } from '@lucide/svelte';
	import type { LayoutState } from './ideaLayout.svelte.ts';

	let { s }: { s: LayoutState } = $props();
</script>

{#if s.selected.length}
	<div
		class="sticky bottom-4 z-10 mx-auto mt-4 flex w-fit items-center gap-2 rounded-2xl border bg-popover px-3 py-2 shadow-lg"
	>
		<span class="tnum text-md font-medium">{s.selected.length} selected</span>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}<Button {...props} size="sm"><FolderInput />Move to</Button
					>{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content>
				{#each s.groups as g (g.name)}
					<DropdownMenu.Item onclick={() => s.move(s.selected[0]!, g.name)}
						>{g.name}</DropdownMenu.Item
					>
				{/each}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
		<Button size="sm" variant="ghost" onclick={() => (s.selected = [])}><X />Clear</Button>
	</div>
{/if}
