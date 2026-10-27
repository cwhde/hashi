<script lang="ts">
	import { api } from '$lib/api/client';

	// S0 shell only; the real pages are designed in S0b (docs/spec/10-ui.md).
	const version = api.GET('/api/version').then(({ data }) => data);
</script>

<svelte:head><title>Hashi</title></svelte:head>

<main class="mx-auto max-w-xl p-8 font-sans">
	<h1 class="text-2xl font-semibold">Hashi</h1>
	<p class="mt-2 text-sm">
		{#await version}
			Connecting…
		{:then v}
			{#if v}Running {v.version} ({v.commit}){:else}API unavailable{/if}
		{:catch}
			API unavailable
		{/await}
	</p>
</main>
