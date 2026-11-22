<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { session, routeFor } from '#lib/auth/session.svelte.ts';
	import { problemText } from '#lib/api/client.ts';

	// Entry point: setup until it is complete, then sign-in, then the admin app.
	// A public listener names its page in index.html (07 §7, §8): the page is at the root and nothing else runs.
	const publicPage = document.querySelector('meta[name="hashi-public"]')?.getAttribute('content');
	// Each page loads only its own code: the apps page needs none of the status page's, the admin neither.
	const pages = {
		status: () => import('#lib/components/hashi/public/PublicStatus.svelte'),
		apps: () => import('#lib/components/hashi/public/PublicApps.svelte')
	};
	let failed = $state('');
	onMount(async () => {
		if (publicPage) return;
		try {
			const s = await session.refresh();
			goto(routeFor(s) ?? '/overview', { replace: true });
		} catch (e) {
			failed = problemText(e);
		}
	});
</script>

{#if publicPage === 'status' || publicPage === 'apps'}
	{#await pages[publicPage]() then m}<m.default />{/await}
{:else if failed}
	<main class="grid min-h-dvh place-items-center px-4">
		<p class="max-w-sm rounded-xl border border-bad/30 bg-bad/10 p-3 text-md text-bad-text">
			{failed}
		</p>
	</main>
{/if}
