<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { Resource } from '$lib/api/types';
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

	let apps = $state<Resource[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		void (async () => {
			try {
				apps = await api.getPublicApps();
			} catch (e) {
				error = e instanceof ApiRequestError ? e.message : 'Failed to load dashboard apps';
			} finally {
				loading = false;
			}
		})();
	});
</script>

<AdminSectionPage
	title="App Display"
	description="Public dashboard tiles, grouping, and port 8081 presentation."
	icon={Monitor}
>
	<PanelSection title="Dashboard-enabled resources" description="Tiles served on /dashboard via /api/public/apps.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if apps.length === 0}
			<p class="text-sm text-muted-foreground">No dashboard tiles enabled. Enable on Resources page.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Display name</TableHead>
						<TableHead>Domain</TableHead>
						<TableHead>Target</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each apps as app}
						<TableRow>
							<TableCell>{app.name}</TableCell>
							<TableCell class="font-mono text-xs">{app.domain ?? '—'}</TableCell>
							<TableCell class="font-mono text-xs">
								{app.targetScheme}://{app.targetHost}:{app.targetPort}
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>
</AdminSectionPage>
