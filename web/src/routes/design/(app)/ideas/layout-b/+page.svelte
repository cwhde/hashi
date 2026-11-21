<script lang="ts">
	// Idea B: a dense outline to arrange on the left (40 entries fit on one screen), and the public page as visitors
	// see it on the right, updating as you drag. Click an entry in the preview to find it in the outline.
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { ArrowDownAZ, GripVertical, Plus } from '@lucide/svelte';
	import IdeaCard from '../../../IdeaCard.svelte';
	import IdeaEditorHead from '../../../IdeaEditorHead.svelte';
	import IdeaSelectionBar from '../../../IdeaSelectionBar.svelte';
	import { LayoutState, type Page } from '../../../ideaLayout.svelte.ts';

	const s = new LayoutState();
	let preview = $state<Page>('apps');
	let flash = $state('');
	const shown = $derived(
		s.groups
			.map((g) => ({
				name: g.name,
				list: g.ids.map((id) => s.items[id]!).filter((e) => s.shownOn(e, preview))
			}))
			.filter((g) => g.list.length)
	);
	function find(id: string) {
		flash = id;
		document.getElementById('o-' + id)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
		setTimeout(() => (flash = ''), 1200);
	}
</script>

<IdeaEditorHead bind:preview variant="B: outline + preview" />

<div class="grid items-start gap-4 lg:grid-cols-[24rem_minmax(0,1fr)]">
	<div
		class="rounded-2xl border bg-card p-2 lg:sticky lg:top-4 lg:max-h-[calc(100dvh-2rem)] lg:overflow-y-auto"
	>
		{#each s.groups as g (g.name)}
			<section {...s.dropOnGroup(g.name)} class="mb-2">
				<header
					{...s.dragGroup(g.name)}
					class="flex items-center gap-1.5 rounded-lg px-1.5 py-1 {g.name !== 'Other'
						? 'cursor-grab'
						: ''} {s.dragging?.kind === 'group' && s.dragging.id !== g.name ? 'bg-primary/10' : ''}"
				>
					{#if g.name !== 'Other'}<GripVertical class="size-4 text-muted-foreground" />{/if}
					<span class="flex-1 text-md font-semibold">{g.name}</span>
					<span class="tnum text-xs text-muted-foreground">{g.ids.length}</span>
					<Button
						variant="ghost"
						size="icon"
						class="size-7"
						title="Sort A–Z"
						onclick={() => s.sortAZ(g.name)}><ArrowDownAZ /></Button
					>
				</header>
				<div class="space-y-1 pl-3">
					{#each g.ids as id (id)}
						<div
							id="o-{id}"
							class="rounded-xl transition-shadow {flash === id ? 'ring-2 ring-warn' : ''}"
						>
							<IdeaCard {s} {id} group={g.name} {preview} compact />
						</div>
					{:else}
						<p class="rounded-lg border border-dashed px-2 py-2 text-sm text-muted-foreground">
							Drop entries here
						</p>
					{/each}
				</div>
			</section>
		{/each}
		<Button variant="ghost" size="sm" onclick={() => s.addGroup()}><Plus />New group</Button>
	</div>

	<div class="overflow-hidden rounded-2xl border bg-background">
		<div
			class="flex items-center gap-2 border-b bg-surface-2/60 px-3 py-1.5 text-xs text-muted-foreground"
		>
			<span class="size-2.5 rounded-full bg-bad/60"></span><span
				class="size-2.5 rounded-full bg-warn/60"
			></span><span class="size-2.5 rounded-full bg-ok/60"></span>
			<span class="ml-2">{preview === 'apps' ? 'apps.example.com' : 'status.example.com'}</span>
		</div>
		<div class="p-5">
			<h2 class="mb-4 text-title font-semibold">
				{preview === 'apps' ? 'example.com apps' : 'example.com status'}
			</h2>
			{#each shown as g (g.name)}
				<h3 class="mt-4 mb-2 text-md font-semibold text-muted-foreground">{g.name}</h3>
				{#if preview === 'apps'}
					<div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
						{#each g.list as e (e.id)}
							<button
								type="button"
								onclick={() => find(e.id)}
								class="flex items-center gap-3 rounded-2xl border bg-card p-3 text-left hover:border-muted-foreground/40"
							>
								<span
									class="grid size-9 place-items-center rounded-xl font-semibold text-white"
									style="background:hsl({e.hue} 45% 42%)">{e.letter}</span
								>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-md font-medium">{e.name}</span>
									<span class="block truncate text-xs text-muted-foreground"
										>{e.name.toLowerCase().replace(/\s+/g, '')}.example.com</span
									>
								</span>
							</button>
						{/each}
					</div>
				{:else}
					<ul class="divide-y rounded-2xl border bg-card">
						{#each g.list as e (e.id)}
							<li>
								<button
									type="button"
									onclick={() => find(e.id)}
									class="grid w-full grid-cols-[9rem_minmax(0,1fr)_6rem] items-center gap-3 px-3 py-2 text-left"
								>
									<span class="truncate text-md font-medium">{e.name}</span>
									<HistoryStrip samples={e.history} labels={false} />
									<span class="text-right"><StatusBadge state={e.state} /></span>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			{/each}
		</div>
	</div>
</div>
<IdeaSelectionBar {s} />
