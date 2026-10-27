<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Fingerprint } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { session } from '$lib/auth/session.svelte';
	import { ceremonyText, createPasskey } from '$lib/auth/webauthn';

	// Console recovery (02-setup-auth.md §2): the token is in the URL fragment, so it never reached any server log.
	let phase = $state<'opening' | 'ready' | 'done' | 'failed'>('opening');
	let failed = $state('');
	let name = $state('');
	let busy = $state(false);

	onMount(async () => {
		const token = location.hash.slice(1);
		history.replaceState(null, '', location.pathname); // drop the token from the address bar
		if (!token) {
			phase = 'failed';
			failed =
				'This link has no recovery token. Run `hashi admin recover` again and open the new link.';
			return;
		}
		const res = await api.POST('/api/auth/recover', { body: { token } });
		if (res.error || !res.data) {
			phase = 'failed';
			failed = problemText(res.error);
			return;
		}
		session.set(res.data);
		phase = 'ready';
	});

	async function register() {
		failed = '';
		busy = true;
		try {
			const opts = await api.POST('/api/auth/recover/passkey/options');
			if (opts.error || !opts.data) throw opts.error;
			let credential;
			try {
				credential = await createPasskey(opts.data.options);
			} catch (e) {
				failed = ceremonyText(e);
				return;
			}
			const res = await api.POST('/api/auth/recover/passkey/verify', {
				body: {
					ceremony_id: opts.data.ceremony_id,
					name: name.trim() || 'Recovered passkey',
					credential
				}
			});
			if (res.error) throw res.error;
			phase = 'done';
		} catch (e) {
			failed = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Recover access · Hashi</title></svelte:head>
<main class="grid min-h-dvh place-items-center px-4">
	<div class="w-full max-w-sm rounded-3xl border bg-card p-7">
		<h1 class="text-[20px] font-semibold">Register a new passkey</h1>
		{#if phase === 'opening'}
			<p class="mt-2 text-[13.5px] text-muted-foreground">Checking the recovery link…</p>
		{:else if phase === 'ready'}
			<p class="mt-2 text-[13.5px] text-muted-foreground">
				This one-time session can only add one passkey. Your other passkeys stay as they are.
			</p>
			<div class="mt-5 space-y-1.5">
				<Label for="pk-name">Passkey name</Label>
				<Input id="pk-name" bind:value={name} placeholder="Laptop" maxlength={64} />
			</div>
			<Button class="mt-5 h-10 w-full" onclick={register} disabled={busy}
				><Fingerprint />{busy ? 'Waiting for your passkey…' : 'Create passkey'}</Button
			>
		{:else if phase === 'done'}
			<p class="mt-2 text-[13.5px]">The new passkey is saved. Sign in with it now.</p>
			<Button class="mt-5 h-10 w-full" onclick={() => goto('/login', { replaceState: true })}
				>Go to sign-in</Button
			>
		{/if}
		{#if failed}
			<p
				role="alert"
				class="mt-5 rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
			>
				{failed}
			</p>
		{/if}
	</div>
</main>
