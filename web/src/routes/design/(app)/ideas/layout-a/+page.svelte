<script lang="ts">
	// Idea A: the board IS the page. "Show as" draws the layout like the apps page (cards) or like the status page
	// (rows with strips); you drag right where the result shows. One order for both pages.
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { ArrowDownAZ, GripVertical, Plus } from '@lucide/svelte';
	import IdeaCard from '../../../IdeaCard.svelte';
	import IdeaEditorHead from '../../../IdeaEditorHead.svelte';
	import IdeaSelectionBar from '../../../IdeaSelectionBar.svelte';
	import { LayoutState, type Page } from '../../../ideaLayout.svelte.ts';

	const s = new LayoutState();
	let preview = $state<Page>('apps');
	let editing = $state('');
</script>

<IdeaEditorHead bind:preview variant="A: edit on the page" />

<div class="space-y-5">
	{#each s.groups as g (g.name)}
		<section
			{...s.dropOnGroup(g.name)}
			class="rounded-2xl border p-3 {s.dragging?.kind === 'group' && s.dragging.id !== g.name
				? 'border-dashed border-primary/50'
				: 'border-transparent'}"
		>
			<header
				{...s.dragGroup(g.name)}
				class="mb-2 flex items-center gap-2 {g.name !== 'Other' ? 'cursor-grab' : ''}"
			>
				{#if g.name !== 'Other'}<GripVertical class="size-4 text-muted-foreground" />{/if}
				{#if editing === g.name}
					<!-- svelte-ignore a11y_autofocus -->
					<input
						autofocus
						value={g.name}
						class="rounded-md border bg-background px-2 py-0.5 text-base font-semibold"
						onblur={(ev) => {
							s.rename(g.name, ev.currentTarget.value);
							editing = '';
						}}
						onkeydown={(ev) => ev.key === 'Enter' && ev.currentTarget.blur()}
					/>
				{:else}
					<button
						type="button"
						class="text-base font-semibold hover:underline disabled:no-underline"
						disabled={g.name === 'Other'}
						onclick={() => (editing = g.name)}>{g.name}</button
					>
				{/if}
				<span class="tnum text-sm text-muted-foreground"
					>{g.ids.filter((id) => s.shownOn(s.items[id]!, preview)).length} shown</span
				>
				<Button variant="ghost" size="sm" class="ml-auto" onclick={() => s.sortAZ(g.name)}
					><ArrowDownAZ />A–Z</Button
				>
			</header>
			{#if g.ids.length === 0}
				<p class="rounded-xl border border-dashed p-4 text-center text-md text-muted-foreground">
					Drop entries here
				</p>
			{:else if preview === 'apps'}
				<div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
					{#each g.ids as id (id)}<IdeaCard {s} {id} group={g.name} {preview} />{/each}
				</div>
			{:else}
				<div class="space-y-1.5">
					{#each g.ids as id (id)}
						<div class="grid items-center gap-3 lg:grid-cols-[22rem_minmax(0,1fr)]">
							<IdeaCard {s} {id} group={g.name} {preview} compact />
							{#if s.items[id]!.status}<HistoryStrip
									samples={s.items[id]!.history}
									labels={false}
								/>{/if}
						</div>
					{/each}
				</div>
			{/if}
		</section>
	{/each}
	<Button variant="outline" onclick={() => s.addGroup()}><Plus />New group</Button>
</div>
<IdeaSelectionBar {s} />
