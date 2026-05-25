<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { navItems } from '$lib/nav';
	import NavRail from '$lib/components/layout/NavRail.svelte';
	import { api } from '$lib/api/client';

	let { children } = $props();

	let pinned = $state(false);
	let expanded = $state(false);
	let checkingSetup = $state(true);

	const currentLabel = $derived(
		navItems.find((item) =>
			item.href === '/'
				? page.url.pathname === '/'
				: page.url.pathname.startsWith(item.href)
		)?.label ?? 'Hashi'
	);

	onMount(async () => {
		try {
			const status = await api.getSetupStatus();
			if (!status.isComplete && !page.url.pathname.startsWith('/setup')) {
				await goto('/setup');
				return;
			}
		} catch {
			// allow offline dev without API
		} finally {
			checkingSetup = false;
		}
	});
</script>

{#if checkingSetup}
	<div class="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
		Loading…
	</div>
{:else}
	<div class="flex min-h-screen bg-hashi-bg text-hashi-foreground">
		<NavRail bind:pinned bind:expanded />

		<div class="flex min-w-0 flex-1 flex-col">
			<header
				class="flex h-14 items-center border-b border-border bg-hashi-bg-dark/80 px-6 backdrop-blur-sm"
			>
				<h1 class="text-sm font-medium tracking-wide text-hashi-contrast">{currentLabel}</h1>
			</header>

			<main class="flex-1 overflow-auto p-6">
				{@render children()}
			</main>
		</div>
	</div>
{/if}
