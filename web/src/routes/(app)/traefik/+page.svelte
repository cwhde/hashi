<script lang="ts">
	import { hrefWithTab, tabFrom } from '#lib/url-tab.ts';
	import SshTarget from '#lib/components/hashi/SshTarget.svelte';
	import UserMiddlewares from '#lib/components/hashi/traefik/UserMiddlewares.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as Tabs from '#lib/components/ui/tabs/index.ts';
	import { CircleAlert, FileCode2, RefreshCw } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { pendingPlans } from '#lib/sync-pending.svelte.ts';
	import { fmtTime, outcomeText } from '#lib/sync.ts';
	import type { Health } from '#lib/design/mock.ts';

	// 04-traefik-services.md §2–6 and 03 §5.3 UI: each Traefik host with the files Hashi applied (and their hashes),
	// and the certificates with expiry, last renewal and per-host distribution.
	type Conn = Schemas['TraefikConnection'];
	type Cert = Schemas['Certificate'];
	let conns = $state<Conn[]>([]);
	let certs = $state<Cert[]>([]);
	let loaded = $state(false);
	let loadError = $state('');
	let busy = $state('');
	let outcome = $state<{ text: string; review: boolean; bad?: boolean } | null>(null);

	// The open tab lives in the address (bug 41).
	const tabs = ['hosts', 'middlewares', 'certificates'];
	let tab = $state(tabFrom(page.url, tabs));
	const setTab = (t: string) =>
		goto(hrefWithTab(page.url, tabs, t), { replace: true, reset: false });

	async function load() {
		const [c, k] = await Promise.all([
			api.GET('/api/traefik/connections'),
			api.GET('/api/certificates')
		]);
		if (c.error || !c.data) throw c.error;
		if (k.error || !k.data) throw k.error;
		conns = c.data.connections;
		certs = k.data.certificates;
		loaded = true;
	}
	onMount(() => load().catch((e) => (loadError = problemText(e))));

	const healthOf = (h: Conn['health']): Health =>
		({
			healthy: 'up',
			degraded: 'degraded',
			failed: 'down',
			validating: 'unknown',
			unknown: 'unknown'
		})[h] as Health;

	async function act(what: string, fn: () => Promise<void>) {
		busy = what;
		outcome = null;
		try {
			await fn();
			await load();
			await pendingPlans.refresh();
		} catch (e) {
			outcome = { text: problemText(e), review: false, bad: true };
		} finally {
			busy = '';
		}
	}

	const plan = (c: Conn) =>
		act('plan:' + c.id, async () => {
			const res = await api.POST('/api/traefik/connections/{id}/plan', {
				params: { path: { id: c.id } }
			});
			if (res.error || !res.data) throw res.error;
			const p = res.data.plan;
			if (!p) outcome = { text: `${c.name} already matches. Nothing to change.`, review: false };
			else if (p.status === 'pending')
				outcome = {
					text: `${p.title}: ${p.items.length} change(s) need your approval.`,
					review: true
				};
			else outcome = { text: outcomeText(p), review: false, bad: p.status === 'failed' };
		});

	const renew = (k: Cert) =>
		act('renew:' + k.id, async () => {
			const res = await api.POST('/api/certificates/{id}/renew', {
				params: { path: { id: k.id } }
			});
			if (res.error || !res.data) throw res.error;
			const p = res.data.plan;
			outcome = p
				? {
						text:
							p.status === 'applied'
								? `${k.name} renewed. "Check and sync" on each host copies it there now; otherwise Hashi does within the hour.`
								: outcomeText(p),
						review: false,
						bad: p.status === 'failed'
					}
				: { text: `Nothing to renew for ${k.name}.`, review: false };
		});

	const base = (p: string) => p.split('/').pop() ?? p;
	const short = (h: string) => (h ? h.slice(0, 12) + '…' : '');
	const daysLeft = (iso?: string) =>
		iso ? Math.floor((new Date(iso).getTime() - Date.now()) / 86_400_000) : undefined;
	const levelTone = (l: Cert['level']) =>
		l === 'expired' || l === 'critical' ? 'bad' : l === 'warning' ? 'warn' : 'neutral';
	const distText = (k: Cert) => {
		// A host counts only while it serves this exact certificate (fingerprint), not an older one.
		const v = k.distributions.filter(
			(d) => d.state === 'verified' && d.fingerprint === k.fingerprint
		).length;
		return `served by ${v} of ${conns.length} host${conns.length === 1 ? '' : 's'}`;
	};
</script>

<svelte:head><title>Traefik · Hashi</title></svelte:head>

