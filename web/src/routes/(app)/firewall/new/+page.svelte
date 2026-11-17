<script lang="ts">
	// Firewall hosts are added on Connections with every other outside system (bug 24); old links land there.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	onMount(() => void goto('/connections?add=firewall', { replace: true }));
</script>
