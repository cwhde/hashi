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

	let acmeEmail = $state('');
	let eabKeyId = $state('');
	let eabHmac = $state('');
	let dnsDelay = $state(30);
	let resolvers = $state('1.1.1.1:53,8.8.8.8:53');
	let validationMessage = $state<string | null>(null);
	let saving = $state(false);
	let error = $state<string | null>(null);

	$effect(() => {
		void loadExisting();
	});

	async function loadExisting() {
		try {
			const existing = await api.getCertificateSetup();
			acmeEmail = existing.acmeEmail ?? '';
			dnsDelay = Number(existing.dnsChallengeDelaySeconds ?? 30);
			resolvers = (existing.resolvers ?? ['1.1.1.1:53', '8.8.8.8:53']).join(',');
		} catch {
			// Setup may not expose certificate settings yet during bootstrap.
		}
	}

	function buildRequest() {
		return {
			acmeEmail,
			eabKeyId,
			eabHmac,
			dnsChallengeDelaySeconds: dnsDelay,
			resolvers: resolvers
				.split(',')
				.map((entry) => entry.trim())
				.filter(Boolean)
		};
	}

	async function validateAcme() {
		error = null;
		validationMessage = null;
		try {
			const result = await api.validateCertificateSetup(buildRequest());
			validationMessage = result.isValid
				? 'ACME settings look valid.'
				: result.errors.join(' ');
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Validation failed';
		}
	}

	async function saveAndContinue() {
		saving = true;
		error = null;
		try {
			const result = await api.saveCertificateSetup(buildRequest());
			if (!result.saved) {
				error = result.error ?? 'Failed to save certificate settings';
				return;
			}

			await oncomplete();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save certificate settings';
		} finally {
			saving = false;
		}
	}
</script>

<div class="grid max-w-xl gap-4">
	<div class="grid gap-1.5">
		<Label for="acme-email">ACME email</Label>
		<Input id="acme-email" type="email" bind:value={acmeEmail} />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="eab-key-id">EAB key ID</Label>
			<Input id="eab-key-id" bind:value={eabKeyId} />
		</div>
		<div class="grid gap-1.5">
			<Label for="eab-hmac">EAB HMAC</Label>
			<Input id="eab-hmac" type="password" bind:value={eabHmac} />
		</div>
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="dns-delay">DNS challenge delay (seconds)</Label>
			<Input id="dns-delay" type="number" bind:value={dnsDelay} />
		</div>
		<div class="grid gap-1.5">
			<Label for="resolvers">Resolver list</Label>
			<Input id="resolvers" bind:value={resolvers} />
		</div>
	</div>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}
	{#if validationMessage}
		<p class="text-sm text-muted-foreground">{validationMessage}</p>
	{/if}

	<div class="flex gap-2">
		<Button variant="outline" onclick={() => validateAcme()}>Validate ACME</Button>
		<Button onclick={() => saveAndContinue()} disabled={advancing || saving}>
			{saving || advancing ? 'Saving…' : 'Save & continue'}
		</Button>
	</div>
</div>
