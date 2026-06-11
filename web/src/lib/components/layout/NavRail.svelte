<script lang="ts">
	import { resolve } from '$app/paths';
	import { Pin, PinOff } from 'lucide-svelte';
	import { page } from '$app/state';
	import { navItems } from '$lib/nav';
	import { cn } from '$lib/utils';
	import { Button } from '$lib/components/ui/button';

	let {
		pinned = $bindable(false),
		expanded = $bindable(false)
	}: {
		pinned?: boolean;
		expanded?: boolean;
	} = $props();

	function togglePin() {
		pinned = !pinned;
		if (pinned) expanded = true;
	}
</script>

<nav
	class={cn(
		'group/rail flex shrink-0 flex-col border-r border-sidebar-border bg-hashi-bg-dark py-3 transition-[width] duration-200',
		expanded || pinned ? 'w-52' : 'w-14'
	)}
	aria-label="Main navigation"
	onmouseenter={() => {
		if (!pinned) expanded = true;
	}}
	onmouseleave={() => {
		if (!pinned) expanded = false;
	}}
>
	<div class="mb-2 flex items-center gap-2 px-3">
		<a
			href={resolve('/')}
			class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-hashi-hover text-sm font-bold text-white overflow-hidden"
			title="Hashi"
		>
			<img src="https://static.juzo.io/assets/logo.png" class="size-6 object-contain" alt="Hashi Logo" />
		</a>
		{#if expanded || pinned}
			<span class="truncate text-sm font-semibold text-white">Hashi</span>
		{/if}
	</div>

	<ul class="flex-1 space-y-0.5 px-2">
		{#each navItems as item (item.href)}
			{@const Icon = item.icon}
			{@const active =
				item.href === '/'
					? page.url.pathname === '/'
					: page.url.pathname.startsWith(item.href)}
			<li>
				<a
					href={resolve(item.href as '/')}
					class={cn(
						'flex items-center gap-2 rounded-md px-2 py-2 text-xs transition-colors',
						active
							? 'bg-hashi-hover/30 text-white'
							: 'text-muted-foreground hover:bg-hashi-hover/15 hover:text-white'
					)}
					title={item.label}
				>
					<Icon class="size-4 shrink-0" aria-hidden="true" />
					{#if expanded || pinned}
						<span class="truncate">{item.label}</span>
					{/if}
				</a>
			</li>
		{/each}
	</ul>

	<div class="mt-auto px-2">
		<Button variant="ghost" size="icon-sm" onclick={togglePin} title={pinned ? 'Unpin nav' : 'Pin nav'}>
			{#if pinned}
				<PinOff class="size-4" />
			{:else}
				<Pin class="size-4" />
			{/if}
		</Button>
	</div>
</nav>
