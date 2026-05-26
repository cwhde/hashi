<script lang="ts">
	import AdminAppShell from '$lib/components/layout/AdminAppShell.svelte';
	import AdminOverviewView from '$lib/components/admin/AdminOverviewView.svelte';
	import PublicDashboardView from '$lib/components/public/PublicDashboardView.svelte';
	import PublicShell from '$lib/components/public/PublicShell.svelte';
	import PublicStatusView from '$lib/components/public/PublicStatusView.svelte';
	import { resolveRootPortMode } from '$lib/public/port-mode';

	let mode = $derived(resolveRootPortMode());
</script>

{#if mode === 'public-dashboard'}
	<PublicShell>
		<PublicDashboardView />
	</PublicShell>
{:else if mode === 'public-status'}
	<PublicShell>
		<PublicStatusView />
	</PublicShell>
{:else}
	<AdminAppShell>
		<AdminOverviewView />
	</AdminAppShell>
{/if}
