<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import {
		extractPrfOutput,
		isPrfSupported,
		isWebAuthnSupported,
		registerPasskeyFromServerOptions,
		serializeRegistration
	} from '$lib/auth/webauthn';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { KeyRound, Shield } from 'lucide-svelte';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let webauthnOk = $state(false);
	let prfOk = $state(false);
	let recoveryKey = $state('');
	let confirmedRecovery = $state(false);
	let registering = $state(false);
	let configuring = $state(false);
	let credentialId = $state<string | null>(null);
	let prfOutput = $state<string | null>(null);
	let message = $state<string | null>(null);
	let error = $state<string | null>(null);
	let serviceSyncVaultReady = $state(false);
	let acknowledgedServiceSync = $state(false);

	onMount(async () => {
		webauthnOk = isWebAuthnSupported();
		prfOk = await isPrfSupported();
		try {
			const generated = await api.generateRecoveryKey();
			recoveryKey = generated.recoveryKey;
		} catch {
			recoveryKey = '(offline — start API to generate)';
		}
		try {
			const vStatus = await api.getVaultStatus();
			serviceSyncVaultReady = vStatus.serviceSyncVaultReady;
		} catch {
			// ignore
		}
	});

	async function registerPasskey() {
		registering = true;
		error = null;
		message = null;
		try {
			const begin = await api.passkeyRegisterBegin('Primary passkey');
			const options = begin.options as Record<string, unknown>;
			const challengeSessionId = String(begin.challengeSessionId ?? '');
			const credential = await registerPasskeyFromServerOptions(options);
			const attestation = serializeRegistration(credential);
			const complete = await api.passkeyRegisterComplete(
				attestation,
				challengeSessionId,
				'Primary passkey',
				prfOk
			);
			credentialId = complete.credentialId;
			prfOutput = extractPrfOutput(credential);
			message = complete.prfSupported
				? 'Passkey registered with PRF support.'
				: 'Passkey registered. Vault will use recovery key wrap.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Passkey registration failed';
		} finally {
			registering = false;
		}
	}

	async function configureVault() {
		if (!credentialId || !recoveryKey || !confirmedRecovery) return;
		if (serviceSyncVaultReady && !acknowledgedServiceSync) return;
		configuring = true;
		error = null;
		try {
			await api.setupVault({
				recoveryKey,
				prfWrapAttempted: prfOutput !== null,
				prfOutputBase64: prfOutput,
				passkeyCredentialId: credentialId
			});
			await api.verifyVaultUnlock(recoveryKey);
			await oncomplete();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Vault setup failed';
		} finally {
			configuring = false;
		}
	}
</script>

<Alert>
	<AlertTitle>Passkey + vault</AlertTitle>
	<AlertDescription>
		Register a passkey for admin auth. PRF enables passkey-bound vault encryption when supported;
		recovery key is always required.
	</AlertDescription>
</Alert>

<div class="mt-4 flex flex-wrap gap-2">
	<Badge variant={webauthnOk ? 'default' : 'outline'}>
		WebAuthn {webauthnOk ? 'available' : 'unavailable'}
	</Badge>
	<Badge variant={prfOk ? 'default' : 'secondary'}>
		PRF {prfOk ? 'supported' : 'fallback to recovery key'}
	</Badge>
</div>

<div class="mt-4 grid max-w-xl gap-4">
	<Button variant="outline" disabled={!webauthnOk || registering} onclick={() => registerPasskey()}>
		<KeyRound class="size-4" />
		{registering ? 'Registering…' : credentialId ? 'Passkey registered' : 'Register passkey'}
	</Button>

	<div class="grid gap-1.5">
		<Label for="recovery-key">Recovery key (store offline)</Label>
		<Input id="recovery-key" readonly value={recoveryKey} class="font-mono text-xs" />
	</div>
	<div class="flex items-center gap-2">
		<Checkbox bind:checked={confirmedRecovery} id="confirm-recovery" />
		<Label for="confirm-recovery">I have saved the recovery key securely</Label>
	</div>

	{#if serviceSyncVaultReady}
		<Alert class="border-amber-500/50 bg-amber-500/10 text-amber-200">
			<AlertTitle>Security Warning: Unattended Sync Enabled</AlertTitle>
			<AlertDescription class="text-xs leading-relaxed">
				An environment key for the service-sync vault is configured. This enables unattended routine synchronization, allowing background syncs to decrypt secrets without an active browser session.
				<br /><br />
				<strong>Security Tradeoff:</strong> If this server is compromised, anyone with access to the vault key will be able to expose sync secrets.
			</AlertDescription>
		</Alert>
		<div class="flex items-start gap-2">
			<Checkbox bind:checked={acknowledgedServiceSync} id="acknowledge-servicesync" />
			<Label for="acknowledge-servicesync" class="text-xs leading-tight">
				I understand and accept the security tradeoff of enabling the unattended service-sync vault.
			</Label>
		</div>
	{/if}

	{#if message}
		<p class="text-xs text-emerald-300">{message}</p>
	{/if}
	{#if error}
		<p class="text-xs text-destructive">{error}</p>
	{/if}

	<div class="flex justify-end">
		<Button
			onclick={() => configureVault()}
			disabled={advancing || configuring || !credentialId || !confirmedRecovery || (serviceSyncVaultReady && !acknowledgedServiceSync)}
		>
			<Shield class="size-4" />
			{configuring ? 'Configuring vault…' : 'Configure vault & continue'}
		</Button>
	</div>
</div>
