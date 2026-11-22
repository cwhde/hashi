<script lang="ts">
	// Public pages (07 §8.1, approved mock design/public-pages; renamed from App display). Only the layout lives here;
	// page-wide options sit in Settings → Public pages behind the gear (10c §1.1, §6). One order for both pages: the
	// outline on the left, the page as visitors see it on the right. Dragging is live (ADR 0004) in both, and every
	// change saves at once, with Undo. On a phone the outline only, and a Preview button.
	import { onMount } from 'svelte';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import DayStrip from '#lib/components/hashi/DayStrip.svelte';
	import LayoutRow from '#lib/components/hashi/public/LayoutRow.svelte';
	import PublicState from '#lib/components/hashi/public/PublicState.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
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
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import type { Health } from '#lib/design/mock.ts';
	import { LayoutState, OTHER_KEY, type Page } from '#lib/public-layout.svelte.ts';
	import { slide, sortable } from '#lib/sortable.ts';
	import { dayDetail } from '#lib/public.ts';

	let s = $state<LayoutState | null>(null);
	let failed = $state('');
	let icons = $state<Record<string, Schemas['AppIcon']>>({});
	let links = $state<Schemas['PublicPages'] | null>(null);
	let preview = $state<Page>('apps');
	let flash = $state('');
	let sheet = $state(false);
	let renaming = $state('');

	const io = {
		put: (body: Schemas['PublicLayoutSave']) => api.PUT('/api/public-layout', { body }),
		get: () => api.GET('/api/public-layout'),
		explain: problemText
	};
	onMount(async () => {
		const r = await api.GET('/api/public-layout');
		if (!r.data) {
			failed = problemText(r.error);
			return;
		}
		s = new LayoutState(r.data, io);
		void api.GET('/api/app-icons').then((x) => {
			for (const a of x.data?.apps ?? []) icons[a.app] = a;
		});
		void api.GET('/api/app-display/pages').then((x) => (links = x.data ?? null));
	});

	const shown = $derived(
		s
			? s.groups
					.map((g) => ({
						key: g.key,
						name: g.name,
						list: g.ids.map((id) => s!.items[id]!).filter((e) => s!.shownOn(e, preview))
					}))
					.filter((g) => g.list.length)
			: []
	);
	const address = $derived(
		(preview === 'apps' ? links?.apps : links?.status)?.[0]?.url.replace(/^https:\/\/|\/$/g, '')
	);
	// A click in the preview finds the entry in the outline.
	function find(id: string) {
		sheet = false;
		flash = id;
		document.getElementById('o-' + id)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
		setTimeout(() => (flash = ''), 1200);
	}
	function iconChanged(a: Schemas['AppIcon']) {
		icons[a.app] = a;
		for (const e of Object.values(s?.items ?? {}))
			if (e.icon_app === a.app) {
				e.icon = a.icon;
				e.icon_light = a.icon_light;
			}
	}
	const domain = (url?: string) => url?.replace(/^https?:\/\/|\/$/g, '') ?? '';
	const groupKey = (ev: KeyboardEvent, key: string) => {
		if (!ev.altKey || (ev.key !== 'ArrowUp' && ev.key !== 'ArrowDown')) return;
		ev.preventDefault();
		s!.shiftGroup(key, ev.key === 'ArrowUp' ? -1 : 1);
		requestAnimationFrame(() => document.getElementById('ggrip-' + key)?.focus());
	};
	function startRename(key: string) {
		renaming = key;
	}
	function addGroup() {
		startRename(s!.addGroup());
	}
</script>

<svelte:head><title>Public pages · Hashi</title></svelte:head>

