<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
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

	function sshBody() {
		return {
			name,
			connectionType: 'firewall',
			host,
			port: 22,
			username: sshUser,
			authMode: 'password' as const,
			password: sshPassword || null,
			privateKeyPem: null,
			privateKeyPassphrase: null
		};
	}

	async function validateHost() {
		if (!host) {
			error = 'Host is required.';
			return;
		}
		validating = true;
		error = null;
		message = null;
		try {
			const created = (await api.createSshConnection(sshBody())) as { id?: string };
			connectionId = created.id ?? null;
			if (!connectionId) throw new Error('Connection was not created.');
			await api.validateConnection(connectionId, sshBody());
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
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="fw-host">Host / IP</Label>
			<Input id="fw-host" bind:value={host} />
		</div>
		<div class="grid gap-1.5">
			<Label for="fw-user">SSH username</Label>
			<Input id="fw-user" bind:value={sshUser} />
		</div>
	</div>
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
		<Button variant="outline" onclick={() => validateHost()} disabled={validating || !host}>
			{validating ? 'Validating…' : 'Validate host'}
		</Button>
		<Button onclick={() => save()} disabled={advancing || saving}>
			{saving ? 'Saving…' : 'Save & continue'}
		</Button>
	</div>
</div>
