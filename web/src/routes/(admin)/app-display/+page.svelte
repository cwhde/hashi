<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { PublicDashboard, PublicDashboardItem } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { Monitor } from 'lucide-svelte';

	let dashboard = $state<PublicDashboard | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		void (async () => {
			try {
				dashboard = await api.getPublicApps();
			} catch (e) {
				error = e instanceof ApiRequestError ? e.message : 'Failed to load dashboard apps';
			} finally {
				loading = false;
			}
		})();
	});

	const apps = $derived((dashboard?.items ?? []) as PublicDashboardItem[]);
</script>

<AdminSectionPage
	title="App Display"
	description="Public dashboard tiles, grouping, and port 8081 presentation."
	icon={Monitor}
>
	<PanelSection title="Dashboard tiles" description="Safe tiles served on /dashboard via /api/public/apps.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading...</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if apps.length === 0}
			<p class="text-sm text-muted-foreground">No dashboard tiles enabled.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Display name</TableHead>
						<TableHead>Source</TableHead>
						<TableHead>Public URL</TableHead>
						<TableHead>Status</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each apps as app (app.id)}
						<TableRow>
							<TableCell>{app.displayName}</TableCell>
							<TableCell>{app.source}</TableCell>
							<TableCell class="font-mono text-xs">{app.publicUrl}</TableCell>
							<TableCell>{app.status}</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>
</AdminSectionPage>
