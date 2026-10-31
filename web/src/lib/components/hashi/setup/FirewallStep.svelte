<script lang="ts">
	// Setup step 6 (02 §1, 05 §2): connect the first firewall host. Validation only reads; confirming hands Hashi the
	// host; the first apply is a high-risk plan under the rescue timer and can wait until after setup.
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import HostForm from '$lib/components/hashi/firewall/HostForm.svelte';
	import HostSettings from '$lib/components/hashi/firewall/HostSettings.svelte';
	import { Check, CircleAlert } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { outcomeText, reviewItems, triggerText, type Plan } from '$lib/sync';
	import type { FirewallInput } from '$lib/firewall';

	type Props = {
		setup: Schemas['SetupState'];
		onupdate: (s: Schemas['SetupState']) => void;
		oncontinue: () => void;
	};
	let { setup, onupdate, oncontinue }: Props = $props();

	const host = $derived(setup.firewall);
	const step = $derived(setup.steps.find((s) => s.n === 6));
	let publicKey = $state('');
	let agreed = $state(false);
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
	async function scan(h: string, port: number) {
		let out: Schemas['HostKey'] | undefined;
		await act('scan', async () => {
			const res = await api.POST('/api/setup/firewall/scan', { body: { host: h, port } });
			if (res.error || !res.data) throw res.error;
			out = res.data;
		});
		return out;
	}
	async function install(body: Schemas['KeyInstallInput']) {
		let ok = false;
		await act('install', async () => {
			const res = await api.POST('/api/setup/firewall/install-key', { body });
			if (res.error) throw res.error;
			ok = true;
		});
		return ok;
	}
	const save = (body: FirewallInput) =>
		act('save', async () => {
			const res = await api.PUT('/api/setup/firewall/host', { body });
			if (res.error || !res.data) throw res.error;
			await reload();
		});
	let access = $state('');
	let accessSaved = $state(false);
	const saveAccess = () =>
		act('access', async () => {
			const addresses = access.split(/[\s,]+/).filter(Boolean);
			const res = await api.PUT('/api/setup/firewall/access', { body: { addresses } });
			if (res.error) throw res.error;
			accessSaved = true;
		});
	const startOver = () =>
		act('over', async () => {
			const res = await api.DELETE('/api/setup/firewall/host');
			if (res.error || !res.data) throw res.error;
			onupdate(res.data);
		});
	const confirm = () =>
		act('confirm', async () => {
			const res = await api.POST('/api/setup/firewall/confirm');
			if (res.error || !res.data) throw res.error;
			onupdate(res.data);
		});
	const planFirst = () =>
		act('plan', async () => {
			const res = await api.POST('/api/setup/firewall/plan');
			if (res.error || !res.data) throw res.error;
			plan = res.data.plan ?? null;
		});
	const run = (id: string) =>
		act('run', async () => {
			const res = await api.POST('/api/setup/firewall/plan/{id}/run', { params: { path: { id } } });
			if (res.error || !res.data) throw res.error;
			plan = res.data;
			await reload();
		});
	const usable = $derived(
		!!host &&
			(host.edge_mode === 'external_nat' || (host.health !== 'failed' && !!host.last_validated_at))
	);
</script>

<h1 class="mt-1 text-[24px] font-semibold">Connect a firewall host</h1>
<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
	The machine with a public address that forwards traffic to Traefik. Hashi keeps its rules in one
	table of its own; NetBird, Proxmox and Docker rules stay untouched.
</p>

