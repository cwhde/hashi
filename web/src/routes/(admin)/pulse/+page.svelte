<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { CreatePulseAgentResult, PulseAgent, PulseInstall } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
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
	let dnsSettings = $state<import('$lib/api/types').InternalAgentDnsSettings | null>(null);
	let installSnippet = $state<PulseInstall | null>(null);
	let loading = $state(true);
	let creating = $state(false);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let newAgentName = $state('');
	let createdToken = $state<CreatePulseAgentResult | null>(null);
	let savingDnsAgentId = $state<string | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			const [agentItems, dns] = await Promise.all([
				api.listPulseAgents(),
				api.getInternalAgentDnsSettings().catch(() => null)
			]);
			agents = agentItems;
			dnsSettings = dns;
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
			createdToken = await api.createPulseAgent({
				name: newAgentName.trim(),
				installType: 'linux_service'
			});
			newAgentName = '';
			message = 'Agent created. Copy the token now — it is shown only once.';
			installSnippet = await api.getPulseInstall(createdToken.id, createdToken.token);
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

	async function revokeAgent(agentId: string) {
		if (!confirm('Revoke this Pulse agent token?')) return;
		try {
			await api.revokePulseAgent(agentId);
			message = 'Agent token revoked.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to revoke agent';
		}
	}

	async function rotateAgent(agentId: string) {
		if (!confirm('Rotate this agent token? The old token stops working immediately.')) return;
		try {
			const rotated = await api.rotatePulseAgentToken(agentId);
			if (rotated) {
				createdToken = rotated;
				message = 'Token rotated. Copy the new token now.';
				installSnippet = await api.getPulseInstall(rotated.id, rotated.token);
			}
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to rotate token';
		}
	}

	async function showInstall(agentId: string) {
		try {
			installSnippet = await api.getPulseInstall(agentId);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load install snippet';
		}
	}

	function dnsAgentSettings(agentId: string) {
		return dnsSettings?.agents.find((item) => item.pulseAgentId === agentId) ?? null;
	}

	async function saveAgentDns(agent: PulseAgent) {
		if (!dnsSettings) return;
		const settings = dnsAgentSettings(agent.id);
		if (!settings) return;
		savingDnsAgentId = agent.id;
		error = null;
		message = null;
		try {
			dnsSettings = await api.updateInternalAgentDnsSettings({
				enabled: dnsSettings.enabled,
				domain: dnsSettings.domain,
				keepLastRewriteWhenAgentStale: dnsSettings.keepLastRewriteWhenAgentStale,
				adGuardConnectionId: dnsSettings.adGuardConnectionId,
				agents: [
					{
						pulseAgentId: agent.id,
						enabled: settings.enabled,
						nameOverride: settings.nameOverride || null,
						ipMode: settings.ipMode,
						keepLastRewriteWhenStale: settings.keepLastRewriteWhenStale
					}
				]
			});
			message = `DNS settings saved for ${agent.name}.`;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save agent DNS settings';
		} finally {
			savingDnsAgentId = null;
		}
	}
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
			{#if installSnippet}
				<div class="space-y-3 rounded-md border border-border p-3 text-xs">
					<div>
						<p class="mb-1 font-medium text-white">Linux install</p>
						<pre class="overflow-auto whitespace-pre-wrap font-mono text-muted-foreground">{installSnippet.linuxInstallScript}</pre>
					</div>
					<div>
						<p class="mb-1 font-medium text-white">Docker Compose</p>
						<pre class="overflow-auto whitespace-pre-wrap font-mono text-muted-foreground">{installSnippet.dockerComposeSnippet}</pre>
					</div>
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
						<TableHead>Hostname</TableHead>
						<TableHead>Version</TableHead>
						<TableHead>Last seen</TableHead>
						<TableHead>Public IP</TableHead>
						<TableHead>DNS</TableHead>
						<TableHead class="w-40"></TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each agents as agent (agent.id)}
						<TableRow>
							<TableCell>{agent.name}</TableCell>
							<TableCell>{agent.status}</TableCell>
							<TableCell class="text-xs">{agent.lastHostname ?? '—'}</TableCell>
							<TableCell class="text-xs">{agent.lastAgentVersion ?? '—'}</TableCell>
							<TableCell class="text-xs">
								{agent.lastSeenAtUtc ? new Date(agent.lastSeenAtUtc).toLocaleString() : '—'}
							</TableCell>
							<TableCell class="font-mono text-xs">{agent.lastPublicIp ?? '—'}</TableCell>
							<TableCell class="text-xs">
								{@const dns = dnsAgentSettings(agent.id)}
								{#if dns}
									<div class="grid min-w-64 gap-2">
										<div class="flex items-center justify-between gap-3">
											<span>{agent.dnsPendingAtUtc ? 'Pending' : dns.enabled ? 'Enabled' : 'Disabled'}</span>
											<Switch bind:checked={dns.enabled} />
										</div>
										<div class="grid grid-cols-[1fr_auto] gap-2">
											<Input
												bind:value={dns.nameOverride}
												placeholder={agent.name.toLowerCase().replaceAll(' ', '-')}
											/>
											<select
												class="h-9 rounded-md border border-border bg-background px-2 text-xs text-white"
												bind:value={dns.ipMode}
											>
												<option value="selected">Selected</option>
												<option value="private_selected">Private</option>
												<option value="public">Public</option>
											</select>
										</div>
										<div class="flex items-center justify-between gap-3">
											<span>Keep stale</span>
											<Switch bind:checked={dns.keepLastRewriteWhenStale} />
										</div>
										<Button
											variant="outline"
											size="sm"
											onclick={() => saveAgentDns(agent)}
											disabled={savingDnsAgentId === agent.id}
										>
											{savingDnsAgentId === agent.id ? 'Saving...' : 'Save DNS'}
										</Button>
									</div>
								{:else}
									{agent.dnsPendingAtUtc ? 'Pending' : '—'}
								{/if}
							</TableCell>
							<TableCell class="space-x-2">
								<Button variant="ghost" size="sm" onclick={() => showInstall(agent.id)}>Install</Button>
								<Button variant="ghost" size="sm" onclick={() => rotateAgent(agent.id)}>Rotate</Button>
								<Button variant="ghost" size="sm" onclick={() => revokeAgent(agent.id)}>Revoke</Button>
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>
</AdminSectionPage>
