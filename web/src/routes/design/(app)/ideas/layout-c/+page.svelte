<script lang="ts">
	// Idea C: groups as columns side by side (a board, like Trello). Good for moving between groups at a glance; the
	// order inside a column is the order on both pages. The page itself is one click away (preview).
	import { Button } from '#lib/components/ui/button/index.ts';
	import { ArrowDownAZ, ExternalLink, GripVertical, Plus } from '@lucide/svelte';
	import IdeaCard from '../../../IdeaCard.svelte';
	import IdeaEditorHead from '../../../IdeaEditorHead.svelte';
	import IdeaSelectionBar from '../../../IdeaSelectionBar.svelte';
	import { LayoutState, type Page } from '../../../ideaLayout.svelte.ts';

	const s = new LayoutState();
	let preview = $state<Page>('apps');
</script>

<IdeaEditorHead bind:preview variant="C: columns">
	{#snippet extra()}
		<Button
			variant="outline"
			size="sm"
			href={preview === 'apps' ? '/design/public-apps' : '/design/ideas/status-a'}
			><ExternalLink />Open preview</Button
		>
	{/snippet}
</IdeaEditorHead>

<div class="flex items-start gap-3 overflow-x-auto pb-3">
	{#each s.groups as g (g.name)}
		<section
			{...s.dropOnGroup(g.name)}
			class="w-64 shrink-0 rounded-2xl border bg-surface-2/40 p-2 {s.dragging?.kind === 'group' &&
			s.dragging.id !== g.name
				? 'border-dashed border-primary/50'
				: ''}"
		>
			<header
				{...s.dragGroup(g.name)}
				class="mb-2 flex items-center gap-1.5 px-1 {g.name !== 'Other' ? 'cursor-grab' : ''}"
			>
				{#if g.name !== 'Other'}<GripVertical class="size-4 text-muted-foreground" />{/if}
				<span class="flex-1 truncate text-md font-semibold">{g.name}</span>
				<span class="tnum text-xs text-muted-foreground">{g.ids.length}</span>
				<Button
					variant="ghost"
					size="icon"
					class="size-7"
					title="Sort A–Z"
					onclick={() => s.sortAZ(g.name)}><ArrowDownAZ /></Button
				>
			</header>
			<div class="space-y-1.5">
				{#each g.ids as id (id)}<IdeaCard {s} {id} group={g.name} {preview} compact />{:else}
					<p
						class="rounded-lg border border-dashed px-2 py-6 text-center text-sm text-muted-foreground"
					>
						Drop entries here
					</p>
				{/each}
			</div>
		</section>
	{/each}
	<Button variant="outline" class="shrink-0" onclick={() => s.addGroup()}><Plus />New group</Button>
</div>
<IdeaSelectionBar {s} />
