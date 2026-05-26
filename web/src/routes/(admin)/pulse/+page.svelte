<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { CreatePulseAgentResult, PulseAgent } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
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
	let creating = $state(false);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let newAgentName = $state('');
	let createdToken = $state<CreatePulseAgentResult | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			agents = await api.listPulseAgents();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load Pulse agents';
		} finally {
			loading = false;
		}
	}

	async function createAgent() {
		if (!newAgentName.trim()) {
			error = 'Agent name is required.';
			return;
		}

		creating = true;
		error = null;
		message = null;
		createdToken = null;
		try {
			createdToken = await api.createPulseAgent({ name: newAgentName.trim() });
			newAgentName = '';
			message = 'Agent created. Copy the token now — it is shown only once.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create Pulse agent';
		} finally {
			creating = false;
		}
	}

	$effect(() => {
		void load();
	});
</script>

<AdminSectionPage
	title="Pulse"
	description="Dynamic endpoint agents, discovery tokens, and last-seen endpoints."
	icon={Zap}
>
	<PanelSection title="Register agent" description="Create a new Pulse agent and copy its one-time token.">
		<div class="grid max-w-md gap-3">
			<div class="grid gap-1">
				<Label for="pulse-name">Name</Label>
				<Input id="pulse-name" bind:value={newAgentName} placeholder="edge-node-1" />
			</div>
			<div>
				<Button onclick={() => createAgent()} disabled={creating || loading}>Create agent</Button>
			</div>
			{#if createdToken}
				<div class="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
					<p class="font-medium text-amber-200">One-time token for {createdToken.name}</p>
					<p class="mt-2 break-all font-mono text-xs">{createdToken.token}</p>
					<p class="mt-2 text-xs text-muted-foreground">Agent ID: {createdToken.id}</p>
				</div>
			{/if}
			{#if message}
				<p class="text-sm text-muted-foreground">{message}</p>
			{/if}
			{#if error}
				<p class="text-sm text-destructive">{error}</p>
			{/if}
		</div>
	</PanelSection>

	<PanelSection title="Registered agents" description="Last heartbeat and reported public IP per agent.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error && agents.length === 0}
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
					{#each agents as agent (agent.id)}
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
