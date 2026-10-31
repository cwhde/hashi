<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { session, routeFor } from '$lib/auth/session.svelte';
	import { problemText } from '$lib/api/client';
	import PublicStatus from '$lib/components/hashi/public/PublicStatus.svelte';
	import PublicApps from '$lib/components/hashi/public/PublicApps.svelte';

	// Entry point: setup until it is complete, then sign-in, then the admin app.
	// A public listener names its page in index.html (07 §7, §8): the page is at the root and nothing else runs.
	const publicPage = document.querySelector('meta[name="hashi-public"]')?.getAttribute('content');
	let failed = $state('');
	onMount(async () => {
		if (publicPage) return;
		try {
			const s = await session.refresh();
			goto(routeFor(s) ?? '/activity', { replaceState: true });
		} catch (e) {
			failed = problemText(e);
		}
	});
</script>

{#if publicPage === 'status'}
	<PublicStatus />
{:else if publicPage === 'apps'}
	<PublicApps />
{:else if failed}
	<main class="grid min-h-dvh place-items-center px-4">
		<p class="max-w-sm rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text">
			{failed}
		</p>
	</main>
{/if}
