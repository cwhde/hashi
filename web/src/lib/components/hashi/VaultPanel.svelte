<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import VaultUnlockDialog from '$lib/components/hashi/VaultUnlockDialog.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Fingerprint, Lock, LockOpen, Trash2 } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { usePasskey, ceremonyText } from '$lib/auth/webauthn';
	import { dateTime } from '$lib/format';
	import { vaultState } from '$lib/vault.svelte';

	// Settings → Account → Vault (02 §5.4): mode, passkeys that unlock it, switching modes.
	let { vault }: { vault: Schemas['VaultState'] } = $props();
	let busy = $state('');
	let error = $state('');
	let notice = $state('');
	let confirmStrict = $state(false);
	let confirmService = $state(false);
	let unlockOpen = $state(false);
	const strict = $derived(vault.mode === 'strict');

	function failed(e: unknown) {
		error = e && typeof e === 'object' && 'type' in e ? problemText(e) : ceremonyText(e);
	}

	async function run(what: string, f: () => Promise<Schemas['VaultState']>, ok: string) {
		busy = what;
		error = '';
		notice = '';
		try {
			vaultState.set(await f());
			notice = ok;
		} catch (e) {
			failed(e);
		} finally {
			busy = '';
		}
	}

	const addUnlocker = () =>
		run(
			'add',
			async () => {
				const opts = await api.POST('/api/vault/unlockers/options');
				if (opts.error || !opts.data) throw opts.error;
				const credential = await usePasskey(opts.data.options);
				const res = await api.POST('/api/vault/unlockers', {
					body: { ceremony_id: opts.data.ceremony_id, credential }
				});
				if (res.error || !res.data) throw res.error;
				return res.data;
			},
			'This passkey can unlock the vault now.'
		);
	const removeUnlocker = (id: string) =>
		run(
			'remove',
			() => withReauth(() => api.DELETE('/api/vault/unlockers/{id}', { params: { path: { id } } })),
			'Removed. That passkey no longer unlocks the vault.'
		);
	const setMode = (mode: 'service' | 'strict') =>
		run(
			'mode',
			() => withReauth(() => api.PUT('/api/vault/mode', { body: { mode } })),
			mode === 'strict'
				? 'Strict mode is on. Remove the master key from the deployment; Hashi starts locked from the next restart.'
				: 'Service mode is on. Hashi unlocks with the master key file at every start again.'
		);
</script>

<Panel title="Vault">
	<div class="space-y-4 text-[13px]">
		<div class="flex flex-wrap items-start gap-3">
			{#if vault.locked}<Lock class="mt-0.5 size-4 text-warn-text" />{:else}<LockOpen
					class="mt-0.5 size-4 text-ok-text"
				/>{/if}
			<div class="min-w-0 flex-1 space-y-1">
				<p class="flex flex-wrap items-center gap-2">
					<span class="font-medium">{strict ? 'Strict mode' : 'Service mode'}</span>
					<Tag tone={vault.locked ? 'warn' : 'ok'}>{vault.locked ? 'locked' : 'unlocked'}</Tag>
				</p>
				<p class="text-muted-foreground">
					{#if strict}
						The master key is not on disk. After every restart Hashi stays locked until you unlock
						it with a passkey below or with the recovery key.
					{:else}
						Secrets unlock with the deployment's master key, so syncs keep running after a restart.
						Strict mode keeps them locked until you unlock with a passkey or the recovery key.
					{/if}
				</p>
				<p class="text-muted-foreground">
					{vault.secrets} stored secret{vault.secrets === 1 ? '' : 's'} · master key version {vault.key_version}
					· recovery key {vault.recovery_key_confirmed ? 'confirmed' : 'not confirmed'}
				</p>
			</div>
			{#if vault.locked}
				<Button size="sm" onclick={() => (unlockOpen = true)}><Fingerprint />Unlock…</Button>
			{:else if strict}
				<Button
					size="sm"
					variant="outline"
					disabled={!!busy}
					onclick={() => (confirmService = true)}>Switch to service mode…</Button
				>
			{:else}
				<Button
					size="sm"
					variant="outline"
					disabled={!!busy || !vault.recovery_key_confirmed}
					title={vault.recovery_key_confirmed ? undefined : 'Confirm the recovery key first'}
					onclick={() => (confirmStrict = true)}>Switch to strict mode…</Button
				>
			{/if}
		</div>

		<div class="rounded-xl border">
			<div class="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
				<span class="text-[12.5px] font-medium">Passkeys that unlock the vault</span>
				<Button size="xs" variant="ghost" disabled={!!busy || vault.locked} onclick={addUnlocker}
					><Fingerprint />{busy === 'add' ? 'Waiting for the passkey…' : 'Add a passkey'}</Button
				>
			</div>
			{#if vault.unlockers.length === 0}
				<p class="px-3 py-3 text-muted-foreground">
					None yet. Only a passkey that supports PRF can unlock; your browser tells you if this one
					can't. {strict
						? 'Until then, unlock with the recovery key.'
						: 'Used in strict mode only.'}
				</p>
			{:else}
				<ul class="divide-y">
					{#each vault.unlockers as u (u.id)}
						<li class="flex flex-wrap items-center gap-2 px-3 py-2">
							<span class="flex-1">{u.passkey_name}</span>
							<span class="text-[12px] text-muted-foreground">added {dateTime(u.created_at)}</span>
							<Button
								size="xs"
								variant="ghost"
								aria-label="Remove {u.passkey_name} as an unlocker"
								disabled={!!busy}
								onclick={() => removeUnlocker(u.id)}><Trash2 /></Button
							>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
		{#if error}<p role="alert" class="text-bad-text">{error}</p>{/if}
		{#if notice}<p role="status" class="text-ok-text">{notice}</p>{/if}
	</div>
</Panel>

<VaultUnlockDialog bind:open={unlockOpen} />

<AlertDialog.Root bind:open={confirmStrict}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Switch to strict mode?</AlertDialog.Title>
			<AlertDialog.Description>
				<span class="block space-y-2 text-left">
					<span class="block"
						>After every restart or update Hashi starts locked. Until you unlock it, DNS, Traefik,
						firewall, AdGuard, certificate and script changes wait, and so do notifications.</span
					>
					<span class="block"
						>Monitoring, public pages, sign-ins, Pulse agents, CAPTCHA passes and signed-in visitors
						of protected apps keep working.</span
					>
					<span class="block"
						>Unlock with a passkey listed here or with the recovery key. Then remove
						<Mono>hashi_master_key</Mono> from the deployment: strict mode ignores it, and a stolen disk
						or backup no longer opens your secrets.</span
					>
					{#if vault.unlockers.length === 0}<span class="block font-medium text-warn-text"
							>No passkey can unlock yet: only the recovery key will.</span
						>{/if}
				</span>
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action onclick={() => setMode('strict')}
				>Switch to strict mode</AlertDialog.Action
			>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={confirmService}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Switch to service mode?</AlertDialog.Title>
			<AlertDialog.Description>
				{#if vault.master_key_file}
					Hashi will unlock with the master key file at every start, so syncs run after a restart
					without you. The passkey unlockers are removed.
				{:else}
					Install the master key as <Mono>hashi_master_key</Mono> (HASHI_MASTER_KEY_FILE) first. Service
					mode needs that file at every start, so Hashi checks it holds this vault's key before switching.
					If installing it restarted Hashi, unlock once more, then switch.
				{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action disabled={!vault.master_key_file} onclick={() => setMode('service')}
				>Switch to service mode</AlertDialog.Action
			>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
