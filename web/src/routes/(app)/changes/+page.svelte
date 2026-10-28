<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { CircleAlert, Fingerprint, LoaderCircle, Play } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { pendingPlans } from '$lib/sync-pending.svelte';
	import { fmtTime, outcomeText, reviewItems, triggerText, type Plan } from '$lib/sync';

	// 01-platform.md §5, 10b §3: every waiting plan with exact before/after; high risk needs the passkey.
	let open = $state<Plan[]>([]);
	let recent = $state<Plan[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let notice = $state('');
	let busy = $state('');

	async function load() {
		const res = await api.GET('/api/sync/plans');
		if (res.error || !res.data) throw res.error;
		open = res.data.open;
		recent = res.data.recent;
		loaded = true;
		pendingPlans.count = open.filter((p) => p.status === 'pending').length;
	}
	onMount(() => load().catch((e) => (error = problemText(e))));

	async function act(p: Plan, what: 'approve' | 'apply' | 'discard') {
		error = notice = '';
		busy = p.id;
		try {
			let ran: Plan;
			if (what === 'discard') {
				ran = await withReauth(() =>
					api.POST('/api/sync/plans/{id}/discard', { params: { path: { id: p.id } } })
				);
			} else {
				if (what === 'approve') {
					await withReauth(() =>
						api.POST('/api/sync/plans/{id}/approve', { params: { path: { id: p.id } } })
					);
				}
				ran = await withReauth(() =>
					api.POST('/api/sync/plans/{id}/apply', { params: { path: { id: p.id } } })
				);
			}
			notice =
				ran.id !== p.id && ran.status === 'pending'
					? 'The provider changed since this plan was made. Review the new plan below.'
					: `${ran.title}: ${outcomeText(ran)}`;
			await load();
		} catch (e) {
			error = problemText(e);
			await load().catch(() => {});
		} finally {
			busy = '';
		}
	}

	const tone = (p: Plan) =>
		p.status === 'applied' ? 'ok' : p.status === 'failed' ? 'bad' : ('neutral' as const);
</script>

<svelte:head><title>Pending changes · Hashi</title></svelte:head>

<PageHeader
	title="Pending changes"
	description="Every change to DNS is planned first. You see the exact values here; what you approve is exactly what gets applied."
/>

{#if error}
	<p
		role="alert"
		class="mb-4 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{error}
	</p>
{/if}
{#if notice}
	<p role="status" class="mb-4 rounded-2xl border bg-card p-4 text-[13px]">{notice}</p>
{/if}

<div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
	<div class="space-y-5">
		{#if !loaded && !error}
			<Skeleton class="h-48 rounded-2xl" />
		{:else if open.length === 0}
			<Panel title="Nothing waiting">
				<p class="text-[13px] text-muted-foreground">
					Hashi and the DNS provider agree. New changes appear here when you save something or the
					background sync finds a difference.
				</p>
			</Panel>
		{/if}
		{#each open as p (p.id)}
			<PlanReview title={p.title} trigger={triggerText(p)} risk={p.risk} items={reviewItems(p)}>
				{#snippet footer()}
					<p class="mr-auto text-[12.5px] text-muted-foreground">
						{#if p.status === 'pending'}Approving asks for your passkey. Nothing changes until you
							approve.{:else if p.status === 'applying'}Applying now…{:else}Approved; applies on the
							next sync.{/if}
					</p>
					{#if p.status !== 'applying'}
						<Button variant="ghost" size="sm" disabled={!!busy} onclick={() => act(p, 'discard')}
							>Discard</Button
						>
					{/if}
					{#if p.status === 'pending'}
						<Button size="sm" disabled={!!busy} onclick={() => act(p, 'approve')}>
							{#if busy === p.id}<LoaderCircle class="animate-spin" />{:else}<Fingerprint />{/if}
							Approve and apply
						</Button>
					{:else if p.status === 'approved'}
						<Button size="sm" disabled={!!busy} onclick={() => act(p, 'apply')}>
							{#if busy === p.id}<LoaderCircle class="animate-spin" />{:else}<Play />{/if}
							Apply now
						</Button>
					{/if}
				{/snippet}
			</PlanReview>
		{/each}
	</div>
	<Panel title="Recent runs" description="Applied, failed and replaced plans">
		{#if recent.length === 0}
			<p class="text-[13px] text-muted-foreground">No plan has finished yet.</p>
		{/if}
		<ul class="space-y-3">
			{#each recent as r (r.id)}
				<li class="text-[13px]">
					<div class="flex items-center justify-between gap-2">
						<span class="font-medium">{r.title}</span>
						<Tag tone={tone(r)}>{r.status}</Tag>
					</div>
					<div class="text-[12.5px] break-words text-muted-foreground">
						{fmtTime(r.finished_at ?? r.created_at)} · {outcomeText(r)}
					</div>
				</li>
			{/each}
		</ul>
	</Panel>
</div>
