<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	// Until S1 brings auth and real pages, the root forwards to the design index (S0b).
	onMount(() => goto('/design', { replaceState: true }));
</script>

<svelte:head><title>Hashi</title></svelte:head>
