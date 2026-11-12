<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Fingerprint, KeyRound } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { usePasskey, ceremonyText } from '$lib/auth/webauthn';
	import { vaultState } from '$lib/vault.svelte';
	import { pendingPlans } from '$lib/sync-pending.svelte';

	// Unlock a strict-mode vault (02 §5.4): with a passkey that has an unlocker, or with the recovery key.
	let { open = $bindable(false) }: { open?: boolean } = $props();
	let key = $state('');
	let busy = $state<'' | 'passkey' | 'key'>('');
	let error = $state('');
	const unlockers = $derived(vaultState.current?.unlockers ?? []);

	function done(s: NonNullable<typeof vaultState.current>) {
		vaultState.set(s);
		key = '';
		open = false;
		pendingPlans.refresh().catch(() => {});
	}

	async function withPasskey() {
		busy = 'passkey';
		error = '';
		try {
			const opts = await api.POST('/api/vault/unlock/passkey/options');
			if (opts.error || !opts.data) throw opts.error;
			const credential = await usePasskey(opts.data.options);
			const res = await api.POST('/api/vault/unlock/passkey', {
				body: { ceremony_id: opts.data.ceremony_id, credential }
			});
			if (res.error || !res.data) throw res.error;
			done(res.data);
		} catch (e) {
			error = e && typeof e === 'object' && 'type' in e ? problemText(e) : ceremonyText(e);
		} finally {
			busy = '';
		}
	}

	async function withKey(ev: SubmitEvent) {
		ev.preventDefault();
		busy = 'key';
		error = '';
		try {
			const res = await api.POST('/api/vault/unlock/recovery', { body: { recovery_key: key } });
			if (res.error || !res.data) throw res.error;
			done(res.data);
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = '';
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Unlock the vault</Dialog.Title>
			<Dialog.Description>
				Hashi restarted in strict mode, so its secrets stay locked until you unlock them. Syncs,
				scripts and notifications wait until then.
			</Dialog.Description>
		</Dialog.Header>
		{#if unlockers.length}
			<Button onclick={withPasskey} disabled={!!busy} class="w-full">
				<Fingerprint />{busy === 'passkey' ? 'Waiting for the passkey…' : 'Unlock with a passkey'}
			</Button>
			<p class="text-center text-[12px] text-muted-foreground">
				{unlockers.map((u) => u.passkey_name).join(', ')}
			</p>
		{/if}
		<form class="space-y-2" onsubmit={withKey}>
			<Label for="vault-recovery-key"
				>{unlockers.length ? 'Or the recovery key' : 'Recovery key'}</Label
			>
			<Input
				id="vault-recovery-key"
				bind:value={key}
				autocomplete="off"
				spellcheck={false}
				placeholder="XXXX-XXXX-…"
				class="font-mono"
			/>
			<Button
				type="submit"
				variant={unlockers.length ? 'outline' : 'default'}
				disabled={!!busy || key.trim().length < 20}
				class="w-full"
			>
				<KeyRound />{busy === 'key' ? 'Unlocking…' : 'Unlock with the recovery key'}
			</Button>
		</form>
		{#if error}<p role="alert" class="text-[13px] text-bad-text">{error}</p>{/if}
	</Dialog.Content>
</Dialog.Root>
