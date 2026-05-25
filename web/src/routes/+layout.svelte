<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import { navItems } from '$lib/nav';
	import { cn } from '$lib/utils';

	let { children } = $props();
</script>

<div class="flex min-h-screen bg-hashi-bg text-hashi-foreground">
	<nav
		class="flex w-14 shrink-0 flex-col items-center gap-1 border-r border-sidebar-border bg-hashi-bg-dark py-3"
		aria-label="Main navigation"
	>
		<a
			href="/"
			class="mb-3 flex size-9 items-center justify-center rounded-lg bg-hashi-hover text-sm font-bold text-white"
			title="Hashi"
		>
			H
		</a>

		{#each navItems as item (item.href)}
			{@const Icon = item.icon}
			{@const active =
				item.href === '/'
					? $page.url.pathname === '/'
					: $page.url.pathname.startsWith(item.href)}
			<a
				href={item.href}
				title={item.label}
				aria-label={item.label}
				class={cn(
					'flex size-10 items-center justify-center rounded-lg transition-colors',
					active
						? 'bg-sidebar-accent text-hashi-contrast'
						: 'text-hashi-foreground hover:bg-hashi-hover/20 hover:text-white'
				)}
			>
				<Icon class="size-5" aria-hidden="true" />
			</a>
		{/each}
	</nav>

	<div class="flex min-w-0 flex-1 flex-col">
		<header
			class="flex h-14 items-center border-b border-border bg-hashi-bg-dark/80 px-6 backdrop-blur-sm"
		>
			<h1 class="text-sm font-medium tracking-wide text-hashi-contrast">
				{navItems.find((item) =>
					item.href === '/'
						? $page.url.pathname === '/'
						: $page.url.pathname.startsWith(item.href)
				)?.label ?? 'Hashi'}
			</h1>
		</header>

		<main class="flex-1 overflow-auto p-6">
			{@render children()}
		</main>
	</div>
</div>
