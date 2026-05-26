<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { Script } from '$lib/api/types';
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
	import { FileCode } from 'lucide-svelte';

	let scripts = $state<Script[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		void (async () => {
			try {
				scripts = await api.listScripts();
			} catch (e) {
				error = e instanceof ApiRequestError ? e.message : 'Failed to load scripts';
			} finally {
				loading = false;
			}
		})();
	});
</script>

<AdminSectionPage
	title="Scripts"
	description="Privileged shell scripts, cron schedules, and manual run output."
	icon={FileCode}
>
	<PanelSection title="Script inventory" description="Create/edit endpoints pending in OpenAPI.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if scripts.length === 0}
			<p class="text-sm text-muted-foreground">No scripts configured.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Description</TableHead>
						<TableHead>Enabled</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each scripts as script}
						<TableRow>
							<TableCell>{script.name}</TableCell>
							<TableCell class="max-w-md truncate text-xs">{script.description}</TableCell>
							<TableCell>{script.enabled ? 'yes' : 'no'}</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>
</AdminSectionPage>
