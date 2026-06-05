<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { CONNECTION_TYPES } from '$lib/api/connection-types';
	import type { PulseAgent } from '$lib/api/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let name = $state('edge-firewall');
	let host = $state('');
	let staticIp = $state('');
	let targetMode = $state('static_host');
	let pulseAgentId = $state('');
	let pulseIpMode = $state('selected');
	let privateCandidateSelector = $state('selected');
	let pulseAgents = $state<PulseAgent[]>([]);
	let sshUser = $state('root');
	let sshPassword = $state('');
	let subnets = $state('192.168.0.0/16');
	let domain = $state('home.arpa');
	let traefikLink = $state('primary-traefik');
	let traefikTargetIp = $state('');
	let connectionId = $state<string | null>(null);
	let validating = $state(false);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);

	$effect(() => {
		void loadPulseAgents();
	});

	async function loadPulseAgents() {
		try {
			pulseAgents = await api.listPulseAgents();
		} catch {
			pulseAgents = [];
		}
	}

	function selectedPulseAgent() {
		return pulseAgents.find((agent) => agent.id === pulseAgentId) ?? null;
	}

	function targetHost() {
		if (targetMode === 'static_ip') return staticIp;
		if (targetMode === 'pulse_agent') return selectedPulseAgent()?.name ?? pulseAgentId;
		return host;
	}

	function targetReady() {
		return targetMode === 'static_host'
			? !!host
			: targetMode === 'static_ip'
				? !!staticIp
				: !!pulseAgentId;
	}

	function sshBody() {
		return {
			name,
			connectionType: CONNECTION_TYPES.firewallHost,
			host: targetHost(),
			port: 22,
			username: sshUser,
			authMode: 'password' as const,
			password: sshPassword || null,
			privateKeyPem: null,
			privateKeyPassphrase: null,
			target: {
				targetMode,
				staticHost: targetMode === 'static_host' ? host : null,
				staticIp: targetMode === 'static_ip' ? staticIp : null,
				pulseAgentId: targetMode === 'pulse_agent' ? pulseAgentId : null,
				pulseIpMode,
				privateCandidateSelector,
				port: 22,
				scheme: 'http',
				pathPrefix: null,
				tlsValidationMode: 'system',
				expectedTlsHostname: null
			}
		};
	}

	async function validateHost() {
		if (!targetReady()) {
			error = 'Connection target is required.';
			return;
		}
		validating = true;
		error = null;
		message = null;
		try {
			const created = (await api.createSshConnection(sshBody())) as { id?: string };
			connectionId = created.id ?? null;
			if (!connectionId) throw new Error('Connection was not created.');
			await api.validateConnection(connectionId);
			message = 'Firewall host SSH validated.';
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Validation failed';
		} finally {
			validating = false;
		}
	}

	async function save() {
		saving = true;
		error = null;
		try {
			if (!connectionId) {
				await validateHost();
			}
			if (!connectionId) return;
			const subnetList = subnets
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
			await api.createFirewallHost({
				connectionId,
				name,
				domain,
				managedSubnets: subnetList,
				linkedTraefikHost: traefikLink,
				internalTraefikIp: traefikTargetIp || '127.0.0.1'
			});
			await oncomplete();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save firewall host';
		} finally {
			saving = false;
		}
	}
</script>

<div class="grid max-w-xl gap-4">
	<div class="grid gap-1.5">
		<Label for="fw-name">Host name</Label>
		<Input id="fw-name" bind:value={name} />
	</div>
	<div class="grid grid-cols-3 gap-3">
		<div class="grid gap-1.5">
			<Label for="fw-target-mode">Target</Label>
			<select
				id="fw-target-mode"
				class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
				bind:value={targetMode}
			>
				<option value="static_host">Static host</option>
				<option value="static_ip">Static IP</option>
				<option value="pulse_agent">Pulse agent</option>
			</select>
		</div>
		<div class="grid gap-1.5">
			<Label for="fw-host">Host / IP</Label>
			{#if targetMode === 'static_ip'}
				<Input id="fw-host" bind:value={staticIp} />
			{:else if targetMode === 'pulse_agent'}
				<select
					id="fw-host"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={pulseAgentId}
				>
					<option value="">Select agent</option>
					{#each pulseAgents as agent (agent.id)}
						<option value={agent.id}>
							{agent.name} - {agent.lastSelectedIp ?? agent.lastPrivateIp ?? agent.lastPublicIp ?? agent.status}
						</option>
					{/each}
				</select>
			{:else}
				<Input id="fw-host" bind:value={host} />
			{/if}
		</div>
		<div class="grid gap-1.5">
			<Label for="fw-user">SSH username</Label>
			<Input id="fw-user" bind:value={sshUser} />
		</div>
	</div>
	{#if targetMode === 'pulse_agent'}
		<div class="grid grid-cols-2 gap-3">
			<div class="grid gap-1.5">
				<Label for="fw-pulse-mode">Pulse IP</Label>
				<select
					id="fw-pulse-mode"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={pulseIpMode}
				>
					<option value="selected">Selected</option>
					<option value="public">Public</option>
					<option value="private_selected">Private selected</option>
					<option value="private_candidate">Private candidate</option>
				</select>
			</div>
			<div class="grid gap-1.5">
				<Label for="fw-private-selector">Candidate</Label>
				<Input id="fw-private-selector" bind:value={privateCandidateSelector} />
			</div>
		</div>
	{/if}
	<div class="grid gap-1.5">
		<Label for="fw-pass">SSH password</Label>
		<Input id="fw-pass" type="password" bind:value={sshPassword} />
	</div>
	<div class="grid gap-1.5">
		<Label for="fw-domain">Domain suffix</Label>
		<Input id="fw-domain" bind:value={domain} />
	</div>
	<div class="grid gap-1.5">
		<Label for="fw-subnets">Managed subnets (comma-separated CIDRs)</Label>
		<Input id="fw-subnets" bind:value={subnets} />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="fw-traefik">Linked Traefik connection</Label>
			<Input id="fw-traefik" bind:value={traefikLink} />
		</div>
		<div class="grid gap-1.5">
			<Label for="fw-target">Internal Traefik target IP</Label>
			<Input id="fw-target" bind:value={traefikTargetIp} />
		</div>
	</div>

	{#if message}<p class="text-xs text-emerald-300">{message}</p>{/if}
	{#if error}<p class="text-xs text-destructive">{error}</p>{/if}

	<div class="flex gap-2">
		<Button variant="outline" onclick={() => validateHost()} disabled={validating || !targetReady()}>
			{validating ? 'Validating…' : 'Validate host'}
		</Button>
		<Button onclick={() => save()} disabled={advancing || saving}>
			{saving ? 'Saving…' : 'Save & continue'}
		</Button>
	</div>
</div>
