<script lang="ts">
	// Setup step 5 (02 §1.5, 04 §2/§5): connect the edge over SSH. The host key is shown and pinned only after the admin
	// compares it; validation reads the host; taking over is an explicit confirmation; the install is a plan.
	import { onMount, untrack } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { cn } from '$lib/utils';
	import { Check, CircleAlert, Copy, ShieldCheck } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { outcomeText, reviewItems, triggerText, type Plan } from '$lib/sync';

	type Props = {
		setup: Schemas['SetupState'];
		onupdate: (s: Schemas['SetupState']) => void;
		oncontinue: () => void;
	};
	let { setup, onupdate, oncontinue }: Props = $props();

	type Auth = Schemas['TraefikConnectionInput']['auth'];
	const conn = $derived(setup.traefik);
	const step = $derived(setup.steps.find((s) => s.n === 5));

	// Starting values only: the form keeps what the admin types while setup reloads.
	let form = $state(
		untrack(() => ({
			name: setup.traefik?.name ?? 'edge',
			host: setup.traefik?.host ?? '',
			port: setup.traefik?.port ?? 22,
			user: setup.traefik?.user ?? 'root',
			auth: (setup.traefik?.auth ?? 'hashi_key') as Auth,
			credential: '',
			passphrase: ''
		}))
	);
	let publicKey = $state('');
	let copied = $state(false);
	let scanned = $state<Schemas['HostKey'] | null>(null);
	let rejected = $state(false);
	let editing = $state(untrack(() => !setup.traefik));
	let takeover = $state(false);
	let plan = $state<Plan | null>(null);
	let busy = $state('');
	let error = $state('');

	onMount(async () => {
		const res = await api.POST('/api/setup/traefik/identity');
		if (res.data) publicKey = res.data.public_key;
	});

	async function act(what: string, fn: () => Promise<void>) {
		error = '';
		busy = what;
		try {
			await fn();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = '';
		}
	}
	async function reload() {
		const st = await api.GET('/api/setup');
		if (st.error || !st.data) throw st.error;
		onupdate(st.data);
	}

	const scan = () =>
		act('scan', async () => {
			rejected = false;
			const res = await api.POST('/api/setup/traefik/scan', {
				body: { host: form.host.trim(), port: Number(form.port) }
			});
			if (res.error || !res.data) throw res.error;
			scanned = res.data;
		});

	const pin = () =>
		act('pin', async () => {
			if (!scanned) return;
			const res = await api.PUT('/api/setup/traefik/connection', {
				body: {
					name: form.name.trim(),
					host: form.host.trim(),
					port: Number(form.port),
					user: form.user.trim(),
					auth: form.auth,
					credential: form.auth === 'hashi_key' ? undefined : form.credential || undefined,
					passphrase: form.auth === 'private_key' ? form.passphrase || undefined : undefined,
					host_key: scanned.key
				}
			});
			if (res.error || !res.data) throw res.error;
			form.credential = '';
			form.passphrase = '';
			scanned = null;
			editing = false;
			await reload();
		});

	const confirm = () =>
		act('confirm', async () => {
			const res = await api.POST('/api/setup/traefik/confirm');
			if (res.error || !res.data) throw res.error;
			onupdate(res.data);
		});

	const planInstall = () =>
		act('plan', async () => {
			const res = await api.POST('/api/setup/traefik/plan');
			if (res.error || !res.data) throw res.error;
			plan = res.data.plan ?? null;
			if (!plan) await reload();
		});

	const runPlan = (id: string) =>
		act('run', async () => {
			const res = await api.POST('/api/setup/traefik/plan/{id}/run', { params: { path: { id } } });
			if (res.error || !res.data) throw res.error;
			plan = res.data;
			await reload();
		});

	async function copyKey() {
		await navigator.clipboard.writeText(publicKey);
		copied = true;
	}

	const credentialLabel = $derived(form.auth === 'password' ? 'Password' : 'Private key');
	const canScan = $derived(
		!!form.host.trim() &&
			!!form.user.trim() &&
			!!form.name.trim() &&
			(form.auth === 'hashi_key' || !!form.credential || conn?.auth === form.auth)
	);
	const validated = $derived(conn?.health === 'healthy' && !!conn.os_family);
	const installed = $derived(!!conn?.installed_version);
</script>

