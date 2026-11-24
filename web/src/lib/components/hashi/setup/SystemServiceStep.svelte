<script lang="ts">
	// Setup step 7 (02 §1.6): the Hashi admin service. Hashi creates it from the base settings, issues a certificate that
	// covers the admin domain when it needs a new wildcard, and shows the Traefik plan before anything is written.
	// DNS and firewall parts of this step arrive with firewall hosts (S6).
	import { untrack } from 'svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import PlanReview from '#lib/components/hashi/PlanReview.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import { Check, CircleAlert } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { outcomeText, reviewItems, triggerText, type Plan } from '#lib/sync.ts';
	import { targetText } from '#lib/services.ts';

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
	let fwPlan = $state<Plan | null>(null);
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
			fwPlan = res.data.firewall_plan ?? null;
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
			fwPlan = res.data.firewall_plan ?? null;
			onupdate(res.data.state);
			const st = res.data.state.steps.find((s) => s.n === 7);
			if (st?.status === 'failed' && st.last_error) error = st.last_error;
		});

	// Opening the ports is high risk and guarded by the rescue timer; afterwards the step checks everything again.
	const runFirewall = (id: string) =>
		act('firewall', async () => {
			const res = await api.POST('/api/setup/firewall/plan/{id}/run', { params: { path: { id } } });
			if (res.error || !res.data) throw res.error;
			fwPlan = res.data;
			if (res.data.status !== 'applied') return;
			const again = await api.POST('/api/setup/system-service');
			if (again.error || !again.data) throw again.error;
			fwPlan = again.data.firewall_plan ?? null;
			onupdate(again.data.state);
		});
</script>

<h1 class="mt-1 text-title font-semibold">Publish Hashi</h1>

<section class="mt-6 rounded-3xl border bg-card p-6">
	<ul class="space-y-2 text-md">
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
	{#if !plan && !fwPlan && step?.status !== 'done'}
		<Button class="mt-4" onclick={publish} disabled={!!busy}
			>{busy === 'publish' ? 'Preparing… (up to a minute)' : 'Prepare'}</Button
		>
	{:else if step?.status === 'done' && !plan}
		<Button class="mt-4" variant="outline" onclick={publish} disabled={!!busy}>Check again</Button>
	{/if}
	{#if error}
		<p
			role="alert"
			class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-md text-bad-text"
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
					<Button size="sm" disabled={!!busy} onclick={() => plan && run(plan.id)}
						>{busy === 'run' ? 'Publishing…' : 'Approve and run'}</Button
					>
				{:else if plan}
					<p class="mr-auto text-sm text-muted-foreground">{outcomeText(plan)}</p>
					{#if plan.status === 'failed'}<Button size="sm" variant="outline" onclick={publish}
							>Plan again</Button
						>{/if}
				{/if}
			{/snippet}
		</PlanReview>
	</section>
{/if}

{#if fwPlan}
	<section class="mt-6">
		<PlanReview
			title={fwPlan.title}
			trigger={triggerText(fwPlan)}
			risk={fwPlan.risk}
			items={reviewItems(fwPlan)}
		>
			{#snippet footer()}
				{#if fwPlan?.status === 'pending' || fwPlan?.status === 'approved'}
					<p class="mr-auto text-sm text-muted-foreground">
						Rolls back by itself if Hashi can't log in again within 3 minutes.
					</p>
					<Button size="sm" disabled={!!busy} onclick={() => fwPlan && runFirewall(fwPlan.id)}
						>{busy === 'firewall' ? 'Applying…' : 'Approve and run'}</Button
					>
				{:else if fwPlan}
					<p class="mr-auto text-sm text-muted-foreground">{outcomeText(fwPlan)}</p>
					<Button size="sm" variant="outline" onclick={publish} disabled={!!busy}>Plan again</Button
					>
				{/if}
			{/snippet}
		</PlanReview>
	</section>
{/if}

<div class="mt-6 flex justify-end">
	<Button onclick={oncontinue} disabled={step?.status !== 'done'}>Continue</Button>
</div>
