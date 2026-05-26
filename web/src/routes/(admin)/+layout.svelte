<script lang="ts">
	import AdminAppShell from '$lib/components/layout/AdminAppShell.svelte';

	let { children } = $props();
</script>

<AdminAppShell>
	{@render children()}
</AdminAppShell>
