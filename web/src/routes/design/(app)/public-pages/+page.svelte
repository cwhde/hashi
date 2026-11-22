<script lang="ts">
	// Mock (07 §8.1, reworked 2026-11-22 after the owner's review): Public pages, renamed from App display. Only the
	// layout lives here; page-wide options (on/off, look, default sort) stay in Settings → Public pages behind the gear
	// (10c §1.1, §6). One order for both pages. Dragging is live, as on a phone's home screen: the dragged entry follows
	// the pointer and the others slide out of its way, in the outline and in the preview alike. Every change saves at
	// once, with Undo. On a phone the outline only, and a Preview button.
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import {
		ArrowDownAZ,
		Eye,
		FolderInput,
		GripVertical,
		Plus,
		Settings,
		Trash2,
		X
	} from '@lucide/svelte';
	import LayoutRow from '../../LayoutRow.svelte';
	import { LayoutState, type Page } from '../../ideaLayout.svelte.ts';
	import { slide, sortable } from '#lib/sortable.ts';

	const s = new LayoutState();
	let preview = $state<Page>('apps');
	let flash = $state('');
	let sheet = $state(false);
	let renaming = $state('');
	const shown = $derived(
		s.groups
			.map((g) => ({
				name: g.name,
				list: g.ids.map((id) => s.items[id]!).filter((e) => s.shownOn(e, preview))
			}))
			.filter((g) => g.list.length)
	);
	function find(id: string) {
		sheet = false;
		flash = id;
		document.getElementById('o-' + id)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
		setTimeout(() => (flash = ''), 1200);
	}
	const domain = (name: string) => `${name.toLowerCase().replace(/[^a-z0-9]+/g, '')}.example.com`;
</script>

