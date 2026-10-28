<script lang="ts">
	// Setup step 4 (02 §1.4, 03 §5.3): Hashi is the ACME client. The account registers here; the first certificate can
	// be issued right away (DNS-01 through the zone from step 3).
	import { untrack } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { cn } from '$lib/utils';
	import { Check, CircleAlert } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { outcomeText, reviewItems, triggerText, type Plan } from '$lib/sync';

	type Props = {
		setup: Schemas['SetupState'];
		onupdate: (s: Schemas['SetupState']) => void;
		oncontinue: () => void;
	};
	let { setup, onupdate, oncontinue }: Props = $props();

	const root = $derived(setup.base_settings?.root_domain ?? '');
	const provider = $derived(setup.certificates);
	const step = $derived(setup.steps.find((s) => s.n === 4));

	const cas = [
		{
			id: 'gts',
			name: 'Google Trust Services',
			hint: 'Needs an External Account Binding key from Google Cloud.'
		},
		{ id: 'letsencrypt', name: "Let's Encrypt", hint: 'No key needed.' },
		{ id: 'custom', name: 'Other ACME CA', hint: 'Any ACME directory URL.' }
	] as const;

	// Starting values only: the form keeps what the admin types while setup reloads.
	let form = $state(
		untrack(() => ({
			directory: (setup.certificates?.directory ??
				'gts') as Schemas['CertificateProviderInput']['directory'],
			directory_url:
				setup.certificates?.directory === 'custom' ? setup.certificates.directory_url : '',
			email: setup.certificates?.email ?? '',
			eab_kid: setup.certificates?.eab_kid ?? '',
			eab_hmac: '',
			propagation_delay_s: setup.certificates?.propagation_delay_s ?? 0,
			resolvers: (setup.certificates?.resolvers ?? []).join(', ')
		}))
	);
	let editing = $state(untrack(() => !setup.certificates?.account_registered));
	let busy = $state(false);
	let error = $state('');
	let plan = $state<Plan | null>(null);
	let nothingNeeded = $state(false);

	async function reload() {
		const st = await api.GET('/api/setup');
		if (st.error || !st.data) throw st.error;
		onupdate(st.data);
	}

	async function act(fn: () => Promise<void>) {
		error = '';
		busy = true;
		try {
			await fn();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}

	const register = () =>
		act(async () => {
			const res = await api.PUT('/api/setup/certificates', {
				body: {
					directory: form.directory,
					directory_url: form.directory === 'custom' ? form.directory_url : undefined,
					email: form.email.trim(),
					eab_kid: form.eab_kid.trim() || undefined,
					eab_hmac: form.eab_hmac || undefined,
					propagation_delay_s: Number(form.propagation_delay_s) || 0,
					resolvers: form.resolvers
						.split(/[\s,]+/)
						.map((r) => r.trim())
						.filter(Boolean)
				}
			});
			if (res.error || !res.data) throw res.error;
			form.eab_hmac = '';
			if (res.data.account_registered) editing = false;
			await reload();
		});

	const issue = () =>
		act(async () => {
			const res = await api.POST('/api/setup/certificates/issue');
			if (res.error || !res.data) throw res.error;
			plan = res.data.plan ?? null;
			nothingNeeded = !res.data.plan;
		});

	const needsKey = $derived(form.directory === 'gts');
	const canRegister = $derived(
		!!form.email.trim() &&
			(form.directory !== 'custom' || !!form.directory_url.trim()) &&
			(!needsKey || (!!form.eab_kid.trim() && (!!form.eab_hmac || !!provider?.has_eab_hmac)))
	);
</script>

{#snippet failure(msg: string)}
	<p
		role="alert"
		class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{msg}
	</p>
{/snippet}

<h1 class="mt-1 text-[24px] font-semibold">Certificates</h1>
<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
	Hashi requests one certificate for <Mono>{root}</Mono> and <Mono>*.{root}</Mono> and proves it owns
	the domain with a temporary DNS record in your zone. Your Traefik hosts get the files from Hashi; they
	never talk to the certificate authority.
</p>

<section class="mt-6 rounded-3xl border bg-card p-6">
	{#if provider?.account_registered && !editing}
		<div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-[13.5px]">
			<span class="flex items-center gap-2"
				><Check class="size-4 text-ok-text" />Account registered with {cas.find(
					(c) => c.id === provider.directory
				)?.name}</span
			>
			<span class="text-muted-foreground">{provider.email}</span>
			<Button variant="ghost" size="sm" class="ml-auto" onclick={() => (editing = true)}
				>Change</Button
			>
		</div>
	{:else}
		<form class="grid gap-4 md:grid-cols-2" onsubmit={(e) => (e.preventDefault(), register())}>
			<fieldset class="md:col-span-2">
				<legend class="text-[13px] font-medium">Certificate authority</legend>
				<div class="mt-2 grid gap-1.5 sm:grid-cols-3">
					{#each cas as c (c.id)}
						<label
							class={cn(
								'flex cursor-pointer flex-col gap-0.5 rounded-[10px] border px-3 py-2.5 text-[13px]',
								form.directory === c.id && 'border-primary bg-primary/[0.06]'
							)}
						>
							<span class="flex items-center gap-2"
								><input
									type="radio"
									name="ca"
									value={c.id}
									bind:group={form.directory}
									class="accent-primary"
								/>{c.name}</span
							>
							<span class="text-[12px] text-muted-foreground">{c.hint}</span>
						</label>
					{/each}
				</div>
			</fieldset>
			{#if form.directory === 'custom'}
				<div class="space-y-1.5 md:col-span-2">
					<Label for="cdir">ACME directory URL</Label><Input
						id="cdir"
						bind:value={form.directory_url}
						placeholder="https://ca.example.net/directory"
						class="h-10 font-mono"
					/>
				</div>
			{/if}
			<div class="space-y-1.5">
				<Label for="cem">Contact email</Label><Input
					id="cem"
					type="email"
					bind:value={form.email}
					placeholder="you@example.com"
					class="h-10"
				/>
				<p class="text-[12px] text-muted-foreground">
					The CA writes here about expiring certificates.
				</p>
			</div>
			<div></div>
			{#if form.directory !== 'letsencrypt'}
				<div class="space-y-1.5">
					<Label for="ckid">EAB key ID</Label><Input
						id="ckid"
						bind:value={form.eab_kid}
						autocomplete="off"
						class="h-10 font-mono"
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="chmac">EAB HMAC key</Label><Input
						id="chmac"
						type="password"
						bind:value={form.eab_hmac}
						autocomplete="off"
						placeholder={provider?.has_eab_hmac && form.eab_kid === provider.eab_kid
							? 'Leave empty to keep the stored key'
							: ''}
						class="h-10 font-mono"
					/>
				</div>
				<p class="text-[12.5px] text-muted-foreground md:col-span-2">
					The key pair links Hashi's account to yours at the CA. It is stored encrypted and never
					shown again.
				</p>
			{/if}
			<details class="md:col-span-2">
				<summary class="cursor-pointer text-[13px] font-medium">DNS check</summary>
				<div class="mt-3 grid gap-4 md:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="cres">DNS servers to check</Label><Input
							id="cres"
							bind:value={form.resolvers}
							placeholder="Your zone's own name servers"
							class="h-10 font-mono"
						/>
						<p class="text-[12px] text-muted-foreground">
							Hashi waits until the temporary record shows up here before asking the CA.
						</p>
					</div>
					<div class="space-y-1.5">
						<Label for="cdelay">Extra wait (s)</Label><Input
							id="cdelay"
							type="number"
							min="0"
							max="600"
							bind:value={form.propagation_delay_s}
							class="h-10 w-32"
						/>
					</div>
				</div>
			</details>
			<div class="flex gap-2 md:col-span-2">
				<Button type="submit" disabled={busy || !canRegister}
					>{busy ? 'Registering…' : 'Register account'}</Button
				>
				{#if provider?.account_registered}<Button variant="ghost" onclick={() => (editing = false)}
						>Cancel</Button
					>{/if}
			</div>
		</form>
	{/if}
	{#if provider && !provider.account_registered && provider.last_error && !error}{@render failure(
			provider.last_error
		)}{/if}
	{#if error}{@render failure(error)}{/if}
</section>

{#if provider?.account_registered}
	<section class="mt-8">
		<h2 class="text-[15px] font-semibold">First certificate</h2>
		<p class="mt-1 mb-3 max-w-[70ch] text-[13px] text-muted-foreground">
			Issuing now takes about a minute. Otherwise Hashi issues it in its daily certificate run. It
			goes to your Traefik host once that host is connected.
		</p>
		{#if plan}
			<PlanReview
				title={plan.title}
				trigger={triggerText(plan)}
				risk={plan.risk}
				items={reviewItems(plan)}
			>
				{#snippet badge()}
					{#if plan?.status === 'applied'}<Tag tone="ok">Issued</Tag
						>{:else if plan?.status === 'failed'}<Tag tone="bad">Failed</Tag>{/if}
				{/snippet}
				{#snippet footer()}
					<p class="mr-auto text-[12.5px] text-muted-foreground">{plan && outcomeText(plan)}</p>
					{#if plan?.status === 'failed'}<Button
							size="sm"
							variant="outline"
							onclick={issue}
							disabled={busy}>Try again</Button
						>{/if}
				{/snippet}
			</PlanReview>
		{:else if nothingNeeded}
			<p class="flex items-center gap-2 text-[13.5px]">
				<Check class="size-4 text-ok-text" />The certificate is already issued and valid.
			</p>
		{:else}
			<Button variant="outline" onclick={issue} disabled={busy}
				>{busy ? 'Issuing…' : 'Issue the certificate now'}</Button
			>
		{/if}
	</section>
{/if}

<div class="mt-6 flex justify-end">
	<Button onclick={oncontinue} disabled={step?.status !== 'done'}>Continue</Button>
</div>
