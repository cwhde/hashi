<script lang="ts">
	import AppShell from '$lib/components/hashi/AppShell.svelte';
	let { children } = $props();
</script>

<AppShell>{@render children()}</AppShell>
