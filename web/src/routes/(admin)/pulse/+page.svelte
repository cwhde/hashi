<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { PulseAgent } from '$lib/api/types';
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
	import { Zap } from 'lucide-svelte';

	let agents = $state<PulseAgent[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		void (async () => {
			try {
				agents = await api.listPulseAgents();
			} catch (e) {
				error = e instanceof ApiRequestError ? e.message : 'Failed to load Pulse agents';
			} finally {
				loading = false;
			}
		})();
	});
</script>

<AdminSectionPage
	title="Pulse"
	description="Dynamic endpoint agents, discovery tokens, and last-seen endpoints."
	icon={Zap}
>
	<PanelSection title="Registered agents" description="Agent registration CRUD pending in OpenAPI.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if agents.length === 0}
			<p class="text-sm text-muted-foreground">No Pulse agents registered.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Status</TableHead>
						<TableHead>Last seen</TableHead>
						<TableHead>Public IP</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each agents as agent}
						<TableRow>
							<TableCell>{agent.name}</TableCell>
							<TableCell>{agent.status}</TableCell>
							<TableCell class="text-xs">
								{agent.lastSeenAtUtc ? new Date(agent.lastSeenAtUtc).toLocaleString() : '—'}
							</TableCell>
							<TableCell class="font-mono text-xs">{agent.lastPublicIp ?? '—'}</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>
</AdminSectionPage>
