<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Button } from '$lib/components/ui/button';
	import { Fingerprint } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { session } from '$lib/auth/session.svelte';
	import { ceremonyText, passkeysSupported, usePasskey } from '$lib/auth/webauthn';

	let failed = $state('');
	let busy = $state(false);
	let supported = $state(true);

	onMount(async () => {
		supported = passkeysSupported();
		try {
			const s = await session.refresh();
			if (!s.setup_completed) goto('/setup', { replaceState: true });
			else if (s.authenticated && s.method === 'passkey') goto(next(), { replaceState: true });
		} catch (e) {
			failed = problemText(e);
		}
	});

	// Only same-app paths: never redirect to another origin.
	function next() {
		const n = page.url.searchParams.get('next') ?? '';
		return n.startsWith('/') && !n.startsWith('//') ? n : '/activity';
	}

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
			goto(next(), { replaceState: true });
		} catch (e) {
			failed = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Sign in · Hashi</title></svelte:head>
<main class="grid min-h-dvh place-items-center px-4">
	<div class="w-full max-w-sm rounded-3xl border bg-card p-7">
		<span
			class="grid size-10 place-items-center rounded-xl bg-primary text-[20px] font-semibold text-primary-foreground"
			>橋</span
		>
		<h1 class="mt-5 text-[20px] font-semibold">Sign in to Hashi</h1>
		<p class="mt-1 text-[13.5px] text-muted-foreground">{page.url.host}</p>
		{#if !supported}
			<p class="mt-5 rounded-xl border border-warn/30 bg-warn/10 p-3 text-[13px] text-warn-text">
				Passkeys need HTTPS and a current browser. Open Hashi at its admin domain.
			</p>
		{/if}
		{#if failed}
			<p
				role="alert"
				class="mt-5 rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
			>
				{failed}
			</p>
		{/if}
		<Button class="mt-6 h-10 w-full" onclick={signIn} disabled={busy || !supported}
			><Fingerprint />{busy ? 'Waiting for your passkey…' : 'Sign in with a passkey'}</Button
		>
		<p class="mt-5 text-[12.5px] text-muted-foreground">
			Lost every passkey? Run <code class="font-mono">hashi admin recover</code> on the server for a one-time
			link.
		</p>
	</div>
</main>
