<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let name = $state('primary-traefik');
	let host = $state('');
	let sshUser = $state('root');
	let sshPassword = $state('');
	let useKey = $state(false);
	let privateKeyPem = $state('');
	let keyPassphrase = $state('');
	let internalIp = $state('');
	let configPath = $state('/etc/traefik');
	let replaceConfirmed = $state(false);
	let connectionId = $state<string | null>(null);
	let validating = $state(false);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);

	function connectionBody() {
		return {
			name,
			connectionType: 'traefik',
			host,
			port: 22,
			username: sshUser,
			authMode: useKey ? 'private_key' : 'password',
			password: useKey ? null : sshPassword || null,
			privateKeyPem: useKey ? privateKeyPem || null : null,
			privateKeyPassphrase: useKey ? keyPassphrase || null : null
		};
	}

	async function validateSsh() {
		if (!host) {
			error = 'Host is required.';
			return;
		}
		validating = true;
		error = null;
		message = null;
		try {
			const created = (await api.createSshConnection(connectionBody())) as { id?: string };
			connectionId = created.id ?? null;
			if (!connectionId) throw new Error('Connection was not created.');
			await api.validateConnection(connectionId, connectionBody());
			message = `SSH validated for ${host}. Config path: ${configPath}, internal IP: ${internalIp || 'unset'}.`;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'SSH validation failed';
		} finally {
			validating = false;
		}
	}

	async function save() {
		if (!replaceConfirmed) {
			error = 'Confirm backup and Hashi ownership before continuing.';
			return;
		}
		saving = true;
		error = null;
		try {
			if (!connectionId) {
				await validateSsh();
			}
			if (!connectionId) return;
			await oncomplete();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save Traefik connection';
		} finally {
			saving = false;
		}
	}
</script>

<div class="grid max-w-xl gap-4">
	<div class="grid gap-1.5">
		<Label for="traefik-name">Connection name</Label>
		<Input id="traefik-name" bind:value={name} />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="traefik-host">Host / IP</Label>
			<Input id="traefik-host" bind:value={host} placeholder="10.0.0.5" />
		</div>
		<div class="grid gap-1.5">
			<Label for="traefik-user">SSH username</Label>
			<Input id="traefik-user" bind:value={sshUser} />
		</div>
	</div>
	<div class="flex items-center gap-2 text-xs">
		<Checkbox bind:checked={useKey} id="traefik-key" />
		<Label for="traefik-key">Use SSH private key instead of password</Label>
	</div>
	{#if useKey}
		<div class="grid gap-1.5">
			<Label for="traefik-pem">Private key PEM</Label>
			<Input id="traefik-pem" bind:value={privateKeyPem} />
		</div>
		<div class="grid gap-1.5">
			<Label for="traefik-passphrase">Private key passphrase (optional)</Label>
			<Input id="traefik-passphrase" type="password" bind:value={keyPassphrase} />
		</div>
	{:else}
		<div class="grid gap-1.5">
			<Label for="traefik-password">SSH password</Label>
			<Input id="traefik-password" type="password" bind:value={sshPassword} />
		</div>
	{/if}
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="traefik-internal">Internal Traefik IP</Label>
			<Input id="traefik-internal" bind:value={internalIp} />
		</div>
		<div class="grid gap-1.5">
			<Label for="traefik-path">Config path</Label>
			<Input id="traefik-path" bind:value={configPath} />
		</div>
	</div>

	<div class="rounded-md border border-border p-3 text-xs text-muted-foreground">
		Validation runs SSH login, OS detection, and write permission checks via the connections API.
	</div>

	<div class="flex items-center gap-2">
		<Checkbox bind:checked={replaceConfirmed} id="traefik-replace" />
		<Label for="traefik-replace">Confirm backup and Hashi ownership if configs exist</Label>
	</div>

	{#if message}<p class="text-xs text-emerald-300">{message}</p>{/if}
	{#if error}<p class="text-xs text-destructive">{error}</p>{/if}

	<div class="flex gap-2">
		<Button variant="outline" onclick={() => validateSsh()} disabled={validating || !host}>
			{validating ? 'Validating…' : 'Validate SSH'}
		</Button>
		<Button onclick={() => save()} disabled={advancing || saving}>
			{saving ? 'Saving…' : 'Save & continue'}
		</Button>
	</div>
</div>
