<script lang="ts">
	import { page } from '$app/stores';
	import { api, ApiRequestError } from '$lib/api/client';
	import type { AdGuardConnection, AdGuardRewrite, PulseAgent } from '$lib/api/types';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
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
	import { Radio } from 'lucide-svelte';

	let connections = $state<AdGuardConnection[]>([]);
	let pulseAgents = $state<PulseAgent[]>([]);
	let rewrites = $state<AdGuardRewrite[]>([]);
	let selectedConnectionId = $state<string | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let syncing = $state(false);
	let testing = $state(false);
	let deletingId = $state<string | null>(null);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let connectionForm = $state({
		name: 'home-adguard',
		baseUrl: 'http://127.0.0.1:3000',
		password: '',
		targetMode: 'static_host',
		staticHost: '127.0.0.1',
		staticIp: '',
		pulseAgentId: '',
		pulseIpMode: 'selected',
		privateCandidateSelector: 'selected',
		scheme: 'http',
		port: 3000,
		pathPrefix: '',
		tlsValidationMode: 'system',
		expectedTlsHostname: ''
	});
	let rewriteForm = $state({
		domain: '',
		answer: ''
	});

	$effect(() => {
		const queryId = $page.url.searchParams.get('connection');
		if (queryId && queryId !== selectedConnectionId) {
			selectedConnectionId = queryId;
		}
	});

	$effect(() => {
		void loadConnections();
	});

	$effect(() => {
		if (selectedConnectionId) {
			void loadRewrites(selectedConnectionId);
		} else {
			rewrites = [];
		}
	});

	async function loadConnections() {
		loading = true;
		error = null;
		try {
			const [connectionItems, agentItems] = await Promise.all([
				api.listAdGuardConnections(),
				api.listPulseAgents().catch(() => [])
			]);
			connections = connectionItems;
			pulseAgents = agentItems;
			if (!selectedConnectionId && connections.length > 0) {
				selectedConnectionId = connections[0]?.id ?? null;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load AdGuard connections';
		} finally {
			loading = false;
		}
	}

	async function loadRewrites(connectionId: string) {
		try {
			rewrites = await api.listAdGuardRewrites(connectionId);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load rewrites';
		}
	}

	async function withReauth<T>(action: () => Promise<T>): Promise<T | null> {
		try {
			return await action();
		} catch (e) {
			if (e instanceof ApiRequestError && e.code === 'reauth_required') {
				const ok = await performPasskeyReauthentication();
				if (ok) {
					return await action();
				}
				error = 'Passkey reauthentication failed.';
				return null;
			}
			throw e;
		}
	}

	async function createConnection() {
		if (!connectionForm.name || !connectionForm.password) {
			error = 'Name and password are required.';
			return;
		}
		if (connectionForm.targetMode === 'static_host' && !connectionForm.staticHost) {
			error = 'Static host is required.';
			return;
		}
		if (connectionForm.targetMode === 'static_ip' && !connectionForm.staticIp) {
			error = 'Static IP is required.';
			return;
		}
		if (connectionForm.targetMode === 'pulse_agent' && !connectionForm.pulseAgentId) {
			error = 'Select a Pulse agent.';
			return;
		}
		if (connectionForm.targetMode === 'pulse_agent') {
			const agent = pulseAgents.find((item) => item.id === connectionForm.pulseAgentId);
			if (
				agent &&
				(agent.status !== 'online' || !agent.lastSeenAtUtc) &&
				!confirm('This Pulse agent is not currently online. Save the AdGuard target anyway?')
			) {
				return;
			}
		}
		if (!connectionForm.baseUrl && connectionForm.targetMode !== 'pulse_agent') {
			error = 'Compatibility base URL is required for static targets.';
			return;
		}
		saving = true;
		error = null;
		message = null;
		try {
			const created = await withReauth(() =>
				api.createAdGuardConnection({
					name: connectionForm.name,
					baseUrl:
						connectionForm.targetMode === 'pulse_agent' ? null : connectionForm.baseUrl || null,
					password: connectionForm.password,
					target: {
						targetMode: connectionForm.targetMode,
						staticHost: connectionForm.targetMode === 'static_host' ? connectionForm.staticHost : null,
						staticIp: connectionForm.targetMode === 'static_ip' ? connectionForm.staticIp : null,
						pulseAgentId:
							connectionForm.targetMode === 'pulse_agent' ? connectionForm.pulseAgentId : null,
						pulseIpMode: connectionForm.pulseIpMode,
						privateCandidateSelector: connectionForm.privateCandidateSelector,
						port: connectionForm.port,
						scheme: connectionForm.scheme,
						pathPrefix: connectionForm.pathPrefix || null,
						tlsValidationMode: connectionForm.tlsValidationMode,
						expectedTlsHostname: connectionForm.expectedTlsHostname || null
					}
				})
			);
			if (created) {
				message = `AdGuard connection "${created.name}" created.`;
				connectionForm.password = '';
				selectedConnectionId = created.id;
				await loadConnections();
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create AdGuard connection';
		} finally {
			saving = false;
		}
	}

	async function upsertRewrite() {
		if (!selectedConnectionId || !rewriteForm.domain || !rewriteForm.answer) {
			error = 'Select a connection and enter domain plus answer.';
			return;
		}
		saving = true;
		error = null;
		message = null;
		try {
			const result = await withReauth(() =>
				api.upsertAdGuardRewrite(selectedConnectionId!, {
					domain: rewriteForm.domain,
					answer: rewriteForm.answer
				})
			);
			if (result) {
				const planned = result.plan.changes.length;
				message = `Rewrite planned for ${result.rewrite?.domain ?? rewriteForm.domain}. ${planned} pending change${planned === 1 ? '' : 's'} will apply on push sync.`;
				rewriteForm.domain = '';
				rewriteForm.answer = '';
				await loadRewrites(selectedConnectionId);
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save rewrite';
		} finally {
			saving = false;
		}
	}

	async function syncConnection() {
		if (!selectedConnectionId) return;
		syncing = true;
		error = null;
		message = null;
		try {
			await withReauth(() => api.syncAdGuardConnection(selectedConnectionId!));
			message = 'Managed rewrites pushed to AdGuard Home.';
			await loadRewrites(selectedConnectionId);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Sync failed';
		} finally {
			syncing = false;
		}
	}

	async function testConnection() {
		if (!selectedConnectionId) return;
		testing = true;
		error = null;
		message = null;
		try {
			const result = await withReauth(() => api.testAdGuardConnection(selectedConnectionId!));
			message = result?.connected
				? `Connection test succeeded at ${result.resolvedBaseUrl ?? 'resolved target'}.`
				: `Connection failed: ${result?.error ?? result?.target?.lastError ?? 'unknown error'}`;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Connection test failed';
		} finally {
			testing = false;
		}
	}

	async function deleteRewrite(rewriteId: string) {
		if (!selectedConnectionId || !confirm('Delete this managed rewrite?')) return;
		deletingId = rewriteId;
		error = null;
		message = null;
		try {
			const result = await withReauth(() => api.deleteAdGuardRewrite(selectedConnectionId!, rewriteId));
			const planned = result?.plan.changes.length ?? 0;
			message = `Rewrite delete planned. ${planned} pending change${planned === 1 ? '' : 's'} will apply on push sync.`;
			await loadRewrites(selectedConnectionId);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to delete rewrite';
		} finally {
			deletingId = null;
		}
	}

	function selectedConnection() {
		return connections.find((connection) => connection.id === selectedConnectionId) ?? null;
	}

	function selectedPulseAgent() {
		return pulseAgents.find((agent) => agent.id === connectionForm.pulseAgentId) ?? null;
	}
</script>

<AdminSectionPage
	title="AdGuard Home"
	description="Optional DNS rewrites managed by Hashi. Linked from Connections — not a primary nav section."
	icon={Radio}
>
	<PanelSection title="Connection" description="Register an AdGuard Home control API endpoint.">
		<div class="grid max-w-3xl gap-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="ag-name">Name</Label>
					<Input id="ag-name" bind:value={connectionForm.name} />
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-url">Base URL</Label>
					<Input id="ag-url" bind:value={connectionForm.baseUrl} placeholder="http://adguard:3000" />
				</div>
			</div>
			<div class="grid grid-cols-3 gap-3">
				<div class="grid gap-1.5">
					<Label for="ag-mode">Target</Label>
					<select
						id="ag-mode"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={connectionForm.targetMode}
					>
						<option value="static_host">Static host</option>
						<option value="static_ip">Static IP</option>
						<option value="pulse_agent">Pulse agent</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-scheme">Scheme</Label>
					<select
						id="ag-scheme"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={connectionForm.scheme}
					>
						<option value="http">HTTP</option>
						<option value="https">HTTPS</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-port">Port</Label>
					<Input id="ag-port" type="number" min="1" max="65535" bind:value={connectionForm.port} />
				</div>
			</div>
			{#if connectionForm.targetMode === 'static_host'}
				<div class="grid gap-1.5">
					<Label for="ag-static-host">Static host</Label>
					<Input id="ag-static-host" bind:value={connectionForm.staticHost} placeholder="adguard.internal" />
				</div>
			{:else if connectionForm.targetMode === 'static_ip'}
				<div class="grid gap-1.5">
					<Label for="ag-static-ip">Static IP</Label>
					<Input id="ag-static-ip" bind:value={connectionForm.staticIp} placeholder="10.0.0.53" />
				</div>
			{:else}
				<div class="grid grid-cols-3 gap-3">
					<div class="grid gap-1.5">
						<Label for="ag-pulse-agent">Pulse agent</Label>
						<select
							id="ag-pulse-agent"
							class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
							bind:value={connectionForm.pulseAgentId}
						>
							<option value="">Select agent</option>
							{#each pulseAgents as agent (agent.id)}
								<option value={agent.id}>
									{agent.name} - {agent.lastSelectedIp ?? agent.lastPrivateIp ?? agent.lastPublicIp ?? agent.status}
								</option>
							{/each}
						</select>
					</div>
					<div class="grid gap-1.5">
						<Label for="ag-pulse-mode">Pulse IP</Label>
						<select
							id="ag-pulse-mode"
							class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
							bind:value={connectionForm.pulseIpMode}
						>
							<option value="selected">Selected</option>
							<option value="public">Public</option>
							<option value="private_selected">Private selected</option>
							<option value="private_candidate">Private candidate</option>
						</select>
					</div>
					<div class="grid gap-1.5">
						<Label for="ag-private-selector">Candidate</Label>
						<Input
							id="ag-private-selector"
							bind:value={connectionForm.privateCandidateSelector}
							placeholder="selected, first_ipv4, address=10.0.0.53"
						/>
					</div>
				</div>
				{#if selectedPulseAgent()}
					<p class="text-xs text-muted-foreground">
						Last seen {selectedPulseAgent()?.lastSeenAtUtc ?? 'never'} - public
						{selectedPulseAgent()?.lastPublicIp ?? 'none'} - private
						{selectedPulseAgent()?.lastPrivateIpv4Candidates.join(', ') || 'none'}
					</p>
				{/if}
			{/if}
			<div class="grid grid-cols-3 gap-3">
				<div class="grid gap-1.5">
					<Label for="ag-path">Path prefix</Label>
					<Input id="ag-path" bind:value={connectionForm.pathPrefix} placeholder="/control-plane" />
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-tls-mode">TLS validation</Label>
					<select
						id="ag-tls-mode"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={connectionForm.tlsValidationMode}
					>
						<option value="system">System</option>
						<option value="expected_hostname">Expected hostname</option>
						<option value="skip">Skip</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-expected-host">Expected hostname</Label>
					<Input id="ag-expected-host" bind:value={connectionForm.expectedTlsHostname} />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="ag-pass">Admin password</Label>
				<Input id="ag-pass" type="password" bind:value={connectionForm.password} />
			</div>
			<Button onclick={() => createConnection()} disabled={saving || loading}>
				{saving ? 'Saving…' : 'Add connection'}
			</Button>
		</div>
	</PanelSection>

	<PanelSection title="Managed rewrites" description="Hashi-owned rewrites only; manual entries are never deleted.">
		<div class="mb-4 flex flex-wrap items-end gap-3">
			<div class="grid min-w-[12rem] gap-1.5">
				<Label for="ag-conn">Connection</Label>
				<select
					id="ag-conn"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={selectedConnectionId}
					disabled={connections.length === 0}
				>
					{#each connections as conn (conn.id)}
						<option value={conn.id}>{conn.name}</option>
					{/each}
				</select>
			</div>
			<Button variant="outline" onclick={() => testConnection()} disabled={!selectedConnectionId || testing}>
				{testing ? 'Testing…' : 'Test connection'}
			</Button>
			<Button variant="outline" onclick={() => syncConnection()} disabled={!selectedConnectionId || syncing}>
				{syncing ? 'Syncing…' : 'Push sync'}
			</Button>
		</div>
		{#if selectedConnection()}
			<div class="mb-4 grid gap-1 text-xs text-muted-foreground">
				<p>
					Resolved target: {selectedConnection()?.resolvedBaseUrl ?? selectedConnection()?.baseUrl} -
					{selectedConnection()?.targetStatus}
				</p>
				{#if selectedConnection()?.targetError}
					<p class="text-amber-300">{selectedConnection()?.targetError}</p>
				{/if}
			</div>
		{/if}

		<div class="mb-4 grid max-w-xl gap-3 rounded-md border border-border p-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="ag-domain">Domain</Label>
					<Input id="ag-domain" bind:value={rewriteForm.domain} placeholder="*.internal.example" />
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-answer">Answer</Label>
					<Input id="ag-answer" bind:value={rewriteForm.answer} placeholder="10.0.0.5" />
				</div>
			</div>
			<Button onclick={() => upsertRewrite()} disabled={saving || !selectedConnectionId}>
				{saving ? 'Saving…' : 'Save rewrite'}
			</Button>
		</div>

		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if !selectedConnectionId}
			<p class="text-sm text-muted-foreground">Add an AdGuard connection to manage rewrites.</p>
		{:else if rewrites.length === 0}
			<p class="text-sm text-muted-foreground">No Hashi-managed rewrites for this connection.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Domain</TableHead>
						<TableHead>Answer</TableHead>
						<TableHead>Source</TableHead>
						<TableHead>Managed</TableHead>
						<TableHead class="w-24"></TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each rewrites as rewrite (rewrite.id)}
						<TableRow>
							<TableCell class="font-mono text-xs">{rewrite.domain}</TableCell>
							<TableCell class="font-mono text-xs">{rewrite.answer}</TableCell>
							<TableCell>
								<span
									class={rewrite.source === 'internal_agent_dns'
										? 'rounded border border-emerald-500/40 bg-emerald-500/10 px-2 py-0.5 text-xs text-emerald-200'
										: 'text-xs text-muted-foreground'}
								>
									{rewrite.source === 'internal_agent_dns' ? 'internal agent DNS' : rewrite.source}
								</span>
							</TableCell>
							<TableCell>{rewrite.managedByHashi ? 'yes' : 'no'}</TableCell>
							<TableCell>
								{#if rewrite.managedByHashi && rewrite.source !== 'internal_agent_dns'}
									<Button
										variant="ghost"
										size="sm"
										disabled={deletingId === rewrite.id}
										onclick={() => deleteRewrite(rewrite.id)}
									>
										{deletingId === rewrite.id ? '…' : 'Delete'}
									</Button>
								{/if}
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>

	{#if message}<p class="text-xs text-emerald-300">{message}</p>{/if}
	{#if error}<p class="text-xs text-destructive">{error}</p>{/if}
</AdminSectionPage>
