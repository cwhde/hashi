<script lang="ts">
	import { actionText, actorText } from '#lib/activity.ts';
	import { onMount } from 'svelte';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import Sparkline from '#lib/components/hashi/Sparkline.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { ArrowUpRight, Settings2 } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { stripHealth, type Endpoint } from '#lib/monitor.ts';
	import { dateTime } from '#lib/format.ts';
	import { arrange, widgetNames, type WidgetId } from '#lib/overview.ts';
	import { theme } from '#lib/theme.svelte.ts';
	import type { Health } from '#lib/design/mock.ts';
	import { ChartSync } from '#lib/chart-sync.svelte.ts';

	// The start page (10-ui.md §3, bug 31): what needs the owner first, then the rest, as widgets in the order and
	// selection kept in Settings → Appearance. Each widget loads its own data, so a slow one never holds up the others.
	type Plan = Schemas['SyncPlan'];
	type Fw = Schemas['FirewallHost'];
	type Tr = Schemas['TraefikConnection'];
	type Cert = Schemas['Certificate'];
	type Sec = Schemas['SecurityDashboard'];
	type Audit = Schemas['AuditEvent'];

	let shown = $state<WidgetId[] | null>(null);
	let plans = $state<Plan[] | null>(null);
	let endpoints = $state<Endpoint[] | null>(null);
	let fw = $state<Fw[] | null>(null);
	let tr = $state<Tr[] | null>(null);
	let certs = $state<Cert[] | null>(null);
	let sec = $state<Sec | null>(null);
	let audit = $state<Audit[] | null>(null);
	// Why a source didn't load, by widget that reads it.
	let failed = $state<Partial<Record<WidgetId, string>>>({});

	function load<T>(
		users: WidgetId[],
		call: () => Promise<{ data?: T; error?: unknown }>,
		set: (d: T) => void
	) {
		if (!users.some((u) => shown!.includes(u))) return;
		void call().then((r) => {
			if (r.data !== undefined) set(r.data);
			else for (const u of users) failed[u] = problemText(r.error);
		});
	}

	onMount(async () => {
		const a = await theme.load();
		shown = arrange(a.data?.overview_widgets)
			.filter((w) => w.visible)
			.map((w) => w.id);
		load(
			['changes'],
			() => api.GET('/api/sync/plans'),
			(d) =>
				(plans = d.open
					.filter((p) => p.status === 'pending')
					.toSorted((x, y) => (x.risk === y.risk ? 0 : x.risk === 'high' ? -1 : 1)))
		);
		load(
			['attention', 'health'],
			() => api.GET('/api/monitor/endpoints'),
			(d) => (endpoints = d.endpoints)
		);
		load(
			['attention', 'edge'],
			() => api.GET('/api/firewall/hosts'),
			(d) => (fw = d.hosts)
		);
		load(
			['attention', 'edge'],
			() => api.GET('/api/traefik/connections'),
			(d) => (tr = d.connections)
		);
		load(
			['attention', 'certificates'],
			() => api.GET('/api/certificates'),
			(d) => (certs = d.certificates)
		);
		load(
			['attention', 'security'],
			() => api.GET('/api/security/dashboard', { params: { query: { range: '24h' } } }),
			(d) => (sec = d)
		);
		load(
			['activity'],
			() => api.GET('/api/activity/audit', { params: { query: { limit: 6 } } }),
			(d) => (audit = d.events)
		);
	});

	const hostState = (h: { health: string }): Health =>
		h.health === 'healthy'
			? 'up'
			: h.health === 'degraded'
				? 'degraded'
				: h.health === 'failed'
					? 'down'
					: 'unknown';
	const bad = (h: { health: string }) => h.health === 'failed' || h.health === 'degraded';
	const daysLeft = (iso?: string) =>
		iso ? Math.floor((new Date(iso).getTime() - Date.now()) / 86_400_000) : undefined;
	const compact = new Intl.NumberFormat(undefined, { notation: 'compact' });

	type Item = { key: string; state: Health; title: string; detail: string; href: string };
	const attention = $derived.by((): Item[] | null => {
		if (!endpoints || !fw || !tr || !certs) return null;
		const out: Item[] = [];
		for (const e of endpoints)
			if (!e.paused && (e.state === 'down' || e.state === 'degraded'))
				out.push({
					key: `e${e.id}`,
					state: e.state,
					title: e.name,
					detail: `${e.state === 'down' ? 'Down' : 'Degraded'} since ${dateTime(e.state_since)}`,
					href: `/status/${e.id}`
				});
		for (const h of fw.filter(bad))
			out.push({
				key: `f${h.id}`,
				state: hostState(h),
				title: h.name,
				detail: h.last_error || 'The last check failed',
				href: `/firewall/${h.id}`
			});
		for (const h of tr.filter(bad))
			out.push({
				key: `t${h.id}`,
				state: hostState(h),
				title: h.name,
				detail: h.last_error || 'The last check failed',
				href: '/traefik?tab=hosts'
			});
		for (const c of certs)
			if (c.level === 'expired' || c.level === 'critical' || c.last_renewal_result === 'failed')
				out.push({
					key: `c${c.id}`,
					state: c.level === 'expired' ? 'down' : 'degraded',
					title: c.name,
					detail:
						c.level === 'expired'
							? 'Certificate expired'
							: c.last_renewal_result === 'failed'
								? `Renewal failed: ${c.last_renewal_error}`
								: `Certificate expires ${dateTime(c.not_after)}`,
					href: '/traefik?tab=certificates'
				});
		for (const f of sec?.stale_feeds ?? [])
			out.push({
				key: `s${f.set_id}`,
				state: 'degraded',
				title: `${f.name} is stale`,
				detail: f.last_success ? `Last fetched ${dateTime(f.last_success)}` : 'Never fetched',
				href: `/ip-sets/${f.set_id}`
			});
		// Down before slow, so the first rows are the worst (bug 128).
		return out.toSorted((a, b) => (a.state === 'down' ? 0 : 1) - (b.state === 'down' ? 0 : 1));
	});

	const live = $derived((endpoints ?? []).filter((e) => !e.paused));
	const rank = { down: 0, degraded: 1, unknown: 2, up: 3, paused: 4 } as const;
	const worst = $derived(
		live
			.toSorted((a, b) => rank[a.state] - rank[b.state] || a.name.localeCompare(b.name))
			.slice(0, 6)
	);
	const byExpiry = $derived(
		(certs ?? [])
			.filter((c) => c.not_after)
			.toSorted((a, b) => a.not_after!.localeCompare(b.not_after!))
	);
	// The security card's three sparklines share one pointer (allowed, challenged, blocked at the same moment).
	const secSync = new ChartSync();
