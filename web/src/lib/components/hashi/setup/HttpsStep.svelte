<script lang="ts">
	// Setup step 8 (02 §1 step 8): the browser opens setup on https://<admin domain> with a one-time link. The setup
	// page there sends the link's nonce back; Hashi accepts it only through its own Traefik and signs this browser in on
	// the HTTPS address, where passkeys (step 9) work.
	import { Button } from '$lib/components/ui/button';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Check, CircleAlert, ExternalLink } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';

	type Props = {
		setup: Schemas['SetupState'];
		oncontinue: () => void;
		/** Set by the setup page when the proof it sent on arrival failed. */
		proofError?: string;
	};
	let { setup, oncontinue, proofError = '' }: Props = $props();

	const step = $derived(setup.steps.find((s) => s.n === 8));
	const admin = $derived(setup.base_settings?.admin_domain ?? '');
	const here = $derived(
		typeof location !== 'undefined' && location.protocol === 'https:' && location.hostname === admin
	);
	let busy = $state(false);
	let error = $state('');

	async function open() {
		error = '';
		busy = true;
		try {
			const res = await api.POST('/api/setup/https/start');
			if (res.error || !res.data) throw res.error;
			location.assign(res.data.url);
		} catch (e) {
			error = problemText(e);
			busy = false;
		}
	}
</script>

<h1 class="mt-1 text-[24px] font-semibold">Open Hashi over HTTPS</h1>
<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
	Passkeys only work on Hashi's own HTTPS address. This step opens setup on <Mono
		>https://{admin}</Mono
	> through the Traefik host and continues there.
</p>

<section class="mt-6 rounded-3xl border bg-card p-6">
	{#if step?.status === 'done' && here}
		<p class="flex items-center gap-2 text-[13.5px]">
			<Check class="size-4 text-ok-text" />This page came through Hashi's Traefik over HTTPS.
		</p>
	{:else if step?.status === 'done'}
		<p class="flex items-center gap-2 text-[13.5px]">
			<Check class="size-4 text-ok-text" />Done. Continue on <Mono>https://{admin}</Mono>: passkeys
			cannot be registered on this address.
		</p>
		<Button class="mt-4" onclick={open} disabled={busy}><ExternalLink />Open https://{admin}</Button
		>
	{:else}
		<ul class="space-y-1.5 text-[13px] text-muted-foreground">
			<li>Your computer must reach the Traefik host and resolve {admin} to it.</li>
			<li>The link works once and for five minutes. You stay signed in on the new address.</li>
		</ul>
		<Button class="mt-4" onclick={open} disabled={busy}
			><ExternalLink />{busy ? 'Opening…' : `Open https://${admin}`}</Button
		>
	{/if}
	{#if error || proofError}
		<p
			role="alert"
			class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
		>
			<CircleAlert class="mt-0.5 size-4 shrink-0" />{error || proofError}
		</p>
	{/if}
</section>

<div class="mt-6 flex justify-end">
	<Button onclick={oncontinue} disabled={step?.status !== 'done'}>Continue</Button>
</div>
