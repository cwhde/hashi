<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import AppShell from '$lib/components/hashi/AppShell.svelte';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { api, problemText } from '$lib/api/client';
	import { session, routeFor } from '$lib/auth/session.svelte';
	import { theme, type ThemeId } from '$lib/theme.svelte';

	let { children } = $props();
	let ready = $state(false);
	let failed = $state('');

	onMount(async () => {
		try {
			const s = await session.refresh();
			const away = routeFor(s, page.url.pathname);
			if (away) {
				const next = away === '/login' ? `?next=${encodeURIComponent(page.url.pathname)}` : '';
				goto(away + next, { replaceState: true });
				return;
			}
			ready = true;
			// The saved theme follows the admin to every browser (Settings → Appearance).
			const a = await api.GET('/api/settings/appearance');
			if (a.data) theme.set(a.data.theme as ThemeId);
		} catch (e) {
			failed = problemText(e);
		}
	});

	async function signOut() {
		await api.POST('/api/auth/logout');
		goto('/login', { replaceState: true });
	}
</script>

{#if ready}
	<AppShell live={{ onSignOut: signOut }}>{@render children()}</AppShell>
{:else if failed}
	<main class="grid min-h-dvh place-items-center px-4">
		<p class="max-w-sm rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text">
			{failed}
		</p>
	</main>
{:else}
	<div class="flex min-h-dvh">
		<Skeleton class="hidden w-16 rounded-none md:block" />
		<div class="flex-1 space-y-4 p-8"><Skeleton class="h-8 w-48" /><Skeleton class="h-64" /></div>
	</div>
{/if}