</script>

<svelte:head><title>Overview · Hashi</title></svelte:head>

<PageHeader title="Overview">
	{#snippet actions()}
		<Button variant="outline" size="sm" href="/settings?category=appearance"
			><Settings2 />Arrange widgets</Button
		>
	{/snippet}
</PageHeader>

{#snippet open(href: string, label: string)}
	<Button variant="ghost" size="icon-sm" {href} aria-label={label}><ArrowUpRight /></Button>
{/snippet}

{#snippet waiting()}
	<div class="space-y-2">
		<Skeleton class="h-6" /><Skeleton class="h-6" /><Skeleton class="h-6" />
	</div>
{/snippet}

{#snippet body(id: WidgetId)}
	{#if failed[id]}
		<p role="alert" class="text-md text-bad-text">{failed[id]}</p>
	{:else if id === 'changes'}
		{#if !plans}{@render waiting()}
		{:else if plans.length === 0}<p class="text-md text-muted-foreground">
				Nothing waits for your approval.
			</p>
		{:else}
			<ul class="divide-y">
				{#each plans.slice(0, 5) as p (p.id)}
					<li class="flex items-center gap-3 py-2.5">
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2 text-md font-medium">
								<span class="truncate">{p.title}</span>{#if p.risk === 'high'}<Tag tone="bad"
										>high risk</Tag
									>{/if}
							</div>
							<div class="truncate text-sm text-muted-foreground">
								{p.target_name || p.subsystem} · {dateTime(p.created_at)}
							</div>
						</div>
					</li>
				{/each}
			</ul>
			{#if plans.length > 5}<p class="pt-2 text-sm text-muted-foreground">
					{plans.length - 5} more
				</p>{/if}
		{/if}
	{:else if id === 'attention'}
		{#if !attention}{@render waiting()}
		{:else if attention.length === 0}<p class="text-md text-muted-foreground">Nothing needs you.</p>
		{:else}
			<ul class="divide-y">
				{#each attention.slice(0, 6) as r (r.key)}
					<li class="flex items-center gap-3 py-2.5">
						<StatusDot state={r.state} />
						<div class="min-w-0 flex-1">
							<div class="truncate text-md font-medium">{r.title}</div>
							<div class="truncate text-sm text-muted-foreground" title={r.detail}>{r.detail}</div>
						</div>
						{@render open(r.href, `Open ${r.title}`)}
					</li>
				{/each}
			</ul>
			{#if attention.length > 6}<a
					href="/status"
					class="block pt-2 text-sm text-muted-foreground underline-offset-4 hover:underline"
					>{attention.length - 6} more</a
				>{/if}
		{/if}
	{:else if id === 'health'}
		{#if !endpoints}{@render waiting()}
		{:else if live.length === 0}<p class="text-md text-muted-foreground">No checks yet.</p>
		{:else}
			<div class="mb-3 flex gap-5 text-md">
				{#each [['up', 'up', 'text-ok-text'], ['degraded', 'slow', 'text-warn-text'], ['down', 'down', 'text-bad-text']] as [s, word, tone] (s)}
					<span
						><span class="tnum text-title font-semibold {tone}"
							>{live.filter((e) => e.state === s).length}</span
						>
						{word}</span
					>
				{/each}
			</div>
			<div class="space-y-2">
				{#each worst as e (e.id)}
					<a href="/status/{e.id}" class="grid grid-cols-[7rem_1fr] items-center gap-3">
						<span class="truncate text-sm hover:underline">{e.name}</span>
						<HistoryStrip samples={stripHealth(e.strip)} labels={false} />
					</a>
				{/each}
			</div>
		{/if}
	{:else if id === 'edge'}
		{#if !fw || !tr}{@render waiting()}
		{:else if fw.length + tr.length === 0}<p class="text-md text-muted-foreground">
				No hosts yet. <a href="/connections" class="underline">Add one</a>
			</p>
		{:else}
			<ul class="space-y-3">
				{#each fw as h (h.id)}
					<li class="flex items-start gap-3">
						<StatusDot state={hostState(h)} class="mt-1.5" />
						<a href="/firewall/{h.id}" class="min-w-0 flex-1">
							<div class="flex items-center gap-2 text-md font-medium">
								<span class="truncate hover:underline">{h.name}</span><Tag
									>{h.edge_mode === 'managed' ? 'firewall' : 'external NAT'}</Tag
								>
							</div>
							<div class="truncate text-sm text-muted-foreground">
								{#if h.public_ip}<Mono>{h.public_ip}</Mono> ·
									{' '}{/if}{h.last_applied_at
									? `applied ${dateTime(h.last_applied_at)}`
									: 'not applied yet'}
							</div>
						</a>
					</li>
				{/each}
				{#each tr as t (t.id)}
					<li class="flex items-start gap-3">
						<StatusDot state={hostState(t)} class="mt-1.5" />
						<a href="/traefik?tab=hosts" class="min-w-0 flex-1">
							<div class="flex items-center gap-2 text-md font-medium">
								<span class="truncate hover:underline">{t.name}</span><Tag>Traefik</Tag>
							</div>
							<div class="truncate text-sm text-muted-foreground">
								{t.installed_version
									? `v${t.installed_version.replace(/^v/, '')}`
									: 'not installed yet'}{#if t.last_validated_at}{' '}
									· checked {dateTime(t.last_validated_at)}{/if}
							</div>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if id === 'security'}
		{#if !sec}{@render waiting()}
		{:else}
			<!-- One row per decision, spread over the card's height (bug 142). -->
			<div class="flex h-full flex-col justify-around gap-3">
				{#each [['allowed', 'Allowed', 'text-ok-text'], ['challenged', 'Challenged', 'text-warn-text'], ['denied', 'Blocked', 'text-bad-text']] as const as [k, word, tone] (k)}
					<a href="/security" class="grid grid-cols-[6.5rem_1fr] items-center gap-3">
						<span class="min-w-0">
							<span class="tnum block text-lg font-semibold">{compact.format(sec.totals[k])}</span>
							<span class="block text-xs text-muted-foreground">{word}</span>
						</span>
						<Sparkline
							values={sec.series.map((p) => p[k])}
							labels={sec.series.map((p) => dateTime(p.at))}
							times={sec.series.map((p) => (p.at ? Date.parse(p.at) : 0))}
							sync={secSync}
							width={240}
							height={36}
							class="w-full {tone}"
						/>
					</a>
				{/each}
			</div>
		{/if}
	{:else if id === 'certificates'}
		{#if !certs}{@render waiting()}
		{:else if byExpiry.length === 0}<p class="text-md text-muted-foreground">
				No certificates yet.
			</p>
		{:else}
			<ul class="space-y-2.5 text-md">
				{#each byExpiry.slice(0, 4) as c (c.id)}
					{@const left = daysLeft(c.not_after)!}
					<li class="flex items-center justify-between gap-3">
						<span class="truncate">{c.name}</span><Tag
							tone={c.level === 'expired' || c.level === 'critical'
								? 'bad'
								: c.level === 'warning'
									? 'warn'
									: 'neutral'}>{left < 0 ? 'expired' : `${left} days`}</Tag
						>
					</li>
				{/each}
				{#if byExpiry.length > 4}<li class="text-muted-foreground">
						{byExpiry.length - 4} more
					</li>{/if}
			</ul>
		{/if}
	{:else if id === 'activity'}
		{#if !audit}{@render waiting()}
		{:else if audit.length === 0}<p class="text-md text-muted-foreground">Nothing yet.</p>
		{:else}
			<ul class="space-y-2.5">
				{#each audit as a (a.id)}
					<li class="grid grid-cols-[5.5rem_1fr] gap-2 text-sm">
						<span class="tnum truncate text-muted-foreground">{dateTime(a.at)}</span><span
							class="min-w-0"
							><span class="flex items-center gap-2"
								><span class="truncate" title={a.action}>{actionText(a.action)}</span
								>{#if a.outcome !== 'success'}<Tag tone={a.outcome === 'failure' ? 'bad' : 'warn'}
										>{a.outcome}</Tag
									>{/if}</span
							><span class="block truncate text-muted-foreground">{actorText(a.actor)}</span></span
						>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
{/snippet}

{#if !shown}
	<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
		{#each [0, 1, 2] as i (i)}<Skeleton class="h-48 rounded-2xl" />{/each}
	</div>
{:else if shown.length === 0}
	<p data-state class="text-md text-muted-foreground">
		Every widget is hidden. <a href="/settings?category=appearance" class="underline"
			>Show some in Appearance</a
		>
	</p>
{:else}
	<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
		{#each shown as id (id)}
			<Panel title={widgetNames[id]}>
				{#snippet actions()}
					{#if id === 'changes'}{@render open('/changes', 'Open changes')}
					{:else if id === 'security'}{@render open('/security', 'Open security')}
					{:else if id === 'certificates'}{@render open(
							'/traefik?tab=certificates',
							'Open certificates'
						)}
					{:else if id === 'activity'}{@render open('/activity', 'Open activity')}
					{:else if id === 'health'}{@render open('/status', 'Open status')}{/if}
				{/snippet}
				{@render body(id)}
			</Panel>
		{/each}
	</div>
{/if}
