<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Fingerprint } from '@lucide/svelte';
	import { api, problemText } from '#lib/api/client.ts';
	import { session } from '#lib/auth/session.svelte.ts';
	import { safeNext } from '#lib/auth/next.ts';
	import { ceremonyText, passkeysSupported, usePasskey } from '#lib/auth/webauthn.ts';

	let failed = $state('');
	let busy = $state(false);
	let supported = $state(true);

	onMount(async () => {
		supported = passkeysSupported();
		try {
			const s = await session.refresh();
			if (!s.setup_completed) goto('/setup', { replace: true });
			else if (s.authenticated && s.method === 'passkey') goto(next(), { replace: true });
		} catch (e) {
			failed = problemText(e);
		}
	});

	const next = () => safeNext(page.url.searchParams.get('next'));

	async function signIn() {
		failed = '';
		busy = true;
		try {
			const opts = await api.POST('/api/auth/passkeys/login/options');
			if (opts.error || !opts.data) throw opts.error;
			let credential;
			try {
				credential = await usePasskey(opts.data.options);
			} catch (e) {
				failed = ceremonyText(e);
				return;
			}
			const res = await api.POST('/api/auth/passkeys/login/verify', {
				body: { ceremony_id: opts.data.ceremony_id, credential }
			});
			if (res.error || !res.data) throw res.error;
			session.set(res.data);
			goto(next(), { replace: true });
		} catch (e) {
			failed = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Sign in · Hashi</title></svelte:head>
<main class="grid min-h-dvh place-items-center px-4">
	<div class="flex w-full max-w-xs flex-col items-center text-center">
		<span
			aria-hidden="true"
			class="grid size-14 place-items-center rounded-2xl bg-primary text-title font-semibold text-primary-foreground"
			>橋</span
		>
		<h1 class="mt-6 text-title font-semibold">Sign in to Hashi</h1>
		{#if page.url.searchParams.has('ended') && !failed}
			<p role="status" class="mt-2 text-md text-muted-foreground">Your session ended.</p>
		{/if}
		{#if !supported}
			<p class="mt-5 w-full rounded-xl border border-warn/30 bg-warn/10 p-3 text-md text-warn-text">
				Passkeys need HTTPS and a current browser.
			</p>
		{/if}
		{#if failed}
			<p
				role="alert"
				class="mt-5 w-full rounded-xl border border-bad/30 bg-bad/10 p-3 text-md text-bad-text"
			>
				{failed}
			</p>
		{/if}
		<Button class="mt-8 h-11 w-full" onclick={signIn} disabled={busy || !supported}
			><Fingerprint />{busy ? 'Waiting for your passkey…' : 'Sign in with a passkey'}</Button
		>
		<details class="group mt-6 text-sm text-muted-foreground">
			<summary
				class="cursor-pointer list-none rounded-md px-2 py-1 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
				>Lost every passkey?</summary
			>
			<p class="mt-2">
				Run <code class="font-mono text-foreground">hashi admin recover</code> on the server.
			</p>
		</details>
	</div>
</main>