<PageHeader title="Traefik">
	{#snippet actions()}<Button href="/connections?add=traefik">Add host</Button>{/snippet}
</PageHeader>

{#if outcome}
	<p
		role={outcome.bad ? 'alert' : 'status'}
		class="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-3 text-md {outcome.bad
			? 'border-bad/30 bg-bad/10 text-bad-text'
			: 'bg-card'}"
	>
		{#if outcome.bad}<CircleAlert class="size-4 shrink-0" />{/if}{outcome.text}
		{#if outcome.review}<a href="/changes" class="font-medium underline underline-offset-2"
				>Review</a
			>{/if}
	</p>
{/if}

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-md text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !loaded}
	<Skeleton class="h-10 w-72" /><Skeleton class="mt-4 h-64 rounded-2xl" />
{:else}
	<Tabs.Root bind:value={tab} onValueChange={setTab}>
		<Tabs.List class="mb-4">
			<Tabs.Trigger value="hosts">Hosts</Tabs.Trigger>
			<Tabs.Trigger value="middlewares">Your middlewares</Tabs.Trigger>
			<Tabs.Trigger value="certificates">Certificates</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="hosts" class="space-y-4">
			{#each conns as c (c.id)}
				<Panel>
					<div class="grid gap-5 lg:grid-cols-[18rem_minmax(0,1fr)]">
						<div class="space-y-3">
							<div>
								<div class="flex items-center gap-2 text-lg font-semibold">
									{c.name}<StatusBadge state={healthOf(c.health)} />
								</div>
								<div class="text-sm text-muted-foreground">
									SSH <Mono>{c.user}@{c.host}:{c.port}</Mono>{#if c.os_name}<span
											>&nbsp;· {c.os_name}</span
										>{/if}
								</div>
							</div>
							<dl class="grid grid-cols-2 gap-y-2 text-md">
								<dt class="text-muted-foreground">Version</dt>
								<dd>{c.installed_version || 'not installed yet'}</dd>
								<dt class="text-muted-foreground">Checked</dt>
								<dd>{c.last_validated_at ? fmtTime(c.last_validated_at) : 'never'}</dd>
								<dt class="text-muted-foreground">Host key</dt>
								<dd class="truncate"><Mono>{c.host_key_fingerprint}</Mono></dd>
							</dl>
							{#if c.health === 'failed' && c.last_error}
								<p class="rounded-xl border border-bad/25 bg-bad/10 p-3 text-sm text-bad-text">
									{c.last_error}
								</p>
							{/if}
							<Button size="sm" variant="outline" onclick={() => plan(c)} disabled={!!busy}
								><RefreshCw />{busy === 'plan:' + c.id ? 'Checking…' : 'Check and sync'}</Button
							>
							<SshTarget kind="traefik" id={c.id} value={c.ssh_target_set_id} />
						</div>
						<div class="min-w-0">
							<div class="mb-1.5 text-xs font-medium text-muted-foreground">
								Files Hashi applied on {c.name}
							</div>
							{#if c.files.length}
								<ul class="divide-y rounded-xl border" aria-label="Files on {c.name}">
									{#each c.files as f (f.path)}
										<li class="flex items-center gap-3 px-3 py-2 text-md">
											<FileCode2 class="size-4 shrink-0 text-muted-foreground" />
											<span class="min-w-0 flex-1 truncate" title={f.path}
												><Mono>{base(f.path)}</Mono></span
											>
											<span class="hidden text-xs text-muted-foreground sm:inline"
												><Mono>{short(f.sha256)}</Mono></span
											>
											<span class="hidden text-xs text-muted-foreground md:inline"
												>{fmtTime(f.applied_at)}</span
											>
										</li>
									{/each}
								</ul>
							{:else}
								<p class="rounded-xl border p-3 text-md text-muted-foreground">
									Nothing applied yet.
								</p>
							{/if}
						</div>
					</div>
				</Panel>
			{:else}
				<Panel>
					<div class="flex items-center gap-3 text-md text-muted-foreground">
						No Traefik host yet.<Button size="sm" variant="outline" href="/connections?add=traefik"
							>Add host</Button
						>
					</div>
				</Panel>
			{/each}
		</Tabs.Content>

		<Tabs.Content value="middlewares"><UserMiddlewares /></Tabs.Content>

		<Tabs.Content value="certificates">
			<Panel title="Certificates" description="Renewed 30 days before expiry" flush>
				{#if certs.length}
					<ul class="divide-y text-md" aria-label="Certificates">
						{#each certs as k (k.id)}
							{@const left = daysLeft(k.not_after)}
							<li class="flex flex-wrap items-center gap-x-4 gap-y-1.5 px-4 py-3">
								<div class="min-w-0 flex-1">
									<div class="font-medium">{k.names.join(', ')}</div>
									<div class="text-sm text-muted-foreground">
										{#if k.issuer}<span>{k.issuer}&nbsp;·&nbsp;</span>{/if}{distText(
											k
										)}{#if k.last_renewal_result === 'failed'}
											<span class="text-bad-text">
												· last renewal failed: {k.last_renewal_error}</span
											>{/if}
									</div>
									{#each k.distributions.filter((d) => d.state === 'failed') as d (d.connection_id)}
										<div class="text-sm text-bad-text">{d.connection_name}: {d.error}</div>
									{/each}
								</div>
								{#if left !== undefined}<Tag tone={levelTone(k.level)}
										>{left < 0 ? 'expired' : `${left} days left`}</Tag
									>{:else}<Tag tone="warn">not issued</Tag>{/if}
								<Button size="sm" variant="outline" onclick={() => renew(k)} disabled={!!busy}
									>{busy === 'renew:' + k.id ? 'Renewing…' : 'Renew now'}</Button
								>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="px-4 py-3 text-md text-muted-foreground">No certificate yet.</p>
				{/if}
			</Panel>
		</Tabs.Content>
	</Tabs.Root>
{/if}