<PageHeader title="Public pages">
	{#snippet actions()}
		<Button variant="outline" size="sm" class="lg:hidden" onclick={() => (sheet = true)}
			><Eye />Preview</Button
		>
		<Button
			variant="ghost"
			size="icon"
			href="/settings?category=public"
			aria-label="Public pages settings"
			title="Settings"><Settings /></Button
		>
	{/snippet}
</PageHeader>

{#if failed}<p role="alert" class="text-md text-bad-text">{failed}</p>
{:else if !s}
	<div class="grid gap-4 lg:grid-cols-[26rem_minmax(0,1fr)]">
		<Skeleton class="h-96 rounded-2xl" /><Skeleton class="hidden h-96 rounded-2xl lg:block" />
	</div>
{:else}
	{#if s.error}<p role="alert" class="mb-3 text-md text-bad-text">{s.error}</p>{/if}
	<div class="grid items-start gap-4 lg:grid-cols-[26rem_minmax(0,1fr)]">
		<div
			role="list"
			aria-label="Layout"
			data-sort-zone="group"
			class="rounded-2xl border bg-card p-2 lg:sticky lg:top-4 lg:max-h-[calc(100dvh-7rem)] lg:overflow-y-auto"
		>
			{#each s.groups as g (g.key)}
				<section
					role="listitem"
					animate:slide
					use:sortable={{
						host: s,
						kind: 'group',
						id: g.key,
						handle: '[data-group-grip]',
						handleOnly: true
					}}
					data-sort-kind="group"
					data-sort-id={g.key}
					aria-label={g.name}
					class="mb-1 rounded-xl bg-card pb-1"
				>
					<header class="flex h-8 items-center gap-1 rounded-lg px-1">
						{#if g.key !== OTHER_KEY}<button
								type="button"
								id="ggrip-{g.key}"
								data-group-grip
								onkeydown={(ev) => groupKey(ev, g.key)}
								aria-label="Move group {g.name}: Alt and the arrow keys"
								class="grid size-6 shrink-0 cursor-grab touch-none place-items-center rounded-md text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
								><GripVertical class="size-4" /></button
							>{:else}<span class="w-6"></span>{/if}
						{#if renaming === g.key}
							<!-- svelte-ignore a11y_autofocus -->
							<input
								autofocus
								class="h-7 min-w-0 flex-1 rounded-md border bg-background px-2 text-md font-semibold"
								value={g.name}
								maxlength={60}
								aria-label="Group name"
								onfocus={(ev) => ev.currentTarget.select()}
								onblur={(ev) => {
									if (s!.rename(g.key, ev.currentTarget.value)) renaming = '';
									else ev.currentTarget.focus();
								}}
								onkeydown={(ev) => {
									if (ev.key === 'Enter') ev.currentTarget.blur();
									if (ev.key === 'Escape') renaming = '';
								}}
							/>
						{:else}
							<button
								type="button"
								class="min-w-0 flex-1 truncate text-left text-md font-semibold disabled:cursor-default"
								disabled={g.key === OTHER_KEY}
								title={g.key === OTHER_KEY ? undefined : 'Rename'}
								onclick={() => startRename(g.key)}>{g.name}</button
							>
						{/if}
						<span class="tnum px-1 text-xs text-muted-foreground">{g.ids.length}</span>
						<Button
							variant="ghost"
							size="icon"
							class="size-7"
							disabled={g.ids.length < 2}
							aria-label="Sort {g.name} A to Z"
							title="Sort A–Z"
							onclick={() => s!.sortAZ(g.key)}><ArrowDownAZ /></Button
						>
						{#if !g.ids.length && g.key !== OTHER_KEY}
							<Button
								variant="ghost"
								size="icon"
								class="size-7"
								aria-label="Delete {g.name}"
								title="Delete"
								onclick={() => s!.deleteGroup(g.key)}><Trash2 /></Button
							>
						{/if}
					</header>
					<div data-sort-zone="item" data-sort-group={g.key} class="min-h-7 space-y-1 pl-2">
						{#each g.ids as id (id)}
							{@const app = s.items[id]!.icon_app}
							<div
								animate:slide
								use:sortable={{
									host: s,
									kind: 'item',
									id,
									scope: 'outline',
									handle: '[data-grip]'
								}}
								data-sort-kind="item"
								data-sort-id={id}
								data-sort-group={g.key}
								class="rounded-lg"
							>
								<LayoutRow
									{s}
									{id}
									{preview}
									flash={flash === id}
									icon={app ? icons[app] : undefined}
									oniconchange={iconChanged}
								/>
							</div>
						{:else}
							<p class="rounded-lg border border-dashed px-2 py-1 text-sm text-muted-foreground">
								Empty
							</p>
						{/each}
					</div>
				</section>
			{/each}
			<Button variant="ghost" size="sm" onclick={addGroup}><Plus />New group</Button>
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
					{#each s.groups as g (g.key)}
						<DropdownMenu.Item onclick={() => s!.moveSelected(g.key)}>{g.name}</DropdownMenu.Item>
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
					<DropdownMenu.Item onclick={() => s!.setSelected('apps', true)}
						>Show on the apps page</DropdownMenu.Item
					>
					<DropdownMenu.Item onclick={() => s!.setSelected('apps', false)}
						>Hide from the apps page</DropdownMenu.Item
					>
					<DropdownMenu.Item onclick={() => s!.setSelected('status', true)}
						>Show on the status page</DropdownMenu.Item
					>
					<DropdownMenu.Item onclick={() => s!.setSelected('status', false)}
						>Hide from the status page</DropdownMenu.Item
					>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
			<Button size="sm" variant="ghost" onclick={() => (s!.selected = [])}><X />Clear</Button>
		</div>
	{:else if s.last}
		<div
			class="sticky bottom-4 z-10 mx-auto mt-4 flex w-fit items-center gap-3 rounded-2xl border bg-popover px-3 py-2 shadow-lg"
		>
			<span class="text-md">{s.last.text}. {s.saving ? 'Saving…' : 'Saved.'}</span>
			<Button size="sm" variant="outline" onclick={() => s!.undo()}>Undo</Button>
		</div>
	{/if}
	<p class="sr-only" aria-live="polite">{s.said}</p>
{/if}

{#snippet page()}
	{#if s}
		<div class="overflow-hidden rounded-2xl border bg-background">
			<div class="flex flex-wrap items-center gap-2 border-b bg-surface-2/60 px-3 py-1.5">
				<span class="min-w-0 flex-1 truncate text-xs text-muted-foreground"
					>{address ?? 'Not published yet'}</span
				>
				<ToggleGroup.Root
					type="single"
					value={preview}
					onValueChange={(v) => v && (preview = v as Page)}
					variant="outline"
					size="sm"
				>
					<ToggleGroup.Item value="apps">Apps page</ToggleGroup.Item>
					<ToggleGroup.Item value="status">Status page</ToggleGroup.Item>
				</ToggleGroup.Root>
			</div>
			<div class="p-5">
				{#if !shown.length}
					<p class="text-md text-muted-foreground">
						Nothing is on the {preview === 'apps' ? 'apps' : 'status'} page yet.
					</p>
				{/if}
				{#each shown as g (g.key)}
					<h3 class="mt-4 mb-2 text-md font-semibold text-muted-foreground first:mt-0">
						{g.name}
					</h3>
					{#if preview === 'apps'}
						<div
							data-sort-zone="item"
							data-sort-group={g.key}
							class="grid gap-2 sm:grid-cols-2 xl:grid-cols-3"
						>
							{#each g.list as e (e.key)}
								<div
									animate:slide
									use:sortable={{ host: s, kind: 'item', id: e.key, scope: 'preview' }}
									data-sort-kind="item"
									data-sort-id={e.key}
									data-sort-group={g.key}
									data-sort-axis="x"
									class="rounded-2xl"
								>
									<button
										type="button"
										data-sort-body
										onclick={() => find(e.key)}
										class="flex w-full cursor-grab items-center gap-3 rounded-2xl border bg-card p-3 text-left hover:border-muted-foreground/40"
									>
										<AppIcon
											name={e.name}
											icon={e.icon}
											light={e.icon_light}
											class="size-9 rounded-xl"
										/>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-md font-medium">{e.name}</span>
											<span class="flex items-center gap-2"
												><span class="min-w-0 flex-1 truncate text-xs text-muted-foreground"
													>{domain(e.url)}</span
												>{#if e.can_status && e.state}<PublicState
														state={e.state}
														class="shrink-0 text-xs"
													/>{/if}</span
											>
										</span>
									</button>
								</div>
							{/each}
						</div>
					{:else}
						<div
							data-sort-zone="item"
							data-sort-group={g.key}
							class="divide-y overflow-hidden rounded-2xl border bg-card"
						>
							{#each g.list as e (e.key)}
								<div
									animate:slide
									use:sortable={{ host: s, kind: 'item', id: e.key, scope: 'preview' }}
									data-sort-kind="item"
									data-sort-id={e.key}
									data-sort-group={g.key}
									class="bg-card"
								>
									<button
										type="button"
										data-sort-body
										onclick={() => find(e.key)}
										class="grid w-full cursor-grab grid-cols-[9rem_minmax(0,1fr)_4.5rem] items-center gap-3 px-3 py-2 text-left"
									>
										<span class="flex min-w-0 items-center gap-2 text-md font-medium"
											><AppIcon
												name={e.name}
												icon={e.icon}
												light={e.icon_light}
												class="size-5 rounded text-[10px]"
											/><span class="truncate">{e.name}</span></span
										>
										<DayStrip
											samples={(e.days ?? []).map((d) => d.state as Health)}
											detail={(i) => dayDetail(e.days![i]!, i === e.days!.length - 1)}
											labels={false}
											class="pointer-events-none"
										/>
										<PublicState state={e.state ?? 'unknown'} />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				{/each}
			</div>
		</div>
	{/if}
{/snippet}