{#if !host}
	<div class="mt-5">
		<HostForm
			traefikId={setup.traefik?.id ?? ''}
			traefikHost={setup.traefik?.host ?? ''}
			busy={!!busy}
			{scan}
			{save}
			{publicKey}
			{install}
		/>
	</div>
{:else}
	<section class="mt-5">
		<h2 class="mb-3 text-[15px] font-semibold">{host.name}</h2>
		{#if host.edge_mode === 'managed'}
			<HostSettings {host} busy={!!busy} {save} />
			{#if host.drop_other_inbound}
				<div class="mt-5 max-w-[62ch] space-y-1.5">
					<label for="fw-access" class="text-[13px] font-medium"
						>Addresses that may still reach this host</label
					>
					<input
						id="fw-access"
						bind:value={access}
						oninput={() => (accessSaved = false)}
						placeholder="203.0.113.7, 10.0.0.0/8"
						class="h-10 w-full rounded-[10px] border bg-transparent px-3 font-mono text-[12.5px]"
					/>
					<p class="text-[12px] text-muted-foreground">Its own networks and NetBird always can.</p>
					<Button size="sm" variant="outline" onclick={saveAccess} disabled={!!busy}
						>{accessSaved ? 'Saved' : 'Save addresses'}</Button
					>
				</div>
			{/if}
			{#if !host.ownership_confirmed}
				<Button class="mt-3" variant="ghost" size="sm" onclick={startOver} disabled={!!busy}
					>Start over with another address or login</Button
				>
			{/if}
		{:else}
			<p class="max-w-[62ch] text-[13px]">
				Hashi does not touch this host's firewall. Forward the public ports on your router to <Mono
					>{host.internal_traefik_ip}</Mono
				>.
			</p>
		{/if}
	</section>
	{#if usable}
		<section class="mt-8">
			<h2 class="text-[15px] font-semibold">Hand over the host</h2>
			{#if host.edge_mode === 'external_nat' || host.ownership_confirmed}
				{#if step?.status === 'done'}<p class="mt-2 flex items-center gap-2 text-[13.5px]">
						<Check class="size-4 text-ok-text" />Confirmed.
					</p>
				{:else}<Button class="mt-3" onclick={confirm} disabled={!!busy}>Confirm</Button>{/if}
			{:else}
				<label class="mt-2 flex max-w-[70ch] items-start gap-2.5 text-[13px]">
					<input type="checkbox" bind:checked={agreed} class="mt-0.5 accent-primary" />
					<span
						>Hashi may manage its own firewall table on {host.name}. Nothing changes on the host
						until you approve a plan.</span
					>
				</label>
				<Button class="mt-3" onclick={confirm} disabled={!agreed || !!busy}>Confirm</Button>
			{/if}
		</section>
	{/if}
	{#if step?.status === 'done' && host.edge_mode === 'managed'}
		<section class="mt-8">
			<h2 class="text-[15px] font-semibold">First apply</h2>
			<p class="mt-1 mb-3 max-w-[70ch] text-[13px] text-muted-foreground">
				Optional here. If Hashi cannot log in again over a new connection within 3 minutes, the host
				goes back to how it was.
			</p>
			{#if plan}
				<PlanReview
					title={plan.title}
					trigger={triggerText(plan)}
					risk={plan.risk}
					items={reviewItems(plan)}
				>
					{#snippet footer()}
						{#if plan?.status === 'pending' || plan?.status === 'approved'}
							<Button size="sm" onclick={() => plan && run(plan.id)} disabled={!!busy}
								>{busy === 'run' ? 'Applying…' : 'Approve and run'}</Button
							>
						{:else if plan}
							<p class="mr-auto text-[12.5px] text-muted-foreground">{outcomeText(plan)}</p>
						{/if}
					{/snippet}
				</PlanReview>
			{:else if host.last_applied_at}
				<p class="flex items-center gap-2 text-[13.5px]">
					<Check class="size-4 text-ok-text" />Applied and confirmed.
				</p>
			{:else}
				<Button variant="outline" onclick={planFirst} disabled={!!busy}
					>{busy === 'plan' ? 'Planning…' : 'Plan the first apply'}</Button
				>
			{/if}
		</section>
	{/if}
{/if}
{#if error}
	<p
		role="alert"
		class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{error}
	</p>
{/if}
<div class="mt-6 flex justify-end">
	<Button onclick={oncontinue} disabled={step?.status !== 'done'}>Continue</Button>
</div>
