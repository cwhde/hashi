<script lang="ts">
	// Setup step 7 (02 §1.6): the Hashi admin service. Hashi creates it from the base settings, issues a certificate that
	// covers the admin domain when it needs a new wildcard, and shows the Traefik plan before anything is written.
	// DNS and firewall parts of this step arrive with firewall hosts (S6).
	import { untrack } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Check, CircleAlert } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { outcomeText, reviewItems, triggerText, type Plan } from '$lib/sync';
	import { targetText } from '$lib/services';

	type Props = {
		setup: Schemas['SetupState'];
		onupdate: (s: Schemas['SetupState']) => void;
		oncontinue: () => void;
	};
	let { setup, onupdate, oncontinue }: Props = $props();

	const step = $derived(setup.steps.find((s) => s.n === 7));
	const svc = $derived(setup.system_service);
	const admin = $derived(setup.base_settings?.admin_domain ?? '');
	let certPlan = $state<Plan | null>(null);
	let plan = $state<Plan | null>(null);
	let busy = $state('');
	let error = $state(untrack(() => (step?.status === 'failed' ? (step.last_error ?? '') : '')));

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

	const publish = () =>
		act('publish', async () => {
			const res = await api.POST('/api/setup/system-service');
			if (res.error || !res.data) throw res.error;
			certPlan = res.data.certificate_plan ?? null;
			plan = res.data.traefik_plan ?? null;
			onupdate(res.data.state);
			if (certPlan?.status === 'failed') error = outcomeText(certPlan);
		});

	const run = (id: string) =>
		act('run', async () => {
			const res = await api.POST('/api/setup/system-service/plan/{id}/run', {
				params: { path: { id } }
			});
			if (res.error || !res.data) throw res.error;
			plan = res.data.traefik_plan ?? null;
			onupdate(res.data.state);
			const st = res.data.state.steps.find((s) => s.n === 7);
			if (st?.status === 'failed' && st.last_error) error = st.last_error;
		});
</script>

<h1 class="mt-1 text-[24px] font-semibold">Publish Hashi</h1>
<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
	Hashi adds itself as the first service: <Mono>{admin}</Mono> on every Traefik host, forwarded to its
	internal address, with a certificate that covers the name.
</p>

<section class="mt-6 rounded-3xl border bg-card p-6">
	<ul class="space-y-2 text-[13.5px]">
		<li class="flex items-center gap-2">
			{#if svc}<Check class="size-4 text-ok-text" />{:else}<span class="size-4 rounded-full border"
				></span>{/if}
			Service <Mono>https://{admin}</Mono>
			{#if svc}→ <Mono>{svc.kind}://{targetText(svc.target)}</Mono>{/if}
		</li>
		{#if certPlan}
			<li class="flex items-center gap-2">
				{#if certPlan.status === 'applied'}<Check class="size-4 text-ok-text" />{:else}<CircleAlert
						class="size-4 text-bad-text"
					/>{/if}
				Certificate: {outcomeText(certPlan)}
			</li>
		{/if}
		{#if step?.status === 'done'}
			<li class="flex items-center gap-2">
				<Check class="size-4 text-ok-text" />The Traefik host serves {admin} with Hashi's certificate.
			</li>
		{/if}
	</ul>
	{#if !plan && step?.status !== 'done'}
		<p class="mt-4 max-w-[62ch] text-[12.5px] text-muted-foreground">
			A name a level deeper than the root (like <Mono>dev.hashi.example.com</Mono>) needs a new
			wildcard certificate first; that takes about a minute.
		</p>
		<Button class="mt-4" onclick={publish} disabled={!!busy}
			>{busy === 'publish' ? 'Preparing… (up to a minute)' : 'Prepare'}</Button
		>
	{:else if step?.status === 'done' && !plan}
		<Button class="mt-4" variant="outline" onclick={publish} disabled={!!busy}>Check again</Button>
	{/if}
	{#if error}
		<p
			role="alert"
			class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
		>
			<CircleAlert class="mt-0.5 size-4 shrink-0" />{error}
		</p>
	{/if}
</section>

{#if plan}
	<section class="mt-6">
		<PlanReview
			title={plan.title}
			trigger={triggerText(plan)}
			risk={plan.risk}
			items={reviewItems(plan)}
		>
			{#snippet badge()}
				{#if plan?.status === 'applied'}<Tag tone="ok">Applied and verified</Tag
					>{:else if plan?.status === 'failed'}<Tag tone="bad">Failed</Tag>{/if}
			{/snippet}
			{#snippet footer()}
				{#if plan?.status === 'pending' || plan?.status === 'approved'}
					<p class="mr-auto text-[12.5px] text-muted-foreground">
						Approving writes these files to the Traefik host. Traefik picks them up without a
						restart.
					</p>
					<Button size="sm" disabled={!!busy} onclick={() => plan && run(plan.id)}
						>{busy === 'run' ? 'Publishing…' : 'Approve and run'}</Button
					>
				{:else if plan}
					<p class="mr-auto text-[12.5px] text-muted-foreground">{outcomeText(plan)}</p>
					{#if plan.status === 'failed'}<Button size="sm" variant="outline" onclick={publish}
							>Plan again</Button
						>{/if}
				{/if}
			{/snippet}
		</PlanReview>
	</section>
{/if}

<div class="mt-6 flex justify-end">
	<Button onclick={oncontinue} disabled={step?.status !== 'done'}>Continue</Button>
</div>
