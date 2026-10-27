<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { cn } from '$lib/utils';
	import { Check, CircleAlert, Copy, Fingerprint, Clock } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { session } from '$lib/auth/session.svelte';
	import { ceremonyText, createPasskey, passkeysSupported } from '$lib/auth/webauthn';
	import { theme, themes, type ThemeId } from '$lib/theme.svelte';

	// 02-setup-auth.md §1: a resumable, persisted sequence; a step completes only after its validation succeeded.
	type SetupState = Schemas['SetupState'];
	let setup = $state<SetupState | null>(null);
	let selected = $state(1);
	let loadError = $state('');
	let error = $state('');
	let busy = $state(false);

	const isSetupSession = $derived(session.has('setup.manage'));
	const step = $derived(setup?.steps.find((s) => s.n === selected));

	function firstOpen(s: SetupState): number {
		return s.steps.find((x) => x.built && x.status !== 'done')?.n ?? 11;
	}

	async function load() {
		const [, res] = await Promise.all([session.refresh(), api.GET('/api/setup')]);
		if (res.error || !res.data) throw res.error;
		setup = res.data;
		if (setup.completed) {
			goto('/login', { replaceState: true });
			return;
		}
		selected = session.has('setup.manage') ? firstOpen(setup) : 1;
		if (setup.base_settings) base = { ...setup.base_settings };
	}

	onMount(() => load().catch((e) => (loadError = problemText(e))));

	async function run(fn: () => Promise<void>) {
		error = '';
		busy = true;
		try {
			await fn();
		} catch (e) {
			error = e instanceof Error ? e.message : problemText(e);
		} finally {
			busy = false;
		}
	}

	// Step 1
	let username = $state('');
	let password = $state('');
	const login = () =>
		run(async () => {
			const res = await api.POST('/api/auth/bootstrap/login', { body: { username, password } });
			if (res.error || !res.data) throw res.error;
			password = '';
			session.set(res.data);
			await load();
		});

	// Step 2
	let base = $state<Schemas['BaseSettings']>({
		root_domain: '',
		admin_domain: '',
		internal_url: typeof location !== 'undefined' ? location.origin : '',
		sync_interval_seconds: 3600,
		theme: theme.current,
		public_dashboard_enabled: true,
		public_status_enabled: true
	});
	const saveBase = () =>
		run(async () => {
			const res = await api.PUT('/api/setup/base-settings', { body: base });
			if (res.error || !res.data) throw res.error;
			setup = res.data;
			theme.set(base.theme as ThemeId);
			selected = firstOpen(res.data);
		});

	// Step 9
	let passkeyName = $state('');
	let recoveryKey = $state('');
	let checksum = $state('');
	let copied = $state(false);
	const addPasskey = () =>
		run(async () => {
			const opts = await api.POST('/api/setup/passkey/options');
			if (opts.error || !opts.data) throw opts.error;
			let credential;
			try {
				credential = await createPasskey(opts.data.options);
			} catch (e) {
				throw new Error(ceremonyText(e), { cause: e });
			}
			const res = await api.POST('/api/setup/passkey/verify', {
				body: {
					ceremony_id: opts.data.ceremony_id,
					name: passkeyName.trim() || 'First passkey',
					credential
				}
			});
			if (res.error) throw res.error;
			await load();
		});
	const generateKey = () =>
		run(async () => {
			const res = await api.POST('/api/setup/recovery-key');
			if (res.error || !res.data) throw res.error;
			recoveryKey = res.data.recovery_key;
			checksum = '';
		});
	const confirmKey = () =>
		run(async () => {
			const res = await api.POST('/api/setup/recovery-key/confirm', { body: { checksum } });
			if (res.error || !res.data) throw res.error;
			recoveryKey = '';
			setup = res.data;
			selected = firstOpen(res.data);
		});
	async function copyKey() {
		await navigator.clipboard.writeText(recoveryKey);
		copied = true;
	}

	// Step 11
	const complete = () =>
		run(async () => {
			const res = await api.POST('/api/setup/complete');
			if (res.error) throw res.error;
			goto('/login', { replaceState: true });
		});

	$effect(() => {
		// Errors belong to the step they happened on.
		void selected;
		error = '';
	});