{#snippet failure(msg: string)}
	<p
		role="alert"
		class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{msg}
	</p>
{/snippet}

<h1 class="mt-1 text-[24px] font-semibold">Connect a Traefik host</h1>
<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
	Hashi installs and runs its own Traefik on this machine over SSH. Existing Traefik config is
	backed up and only replaced after you confirm.
</p>

<section class="mt-6 rounded-3xl border bg-card p-6">
	{#if conn && !editing}
		<div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-[13.5px]">
			<span class="flex items-center gap-2"
				><Check class="size-4 text-ok-text" />{conn.name} · <Mono
					>{conn.user}@{conn.host}:{conn.port}</Mono
				></span
			>
			<span class="text-muted-foreground"
				>{conn.host_key_type} <Mono>{conn.host_key_fingerprint}</Mono></span
			>
			<Button variant="ghost" size="sm" class="ml-auto" onclick={() => (editing = true)}
				>Change</Button
			>
		</div>
	{:else}
		<form class="grid gap-4 md:grid-cols-2" onsubmit={(e) => (e.preventDefault(), scan())}>
			<div class="space-y-1.5">
				<Label for="th">Host or IP</Label><Input
					id="th"
					bind:value={form.host}
					placeholder="10.0.0.11"
					class="h-10 font-mono"
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="tp">SSH port</Label><Input
					id="tp"
					type="number"
					bind:value={form.port}
					class="h-10"
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="tu">User</Label><Input id="tu" bind:value={form.user} class="h-10" />
				<p class="text-[12px] text-muted-foreground">
					Hashi installs a service, so it logs in as root.
				</p>
			</div>
			<div class="space-y-1.5">
				<Label for="tn">Name in Hashi</Label><Input id="tn" bind:value={form.name} class="h-10" />
			</div>
			<fieldset class="md:col-span-2">
				<legend class="text-[13px] font-medium">How Hashi logs in</legend>
				<div class="mt-2 grid gap-1.5 sm:grid-cols-3">
					{#each [['hashi_key', "Hashi's own key"], ['private_key', 'Your private key'], ['password', 'Password']] as [id, label] (id)}
						<label
							class={cn(
								'flex h-10 cursor-pointer items-center gap-2.5 rounded-[10px] border px-3 text-[13px]',
								form.auth === id && 'border-primary bg-primary/[0.06]'
							)}
							><input
								type="radio"
								name="auth"
								value={id}
								bind:group={form.auth}
								class="accent-primary"
							/>{label}</label
						>
					{/each}
				</div>
			</fieldset>
			{#if form.auth === 'hashi_key'}
				<div class="md:col-span-2">
					<p class="text-[12.5px] text-muted-foreground">
						Add this line to <Mono>/root/.ssh/authorized_keys</Mono> on the host, then continue.
					</p>
					<div class="mt-2 flex items-start gap-2">
						<Mono class="block flex-1 rounded-lg bg-surface-2 px-3 py-2 break-all"
							><span data-testid="hashi-public-key">{publicKey || '…'}</span></Mono
						>
						<Button variant="outline" size="sm" onclick={copyKey} disabled={!publicKey}
							><Copy />{copied ? 'Copied' : 'Copy'}</Button
						>
					</div>
				</div>
			{:else}
				<div class="space-y-1.5 md:col-span-2">
					<Label for="tc">{credentialLabel}</Label>
					{#if form.auth === 'private_key'}
						<textarea
							id="tc"
							bind:value={form.credential}
							rows="4"
							autocomplete="off"
							placeholder={conn?.auth === 'private_key' ? 'Leave empty to keep the stored key' : ''}
							class="w-full rounded-[10px] border bg-transparent px-3 py-2 font-mono text-[12.5px]"
						></textarea>
					{:else}
						<Input
							id="tc"
							type="password"
							bind:value={form.credential}
							autocomplete="off"
							placeholder={conn?.auth === 'password'
								? 'Leave empty to keep the stored password'
								: ''}
							class="h-10"
						/>
					{/if}
					<p class="text-[12px] text-muted-foreground">Stored encrypted, never shown again.</p>
				</div>
				{#if form.auth === 'private_key'}
					<div class="space-y-1.5">
						<Label for="tpp">Passphrase (if any)</Label><Input
							id="tpp"
							type="password"
							bind:value={form.passphrase}
							autocomplete="off"
							class="h-10"
						/>
					</div>
				{/if}
			{/if}
			<div class="flex gap-2 md:col-span-2">
				<Button type="submit" disabled={!!busy || !canScan}
					>{busy === 'scan' ? 'Connecting…' : 'Read the host key'}</Button
				>
				{#if conn}<Button variant="ghost" onclick={() => ((editing = false), (scanned = null))}
						>Cancel</Button
					>{/if}
			</div>
		</form>

		{#if scanned}
			<div class="mt-5 rounded-2xl border border-primary/40 bg-primary/[0.06] p-4">
				<div class="flex items-center gap-2 text-[13.5px] font-medium">
					<ShieldCheck class="size-4" />Confirm this host key
				</div>
				<p class="mt-1 text-[13px] text-muted-foreground">
					First contact with {form.host}. Compare it with <Mono
						>ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub</Mono
					> on the host.
				</p>
				<Mono class="mt-3 block rounded-lg bg-surface-2 px-3 py-2"
					><span data-testid="host-key">{scanned.type} {scanned.fingerprint}</span></Mono
				>
				<div class="mt-3 flex gap-2">
					<Button size="sm" onclick={pin} disabled={!!busy}
						>{busy === 'pin' ? 'Checking the host…' : 'It matches, pin it'}</Button
					><Button size="sm" variant="ghost" onclick={() => ((scanned = null), (rejected = true))}
						>It doesn't match</Button
					>
				</div>
			</div>
		{:else if rejected}
			{@render failure(
				'Connection stopped. A different key can mean the wrong machine or an interception. Check the address and try again.'
			)}
		{/if}
	{/if}

	{#if conn}
		<ul class="mt-5 space-y-1.5 text-[13px]" aria-label="Checks">
			{#if conn.health === 'failed'}
				<li class="flex items-start gap-2 text-bad-text">
					<CircleAlert class="mt-0.5 size-4 shrink-0" />{conn.last_error}
				</li>
			{:else if validated}
				<li class="flex items-center gap-2">
					<Check class="size-4 text-ok-text" />SSH login as {conn.user}
				</li>
				<li class="flex items-center gap-2">
					<Check class="size-4 text-ok-text" />{conn.os_name}, {conn.package_manager}, {conn.init_system},
					{conn.arch}
				</li>
				<li class="flex items-center gap-2">
					<Check class="size-4 text-ok-text" />Hashi can write in <Mono>/var/lib/hashi</Mono>
				</li>
				<li class="flex items-center gap-2">
					<Check
						class="size-4 text-ok-text"
					/>{#if conn.existing_binary || conn.existing_configs.length}
						Existing Traefik: {[conn.existing_binary, ...conn.existing_configs]
							.filter(Boolean)
							.join(', ')}
					{:else}No Traefik found on the host{/if}
				</li>
			{/if}
		</ul>
	{/if}
	{#if error}{@render failure(error)}{/if}
</section>

{#if conn && validated && !editing}
	<section class="mt-8">
		<h2 class="text-[15px] font-semibold">Take over the host</h2>
		{#if conn.ownership_confirmed}
			<p class="mt-2 flex items-center gap-2 text-[13.5px]">
				<Check class="size-4 text-ok-text" />Confirmed. Hashi manages Traefik on {conn.name}.
			</p>
		{:else}
			<label class="mt-2 flex max-w-[70ch] items-start gap-2.5 text-[13px]">
				<input type="checkbox" bind:checked={takeover} class="mt-0.5 accent-primary" />
				<span>
					Hashi may run its own Traefik service on {conn.name}.
					{#if conn.existing_configs.length}The config in <Mono
							>{conn.existing_configs.join(', ')}</Mono
						> is copied to <Mono>/var/lib/hashi/traefik/backup</Mono> first.{/if}
					Nothing changes on the host until you approve the install plan.
				</span>
			</label>
			<Button class="mt-3" onclick={confirm} disabled={!takeover || !!busy}>Confirm</Button>
		{/if}
	</section>

	{#if conn.ownership_confirmed}
		<section class="mt-8">
			<h2 class="text-[15px] font-semibold">Install</h2>
			<p class="mt-1 mb-3 max-w-[70ch] text-[13px] text-muted-foreground">
				Traefik {installed
					? conn.installed_version + ' runs on the host.'
					: 'is downloaded, checked against its published checksum and installed with Hashi’s files.'}
				The plan lists every file before anything is written.
			</p>
			{#if plan}
				<PlanReview
					title={plan.title}
					trigger={triggerText(plan)}
					risk={plan.risk}
					items={reviewItems(plan)}
				>
					{#snippet badge()}
						{#if plan?.status === 'applied'}<Tag tone="ok">Installed and verified</Tag
							>{:else if plan?.status === 'failed'}<Tag tone="bad">Failed</Tag>{/if}
					{/snippet}
					{#snippet footer()}
						{#if plan?.status === 'pending' || plan?.status === 'approved'}
							<p class="mr-auto text-[12.5px] text-muted-foreground">
								Approving writes these changes to {conn?.name} and restarts Traefik there.
							</p>
							<Button variant="ghost" size="sm" onclick={() => (plan = null)}>Not now</Button>
							<Button size="sm" disabled={!!busy} onclick={() => plan && runPlan(plan.id)}
								>{busy === 'run' ? 'Installing…' : 'Approve and run'}</Button
							>
						{:else if plan}
							<p class="mr-auto text-[12.5px] text-muted-foreground">{outcomeText(plan)}</p>
							{#if plan.status === 'failed'}<Button
									size="sm"
									variant="outline"
									onclick={planInstall}>Plan again</Button
								>{/if}
						{/if}
					{/snippet}
				</PlanReview>
			{:else if installed}
				<p class="flex items-center gap-2 text-[13.5px]">
					<Check class="size-4 text-ok-text" />Hashi's Traefik runs on {conn.name} and loaded every file.
				</p>
				<Button class="mt-3" variant="outline" onclick={planInstall} disabled={!!busy}
					>Check again</Button
				>
			{:else}
				<Button variant="outline" onclick={planInstall} disabled={!!busy}
					>{busy === 'plan' ? 'Planning…' : 'Plan the install'}</Button
				>
			{/if}
		</section>
	{/if}
{/if}

<div class="mt-6 flex justify-end">
	<Button onclick={oncontinue} disabled={step?.status !== 'done'}>Continue</Button>
</div>
