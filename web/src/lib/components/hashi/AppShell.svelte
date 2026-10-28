<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { nav as designNav, liveNav } from '$lib/design/nav';
	import { session } from '$lib/auth/session.svelte';
	import { time } from '$lib/format';
	import { theme, themes } from '$lib/theme.svelte';
	import { cn } from '$lib/utils';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Command from '$lib/components/ui/command';
	import { Button } from '$lib/components/ui/button';
	import {
		Pin,
		PinOff,
		Search,
		GitPullRequestArrow,
		LockOpen,
		Menu,
		Palette,
		LogOut,
		KeyRound
	} from '@lucide/svelte';

	// live: the real admin app (session from the server, only built pages). Without it the shell shows mock data (/design).
	let { children, live }: { children: Snippet; live?: { onSignOut: () => void } } = $props();
	const nav = $derived(live ? liveNav : designNav);
	const s = $derived(session.current);

	// Rail is 64px; hovering expands it over the content, pinning reserves 240px (10-ui.md §3).
	let pinned = $state(readPinned());
	let hovering = $state(false);
	let mobileOpen = $state(false);
	let searchOpen = $state(false);
	const expanded = $derived(pinned || hovering);

	function readPinned() {
		try {
			return (
				typeof localStorage !== 'undefined' && localStorage.getItem('hashi.nav.pinned') === '1'
			);
		} catch {
			return false;
		}
	}
	function togglePin() {
		pinned = !pinned;
		try {
			localStorage.setItem('hashi.nav.pinned', pinned ? '1' : '0');
		} catch {
			// storage blocked: the choice lasts for this page view
		}
	}

	// Pages reached from the top bar rather than the navigation.
	const extraTitles: Record<string, string> = {
		'/design/changes': 'Pending changes',
		'/design/states': 'States'
	};
	const current = $derived(
		nav
			.flatMap((g) => g.items)
			.find((i) => page.url.pathname === i.href || page.url.pathname.startsWith(i.href + '/'))
	);

	function onKey(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			searchOpen = !searchOpen;
		}
	}
</script>

<svelte:window onkeydown={onKey} />