</script>

{#snippet failure(msg: string)}
	<p
		role="alert"
		class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{msg}
	</p>
{/snippet}

<svelte:head><title>Set up Hashi</title></svelte:head>
<div
	class="mx-auto grid min-h-dvh max-w-6xl gap-8 px-4 py-8 md:grid-cols-[16rem_minmax(0,1fr)] md:py-14"
>
	<aside>
		<div class="mb-6 flex items-center gap-2.5">
			<span
				class="grid size-8 place-items-center rounded-lg bg-primary text-[16px] font-semibold text-primary-foreground"
				>橋</span
			><span class="text-[15px] font-semibold">Set up Hashi</span>
		</div>
		{#if setup}
			<ol class="space-y-0.5">
				{#each setup.steps as s (s.n)}
					<li>
						<button
							onclick={() => (selected = s.n)}
							disabled={!isSetupSession && s.n !== 1}
							aria-current={selected === s.n ? 'step' : undefined}
							class={cn(
								'flex h-9 w-full items-center gap-3 rounded-[10px] px-2.5 text-left text-[13.5px]',
								selected === s.n && 'bg-surface-2 font-medium',
								s.status !== 'done' && selected !== s.n && 'text-muted-foreground'
							)}
						>
							<span
								class={cn(
									'tnum grid size-5 shrink-0 place-items-center rounded-full border text-[11px]',
									s.status === 'done' && 'border-ok/40 bg-ok/15 text-ok-text',
									s.status === 'failed' && 'border-bad/40 bg-bad/15 text-bad-text',
									selected === s.n && s.status !== 'done' && 'border-primary text-foreground'
								)}
							>
								{#if s.status === 'done'}<Check class="size-3" />{:else}{s.n}{/if}
							</span>
							<span class="flex-1 truncate">{s.title}</span>
							{#if !s.built}<Clock
									class="size-3.5 shrink-0"
									aria-label="Arrives in a later slice"
								/>{/if}
						</button>
					</li>
				{/each}
			</ol>
		{:else if !loadError}
			<div class="space-y-1.5">
				{#each Array(11).keys() as i (i)}<Skeleton class="h-9" />{/each}
			</div>
		{/if}
		<p class="mt-6 text-[12.5px] text-muted-foreground">
			Progress is saved after every step. You can close this page and continue later.
		</p>
	</aside>

	<main class="min-w-0">
		{#if loadError}
			{@render failure(loadError)}
		{:else if !setup || !step}
			<Skeleton class="h-6 w-24" /><Skeleton class="mt-3 h-8 w-80" /><Skeleton
				class="mt-6 h-64 rounded-3xl"
			/>
		{:else}
			<p class="text-[13px] text-muted-foreground">Step {step.n} of 11</p>

			{#if step.n === 1}
				<h1 class="mt-1 text-[24px] font-semibold">Sign in with the bootstrap login</h1>
				<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
					Hashi printed a one-time username and password in its container log on first start. It
					works only from a private network and stops working when setup is complete.
				</p>
				<section class="mt-6 rounded-3xl border bg-card p-6">
					{#if isSetupSession}
						<p class="flex items-center gap-2 text-[13.5px]">
							<Check class="size-4 text-ok-text" />Signed in from <Mono
								>{session.current?.bound_ip}</Mono
							>
						</p>
					{:else}
						<form class="grid gap-4 md:grid-cols-2" onsubmit={(e) => (e.preventDefault(), login())}>
							<div class="space-y-1.5">
								<Label for="bu">Username</Label><Input
									id="bu"
									bind:value={username}
									autocomplete="off"
									class="h-10 font-mono"
								/>
							</div>
							<div class="space-y-1.5">
								<Label for="bp">Password</Label><Input
									id="bp"
									type="password"
									bind:value={password}
									autocomplete="off"
									class="h-10 font-mono"
								/>
							</div>
							<div class="md:col-span-2">
								<p class="text-[12.5px] text-muted-foreground">
									Lost it? Run <code class="font-mono">hashi bootstrap reset</code> in the container for
									a new one.
								</p>
								<Button type="submit" class="mt-4" disabled={busy || !username || !password}
									>Sign in</Button
								>
							</div>
						</form>
					{/if}
				</section>
			{:else if step.n === 2}
				<h1 class="mt-1 text-[24px] font-semibold">Base settings</h1>
				<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
					Passkeys are bound to the root domain, so it cannot change after setup. The admin domain
					can.
				</p>
				<section class="mt-6 rounded-3xl border bg-card p-6">
					<div class="grid gap-4 md:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="rd">Root domain</Label><Input
								id="rd"
								bind:value={base.root_domain}
								placeholder="example.com"
								class="h-10"
							/>
						</div>
						<div class="space-y-1.5">
							<Label for="ad">Admin domain</Label><Input
								id="ad"
								bind:value={base.admin_domain}
								placeholder="hashi.example.com"
								class="h-10"
							/>
						</div>
						<div class="space-y-1.5">
							<Label for="iu">Internal Hashi URL</Label><Input
								id="iu"
								bind:value={base.internal_url}
								class="h-10 font-mono"
							/>
							<p class="text-[12px] text-muted-foreground">
								How Traefik reaches Hashi on your network.
							</p>
						</div>
						<div class="space-y-1.5">
							<Label for="si">Reconcile every (minutes)</Label>
							<Input
								id="si"
								type="number"
								min="1"
								max="1440"
								value={base.sync_interval_seconds / 60}
								oninput={(e) =>
									(base.sync_interval_seconds = Math.round(Number(e.currentTarget.value) * 60))}
								class="h-10"
							/>
						</div>
						<label
							class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-[13px]"
							>Public app dashboard<Switch bind:checked={base.public_dashboard_enabled} /></label
						>
						<label
							class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-[13px]"
							>Public status page<Switch bind:checked={base.public_status_enabled} /></label
						>
					</div>
					<fieldset class="mt-5">
						<legend class="text-[13px] font-medium">Theme</legend>
						<div class="mt-2 flex flex-wrap gap-2">
							{#each themes as t (t.id)}
								<button
									type="button"
									onclick={() => ((base.theme = t.id), theme.set(t.id))}
									aria-pressed={base.theme === t.id}
									class={cn(
										'rounded-[10px] border px-3 py-1.5 text-[13px]',
										base.theme === t.id && 'ring-2 ring-ring'
									)}>{t.name}</button
								>
							{/each}
						</div>
					</fieldset>
					{#if step.status === 'failed' && step.last_error && !error}{@render failure(
							step.last_error
						)}{/if}
					{#if error}{@render failure(error)}{/if}
				</section>
				<div class="mt-6 flex justify-end">
					<Button onclick={saveBase} disabled={busy}
						>{busy ? 'Checking…' : 'Save and continue'}</Button
					>
				</div>
			{:else if step.n === 9}
				<h1 class="mt-1 text-[24px] font-semibold">Passkey and recovery key</h1>
				<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
					After setup, only passkeys sign in. The recovery key restores the vault on a new host if
					the master key is lost; keep it in your password manager.
				</p>
				<section class="mt-6 rounded-3xl border bg-card p-6">
					<h2 class="text-[14px] font-semibold">1. Passkey</h2>
					{#if (setup.passkeys ?? 0) > 0}
						<p class="mt-2 flex items-center gap-2 text-[13.5px]">
							<Check class="size-4 text-ok-text" />{setup.passkeys} passkey{setup.passkeys === 1
								? ''
								: 's'} registered
						</p>
					{:else if !passkeysSupported()}
						<p class="mt-2 text-[13px] text-warn-text">
							This page is not on HTTPS, so the browser cannot create a passkey here. Open Hashi at
							its admin domain to finish this step.
						</p>
					{:else}
						<div class="mt-3 flex flex-wrap items-end gap-3">
							<div class="space-y-1.5">
								<Label for="pkn">Name</Label><Input
									id="pkn"
									bind:value={passkeyName}
									placeholder="Laptop"
									maxlength={64}
									class="h-10 w-56"
								/>
							</div>
							<Button onclick={addPasskey} disabled={busy}><Fingerprint />Create passkey</Button>
						</div>
					{/if}

					<h2 class="mt-7 text-[14px] font-semibold">2. Recovery key</h2>
					{#if step.status === 'done'}
						<p class="mt-2 flex items-center gap-2 text-[13.5px]">
							<Check class="size-4 text-ok-text" />Recovery key confirmed and tested
						</p>
					{:else if recoveryKey}
						<p class="mt-2 text-[13px] text-muted-foreground">
							Shown once. Save it now, then type its last group to confirm.
						</p>
						<div class="mt-3 flex items-start gap-2">
							<Mono class="block flex-1 rounded-lg bg-surface-2 px-3 py-2 break-all"
								><span data-testid="recovery-key">{recoveryKey}</span></Mono
							>
							<Button variant="outline" size="sm" onclick={copyKey}
								><Copy />{copied ? 'Copied' : 'Copy'}</Button
							>
						</div>
						<div class="mt-4 flex flex-wrap items-end gap-3">
							<div class="space-y-1.5">
								<Label for="cs">Last group</Label><Input
									id="cs"
									bind:value={checksum}
									maxlength={4}
									class="h-10 w-28 font-mono uppercase"
								/>
							</div>
							<Button onclick={confirmKey} disabled={busy || checksum.length !== 4}
								>Confirm and test</Button
							>
						</div>
					{:else}
						<Button
							class="mt-3"
							variant="outline"
							onclick={generateKey}
							disabled={busy || (setup.passkeys ?? 0) === 0}>Show recovery key</Button
						>
					{/if}
					{#if error}{@render failure(error)}{/if}
				</section>
			{:else if step.n === 11}
				<h1 class="mt-1 text-[24px] font-semibold">Finish setup</h1>
				<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
					Finishing deletes the bootstrap login and signs out this session. From then on you sign in
					with your passkey.
				</p>
				<section class="mt-6 rounded-3xl border bg-card p-6">
					<ul class="space-y-1.5 text-[13px]">
						{#each setup.steps.filter((s) => s.required && s.n !== 11) as s (s.n)}
							<li class="flex items-center gap-2">
								{#if s.status === 'done'}<Check class="size-4 text-ok-text" />{:else}<span
										class="size-4 text-center text-muted-foreground">·</span
									>{/if}
								<span class={s.status === 'done' ? '' : 'text-muted-foreground'}
									>{s.title}{#if !s.built}
										— arrives in slice {s.slice}{/if}</span
								>
							</li>
						{/each}
					</ul>
					{#if error}{@render failure(error)}{/if}
				</section>
				<div class="mt-6 flex justify-end">
					<Button onclick={complete} disabled={busy}>Finish setup</Button>
				</div>
			{:else}
				<h1 class="mt-1 text-[24px] font-semibold">{step.title}</h1>
				<section class="mt-6 rounded-3xl border bg-card p-6">
					<p class="flex gap-2 text-[13.5px] text-muted-foreground">
						<Clock class="mt-0.5 size-4 shrink-0" />This step arrives with slice {step.slice}. Until
						then setup cannot finish; everything you entered so far stays saved.
					</p>
				</section>
			{/if}
		{/if}
	</main>
</div>