<PageHeader title="Public pages">
	{#snippet actions()}
		<Button variant="outline" size="sm" class="lg:hidden" onclick={() => (sheet = true)}
			><Eye />Preview</Button
		>
		<Button
			variant="ghost"
			size="icon"
			href="/design/settings"
			aria-label="Public pages settings"
			title="Settings"><Settings /></Button
		>
	{/snippet}
</PageHeader>

<div class="grid items-start gap-4 lg:grid-cols-[26rem_minmax(0,1fr)]">
	<div
		role="list"
		aria-label="Layout"
		data-sort-zone="group"
		class="rounded-2xl border bg-card p-2 lg:sticky lg:top-4 lg:max-h-[calc(100dvh-7rem)] lg:overflow-y-auto"
	>
		{#each s.groups as g (g.name)}
			<section
				animate:slide
				use:sortable={{
					host: s,
					kind: 'group',
					id: g.name,
					handle: '[data-group-grip]',
					handleOnly: true
				}}
				data-sort-kind="group"
				data-sort-id={g.name}
				aria-label={g.name}
				class="mb-1 rounded-xl bg-card pb-1"
			>
				<header class="flex items-center gap-1 rounded-lg px-1 py-1">
					{#if g.name !== 'Other'}<span
							data-group-grip
							class="grid size-6 shrink-0 cursor-grab touch-none place-items-center text-muted-foreground"
							><GripVertical class="size-4" /></span
						>{:else}<span class="w-6"></span>{/if}
					{#if renaming === g.name}
						<!-- svelte-ignore a11y_autofocus -->
						<input
							autofocus
							class="h-7 flex-1 rounded-md border bg-background px-2 text-md font-semibold"
							value={g.name}
							aria-label="Group name"
							onblur={(ev) => {
								s.rename(g.name, ev.currentTarget.value);
								renaming = '';
							}}
							onkeydown={(ev) => ev.key === 'Enter' && ev.currentTarget.blur()}
						/>
					{:else}
						<button
							type="button"
							class="flex-1 truncate text-left text-md font-semibold"
							disabled={g.name === 'Other'}
							title={g.name === 'Other' ? undefined : 'Rename'}
							onclick={() => (renaming = g.name)}>{g.name}</button
						>
					{/if}
					<span class="tnum px-1 text-xs text-muted-foreground">{g.ids.length}</span>
					<Button
						variant="ghost"
						size="icon"
						class="size-7"
						aria-label="Sort {g.name} A to Z"
						title="Sort A–Z"
						onclick={() => s.sortAZ(g.name)}><ArrowDownAZ /></Button
					>
					{#if !g.ids.length && g.name !== 'Other'}
						<Button
							variant="ghost"
							size="icon"
							class="size-7"
							aria-label="Delete {g.name}"
							onclick={() => (s.groups = s.groups.filter((x) => x.name !== g.name))}
							><Trash2 /></Button
						>
					{/if}
				</header>
				<div data-sort-zone="item" data-sort-group={g.name} class="min-h-8 space-y-1 pl-2">
					{#each g.ids as id (id)}
						<div
							animate:slide
							use:sortable={{ host: s, kind: 'item', id, handle: '[data-grip]' }}
							data-sort-kind="item"
							data-sort-id={id}
							data-sort-group={g.name}
							class="rounded-lg"
						>
							<LayoutRow {s} {id} {preview} flash={flash === id} />
						</div>
					{:else}
						<p class="rounded-lg border border-dashed px-2 py-1.5 text-sm text-muted-foreground">
							Empty
						</p>
					{/each}
				</div>
			</section>
		{/each}
		<Button variant="ghost" size="sm" onclick={() => s.addGroup()}><Plus />New group</Button>
	</div>
	<div class="hidden lg:block">{@render page()}</div>
</div>

<Sheet.Root bind:open={sheet}>
	<Sheet.Content side="bottom" class="max-h-[90dvh] overflow-y-auto p-0">
		<Sheet.Title class="sr-only">Preview</Sheet.Title>
		{@render page()}
	</Sheet.Content>
</Sheet.Root>

{#if s.selected.length}
	<div
		class="sticky bottom-4 z-10 mx-auto mt-4 flex w-fit flex-wrap items-center gap-2 rounded-2xl border bg-popover px-3 py-2 shadow-lg"
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
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}<Button {...props} size="sm" variant="outline"
						>Show or hide</Button
					>{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content>
				<DropdownMenu.Item onclick={() => s.setSelected('apps', true)}
					>Show on the apps page</DropdownMenu.Item
				>
				<DropdownMenu.Item onclick={() => s.setSelected('apps', false)}
					>Hide from the apps page</DropdownMenu.Item
				>
				<DropdownMenu.Item onclick={() => s.setSelected('status', true)}
					>Show on the status page</DropdownMenu.Item
				>
				<DropdownMenu.Item onclick={() => s.setSelected('status', false)}
					>Hide from the status page</DropdownMenu.Item
				>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
		<Button size="sm" variant="ghost" onclick={() => (s.selected = [])}><X />Clear</Button>
	</div>
{:else if s.last}
	<div
		role="status"
		class="sticky bottom-4 z-10 mx-auto mt-4 flex w-fit items-center gap-3 rounded-2xl border bg-popover px-3 py-2 shadow-lg"
	>
		<span class="text-md">{s.last.text}. Saved.</span>
		<Button size="sm" variant="outline" onclick={() => s.undo()}>Undo</Button>
	</div>
{/if}
<p class="sr-only" aria-live="polite">{s.said}</p>

{#snippet page()}
	<div class="overflow-hidden rounded-2xl border bg-background">
		<div class="flex flex-wrap items-center gap-2 border-b bg-surface-2/60 px-3 py-1.5">
			<span class="flex-1 truncate text-xs text-muted-foreground"
				>{preview === 'apps' ? 'apps.example.com' : 'status.example.com'}</span
			>
			<ToggleGroup.Root type="single" bind:value={preview} variant="outline" size="sm">
				<ToggleGroup.Item value="apps">Apps page</ToggleGroup.Item>
				<ToggleGroup.Item value="status">Status page</ToggleGroup.Item>
			</ToggleGroup.Root>
		</div>
		<div class="p-5">
			<h2 class="mb-3 text-title font-semibold">
				{preview === 'apps' ? 'example.com' : 'example.com status'}
			</h2>
			{#each shown as g (g.name)}
				<h3 class="mt-4 mb-2 text-md font-semibold text-muted-foreground">{g.name}</h3>
				{#if preview === 'apps'}
					<div
						data-sort-zone="item"
						data-sort-group={g.name}
						class="grid gap-2 sm:grid-cols-2 xl:grid-cols-3"
					>
						{#each g.list as e (e.id)}
							<div
								animate:slide
								use:sortable={{ host: s, kind: 'item', id: e.id }}
								data-sort-kind="item"
								data-sort-id={e.id}
								data-sort-group={g.name}
								data-sort-axis="x"
								class="rounded-2xl"
							>
								<button
									type="button"
									data-sort-body
									onclick={() => find(e.id)}
									class="flex w-full cursor-grab items-center gap-3 rounded-2xl border bg-card p-3 text-left hover:border-muted-foreground/40"
								>
									<AppIcon name={e.name} class="size-9 rounded-xl" />
									<span class="min-w-0 flex-1">
										<span class="block truncate text-md font-medium">{e.name}</span>
										<span class="block truncate text-xs text-muted-foreground"
											>{domain(e.name)}</span
										>
									</span>
								</button>
							</div>
						{/each}
					</div>
				{:else}
					<div
						data-sort-zone="item"
						data-sort-group={g.name}
						class="divide-y overflow-hidden rounded-2xl border bg-card"
					>
						{#each g.list as e (e.id)}
							<div
								animate:slide
								use:sortable={{ host: s, kind: 'item', id: e.id }}
								data-sort-kind="item"
								data-sort-id={e.id}
								data-sort-group={g.name}
								class="bg-card"
							>
								<button
									type="button"
									data-sort-body
									onclick={() => find(e.id)}
									class="grid w-full cursor-grab grid-cols-[9rem_minmax(0,1fr)_5.5rem] items-center gap-3 px-3 py-2 text-left"
								>
									<span class="flex min-w-0 items-center gap-2 text-md font-medium"
										><AppIcon name={e.name} class="size-5 rounded text-[10px]" /><span
											class="truncate">{e.name}</span
										></span
									>
									<HistoryStrip samples={e.days} labels={false} />
									<span class="text-right"><StatusBadge state={e.state} /></span>
								</button>
							</div>
						{/each}
					</div>
				{/if}
			{/each}
		</div>
	</div>
{/snippet}