{#snippet navList(full: boolean)}
	<nav aria-label="Main" class="flex flex-col gap-4 px-2.5 py-3">
		{#each nav as section, si (si)}
			<div class="flex flex-col gap-0.5">
				{#if section.group}
					<div
						class={cn(
							'h-6 px-2.5 text-[11.5px] font-medium text-muted-foreground',
							!full && 'sr-only'
						)}
					>
						{section.group}
					</div>
				{/if}
				{#each section.items as item (item.href)}
					{@const active = current?.href === item.href}
					<a
						href={item.href}
						onclick={() => (mobileOpen = false)}
						aria-current={active ? 'page' : undefined}
						title={full ? undefined : item.label}
						class={cn(
							'flex h-9 items-center gap-3 rounded-[10px] px-2.5 text-[13.5px] transition-colors',
							active
								? 'bg-surface-2 font-medium text-foreground'
								: 'text-muted-foreground hover:bg-surface-2/60 hover:text-foreground'
						)}
					>
						<item.icon
							class={cn('size-[18px] shrink-0', active && 'text-primary')}
							strokeWidth={1.75}
						/>
						<span class={cn('truncate', !full && 'sr-only')}>{item.label}</span>
					</a>
				{/each}
			</div>
		{/each}
	</nav>
{/snippet}

{#snippet brand(full: boolean)}
	<a
		href={live ? '/' : '/design'}
		class="flex h-[52px] shrink-0 items-center gap-2.5 px-[18px]"
		aria-label={live ? 'Hashi home' : 'Hashi design index'}
	>
		<span
			class="grid size-7 shrink-0 place-items-center rounded-lg bg-primary text-[15px] font-semibold text-primary-foreground"
			>橋</span
		>
		{#if full}<span class="text-[15px] font-semibold tracking-[-0.01em]">Hashi</span>{/if}
	</a>
{/snippet}

<div class="min-h-dvh bg-background">
	<!-- Desktop rail -->
	<aside
		class={cn(
			'fixed inset-y-0 left-0 z-40 hidden flex-col border-r bg-sidebar transition-[width] duration-150 md:flex',
			expanded ? 'w-60' : 'w-16',
			hovering && !pinned && 'shadow-xl shadow-black/20'
		)}
		onmouseenter={() => (hovering = true)}
		onmouseleave={() => (hovering = false)}
		onfocusin={() => (hovering = true)}
		onfocusout={() => (hovering = false)}
	>
		{@render brand(expanded)}
		<div class="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">{@render navList(expanded)}</div>
		<div class="border-t p-2.5">
			<button
				onclick={togglePin}
				class="flex h-9 w-full items-center gap-3 rounded-[10px] px-2.5 text-[13px] text-muted-foreground hover:bg-surface-2/60 hover:text-foreground"
				aria-pressed={pinned}
			>
				{#if pinned}<PinOff class="size-[18px] shrink-0" strokeWidth={1.75} />{:else}<Pin
						class="size-[18px] shrink-0"
						strokeWidth={1.75}
					/>{/if}
				<span class={cn(!expanded && 'sr-only')}
					>{pinned ? 'Unpin navigation' : 'Keep navigation open'}</span
				>
			</button>
		</div>
	</aside>

	<div
		class={cn(
			'flex min-h-dvh flex-col transition-[padding] duration-150',
			pinned ? 'md:pl-60' : 'md:pl-16'
		)}
	>
		<header
			class="sticky top-0 z-30 flex h-[52px] items-center gap-2 border-b bg-background/90 px-3 backdrop-blur-sm md:px-5"
		>
			<Sheet.Root bind:open={mobileOpen}>
				<Sheet.Trigger>
					{#snippet child({ props })}
						<Button
							{...props}
							variant="ghost"
							size="icon"
							class="md:hidden"
							aria-label="Open navigation"><Menu /></Button
						>
					{/snippet}
				</Sheet.Trigger>
				<Sheet.Content side="left" class="w-72 bg-sidebar p-0">
					<Sheet.Title class="sr-only">Navigation</Sheet.Title>
					{@render brand(true)}
					{@render navList(true)}
				</Sheet.Content>
			</Sheet.Root>

			<div class="min-w-0 flex-1 truncate text-[13.5px] font-medium">
				{current?.label ?? extraTitles[page.url.pathname] ?? 'Hashi'}
			</div>

			<button
				onclick={() => (searchOpen = true)}
				class="hidden h-8 w-64 items-center gap-2 rounded-[10px] border bg-surface px-2.5 text-[13px] text-muted-foreground hover:text-foreground lg:flex"
			>
				<Search class="size-4" />
				<span class="flex-1 text-left">Search IPs, domains, pages</span>
				<kbd class="text-[11px]">⌘K</kbd>
			</button>
			<Button
				variant="ghost"
				size="icon"
				class="lg:hidden"
				aria-label="Search"
				onclick={() => (searchOpen = true)}><Search /></Button
			>

			{#if !live}<a
					href="/design/changes"
					class="flex h-8 items-center gap-1.5 rounded-[10px] border border-warn/30 bg-warn/10 px-2.5 text-[13px] font-medium text-warn-text hover:bg-warn/15"
				>
					<GitPullRequestArrow class="size-4" />
					<span class="tnum">2</span><span class="hidden sm:inline">pending changes</span>
				</a>{/if}

			<span
				class="hidden h-8 items-center gap-1.5 rounded-[10px] px-2 text-[12.5px] text-muted-foreground xl:flex"
				title="Vault unlocked · service mode"
			>
				<LockOpen class="size-4 text-ok-text" />Vault unlocked
			</span>

			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button {...props} variant="ghost" size="icon" aria-label="Theme"><Palette /></Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="w-48">
					<DropdownMenu.Label>Theme</DropdownMenu.Label>
					<DropdownMenu.RadioGroup
						value={theme.current}
						onValueChange={(v) => theme.set(v as typeof theme.current)}
					>
						{#each themes as t (t.id)}
							<DropdownMenu.RadioItem value={t.id}>
								<span
									data-theme={t.id}
									class="flex h-4 w-6 items-center justify-end rounded-sm border bg-card px-0.5"
								>
									<span class="size-2 rounded-full bg-primary"></span>
								</span>
								{t.name}
							</DropdownMenu.RadioItem>
						{/each}
					</DropdownMenu.RadioGroup>
				</DropdownMenu.Content>
			</DropdownMenu.Root>

			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<button
							{...props}
							class="grid size-8 place-items-center rounded-full border bg-surface-2 text-[12px] font-semibold"
							aria-label="Account">A</button
						>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="w-56">
					<DropdownMenu.Label class="font-normal">
						<div class="text-[13px] font-medium">Admin</div>
						<div class="text-[12px] text-muted-foreground">
							{#if live}Session ends {time(s?.absolute_expires_at)} · bound to {s?.bound_ip}{:else}Session
								ends 19:10 · bound to 192.0.2.10{/if}
						</div>
					</DropdownMenu.Label>
					<DropdownMenu.Separator />
					<DropdownMenu.Item onSelect={() => live && goto('/settings?category=account')}
						><KeyRound />Passkeys and sessions</DropdownMenu.Item
					>
					<DropdownMenu.Item onSelect={() => live?.onSignOut()}
						><LogOut />Sign out</DropdownMenu.Item
					>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</header>

		<main class="mx-auto w-full max-w-[1480px] flex-1 px-4 py-6 md:px-8">
			{@render children()}
		</main>
	</div>
</div>

<Command.Dialog bind:open={searchOpen}>
	<Command.Input placeholder="IP, CIDR, ASN, domain or page…" />
	<Command.List>
		<Command.Empty>No match. Try an IP like 192.0.2.201 or a domain.</Command.Empty>
		{#if !live}<Command.Group heading="Addresses">
				<Command.Item onSelect={() => (searchOpen = false)}>192.0.2.201 · banned · NL</Command.Item>
				<Command.Item onSelect={() => (searchOpen = false)}
					>203.0.113.9 · challenged · DE</Command.Item
				>
			</Command.Group>
			<Command.Group heading="Services">
				<Command.Item onSelect={() => (searchOpen = false)}>git.example.com</Command.Item>
				<Command.Item onSelect={() => (searchOpen = false)}>media.example.com</Command.Item>
			</Command.Group>{/if}
		<Command.Group heading="Pages">
			{#each nav.flatMap((g) => g.items) as item (item.href)}
				<Command.Item
					onSelect={() => {
						searchOpen = false;
						goto(item.href);
					}}>{item.label}</Command.Item
				>
			{/each}
		</Command.Group>
	</Command.List>
</Command.Dialog>
