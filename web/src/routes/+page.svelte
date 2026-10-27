<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { session, routeFor } from '$lib/auth/session.svelte';
	import { problemText } from '$lib/api/client';

	// Entry point: setup until it is complete, then sign-in, then the admin app.
	let failed = $state('');
	onMount(async () => {
		try {
			const s = await session.refresh();
			goto(routeFor(s) ?? '/activity', { replaceState: true });
		} catch (e) {
			failed = problemText(e);
		}
	});
</script>

<svelte:head><title>Hashi</title></svelte:head>
{#if failed}
	<main class="grid min-h-dvh place-items-center px-4">
		<p class="max-w-sm rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text">
			{failed}
		</p>
	</main>
{/if}
