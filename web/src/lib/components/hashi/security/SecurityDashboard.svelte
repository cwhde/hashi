<script lang="ts">
	// Security → Dashboard (06 §13): what was let through, challenged and banned in a period, and why.
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import Sparkline from '#lib/components/hashi/Sparkline.svelte';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import AddressLink from '#lib/components/hashi/ipsets/AddressLink.svelte';
	import { TriangleAlert } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { country, dateTime, time } from '#lib/format.ts';
	import { eventLabel, sourceLabel } from './labels';

	type Range = '1h' | '24h' | '7d' | '30d';
	let range = $state<Range>('24h');
	let d = $state<Schemas['SecurityDashboard'] | null>(null);
	let error = $state('');
	// While a period loads, placeholders stand in for the numbers (bug 99), also when switching periods: dimmed old
	// numbers looked like a page that hangs.
	let loading = $state(false);
	let asked = 0;

	async function load(r: Range) {
		error = '';
		loading = true;
		const mine = ++asked;
		const res = await api.GET('/api/security/dashboard', { params: { query: { range: r } } });
		if (mine !== asked) return; // a newer period was picked meanwhile
		loading = false;
		if (res.error || !res.data) {
			error = problemText(res.error);
			return;
		}
		d = res.data;
	}
	// Each series point's time, for the values shown on hover (bug 119).
	const pointLabels = $derived(
		(d?.series ?? []).map((p) =>
			p.at ? (range === '1h' || range === '24h' ? time(p.at) : dateTime(p.at)) : ''
		)
	);
	$effect(() => {
		load(range);
	});

	const n = (v: number) => v.toLocaleString();
	type Key = 'allowed' | 'challenged' | 'denied' | 'waf_matches';
	const tiles: { k: string; key: Key; tone: string }[] = [
		{ k: 'Allowed', key: 'allowed', tone: 'text-ok-text' },
		{ k: 'Challenged', key: 'challenged', tone: 'text-warn-text' },
		{ k: 'Refused', key: 'denied', tone: 'text-bad-text' },
		{ k: 'WAF matches', key: 'waf_matches', tone: 'text-info-text' }
	];
	const countryMax = $derived(Math.max(1, ...(d?.countries.map((c) => c.refused) ?? [])));
	const funnel = $derived(
		d
			? [
					{ k: 'Challenged', n: d.funnel.challenged, tone: 'bg-warn/70' },
					{ k: 'Solved', n: d.funnel.solved, tone: 'bg-ok/70' },
					{
						k: 'Not solved yet',
						n: Math.max(0, d.funnel.challenged - d.funnel.solved - d.funnel.banned),
						tone: 'bg-idle'
					},
					{ k: 'Banned', n: d.funnel.banned, tone: 'bg-bad/70' }
				]
			: []
	);
	const sev = { info: 'ok', warning: 'warn', critical: 'bad' } as const;
	const ago = (iso?: string) => {
		if (!iso) return 'never';
		const h = Math.round((Date.now() - new Date(iso).getTime()) / 3_600_000);
		return h < 48 ? `${h} h` : `${Math.round(h / 24)} d`;
	};
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center gap-2">
		<ToggleGroup.Root
			type="single"
			bind:value={range}
			variant="outline"
			size="sm"
			aria-label="Period"
			>{#each ['1h', '24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}>{r}</ToggleGroup.Item
				>{/each}</ToggleGroup.Root
		>
		<span class="text-sm text-muted-foreground"
			>All services, all Traefik hosts, all firewall hosts</span
		>
		{#each d?.stale_feeds ?? [] as f (f.set_id)}
			<a
				href="/ip-sets/{f.set_id}"
				class="ml-auto inline-flex items-center gap-1.5 rounded-[10px] border border-warn/30 bg-warn/10 px-2.5 py-1 text-sm text-warn-text"
				><TriangleAlert class="size-3.5" />{f.name} is stale ({f.last_success
					? ago(f.last_success)
					: 'never fetched'})</a
			>
		{/each}
	</div>
	{#if error}<p
			role="alert"
			class="rounded-2xl border border-bad/30 bg-bad/10 p-3 text-md text-bad-text"
		>
			{error}
		</p>{/if}
	{#if (!d || loading) && !error}
		<div
			class="grid grid-cols-2 gap-px overflow-hidden rounded-2xl border md:grid-cols-4"
			aria-busy="true"
		>
			{#each tiles as t (t.k)}<div class="space-y-2 bg-card p-4">
					<div class="text-sm text-muted-foreground">{t.k}</div>
					<Skeleton class="h-7 w-20" /><Skeleton class="h-7 w-full" />
				</div>{/each}
		</div>
		<div class="grid gap-4 xl:grid-cols-3">
			<Skeleton class="h-64 rounded-2xl xl:col-span-2" /><Skeleton class="h-64 rounded-2xl" />
		</div>
	{:else if d}
		<div class="space-y-4">
			<div
				class="grid grid-cols-2 divide-x divide-y rounded-2xl border bg-card md:grid-cols-4 md:divide-y-0"
			>
				{#each tiles as t (t.k)}
					<div class="p-4">
						<div class="text-sm text-muted-foreground">{t.k}</div>
						<div class="tnum text-title font-semibold">{n(d.totals[t.key])}</div>
						<Sparkline
							values={d.series.map((p) => p[t.key])}
							width={200}
							labels={pointLabels}
							class="{t.tone} mt-1 w-full"
						/>
					</div>
				{/each}
			</div>
			<div class="grid gap-4 xl:grid-cols-3">
				<Panel title="Most refused addresses" class="xl:col-span-2" flush>
					<ul class="divide-y text-md">
						{#each d.top_addresses as ip (ip.address)}
							<li
								class="grid grid-cols-[minmax(0,9rem)_4rem_minmax(0,1fr)] items-center gap-3 px-4 py-2.5 lg:grid-cols-[10rem_4rem_4rem_5rem_minmax(0,1fr)_8rem]"
							>
								<AddressLink address={ip.address} />
								<span class="tnum text-right">{n(ip.refused)}</span>
								<span class="hidden text-muted-foreground lg:inline">{country(ip.country)}</span>
								<span class="hidden truncate text-muted-foreground lg:inline"
									>{ip.asn ? `AS${ip.asn}` : ''}</span
								>
								<span class="truncate"
									>{#if ip.state === 'banned'}<Tag tone="bad">banned</Tag
										>{:else if ip.state === 'challenged'}<Tag tone="warn">challenged</Tag>{/if}
									{ip.reason ?? ''}</span
								>
								<span class="hidden text-right text-muted-foreground lg:inline"
									>{ip.until ? `until ${dateTime(ip.until)}` : ''}</span
								>
							</li>
						{:else}
							<li class="px-4 py-6 text-center text-muted-foreground">
								Nothing refused in this period.
							</li>
						{/each}
					</ul>
				</Panel>
				<Panel title="Refused by country">
					{#if d.countries.length}
						<ul class="space-y-2 text-md">
							{#each d.countries as c (c.country)}
								<li class="grid grid-cols-[3.5rem_1fr_3.5rem] items-center gap-2">
									<span class="whitespace-nowrap">{country(c.country)}</span><span
										class="h-1.5 rounded-full bg-surface-2"
										><span
											class="block h-full rounded-full bg-bad/70"
											style="width:{(c.refused / countryMax) * 100}%"
										></span></span
									><span class="tnum text-right">{n(c.refused)}</span>
								</li>
							{/each}
						</ul>
					{:else}<p class="text-md text-muted-foreground">Nothing refused in this period.</p>{/if}
					<div class="mt-4 mb-2 text-sm font-medium text-muted-foreground">
						Most targeted services
					</div>
					<ul class="space-y-1.5 text-md">
						{#each d.services as s (s.service_id)}<li class="flex justify-between gap-2">
								<a href="/services/{s.service_id}" class="truncate">{s.name || 'Unknown service'}</a
								><span class="tnum">{n(s.refused)}</span>
							</li>{:else}<li class="text-muted-foreground">None</li>{/each}
					</ul>
				</Panel>
			</div>
			<div class="grid gap-4 lg:grid-cols-2">
				<Panel title="Challenges">
					<ul class="space-y-2 text-md">
						{#each funnel as s (s.k)}
							<li class="grid grid-cols-[8.5rem_1fr_3.5rem] items-center gap-2">
								<span>{s.k}</span><span class="h-1.5 rounded-full bg-surface-2"
									><span
										class="block h-full rounded-full {s.tone}"
										style="width:{(s.n / Math.max(1, d.funnel.challenged)) * 100}%"
									></span></span
								><span class="tnum text-right">{n(s.n)}</span>
							</li>
						{/each}
					</ul>
				</Panel>
				<Panel title="Kernel meters">
					<ul class="divide-y text-md">
						{#each d.meters as m (m.host_id)}
							<li class="flex justify-between gap-2 py-2">
								<span class="font-medium">{m.host || 'Unknown host'}</span><span class="tnum"
									>{n(m.bans)} <span class="text-muted-foreground">banned</span></span
								>
							</li>
						{:else}
							<li class="py-2 text-muted-foreground">No meter tripped in this period.</li>
						{/each}
					</ul>
				</Panel>
			</div>
			<Panel title="Recent events" flush>
				<ul class="divide-y text-md">
					{#each d.events as e, i (i)}
						<li
							class="grid grid-cols-[4.5rem_minmax(0,9rem)_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5 md:grid-cols-[4.5rem_10rem_minmax(0,1fr)_minmax(0,14rem)_6rem]"
						>
							<span class="tnum text-muted-foreground">{time(e.at)}</span><span
								class="flex min-w-0 items-center gap-1.5"
								>{#if e.country}<span class="shrink-0 text-muted-foreground" title={e.country}
										>{country(e.country)}</span
									>{/if}<AddressLink address={e.address} class="truncate" /></span
							><span class="truncate"
								>{eventLabel(e.type)}{#if e.path}<span class="text-muted-foreground">
										{e.method}
										{e.path}</span
									>{/if}{#if e.rules.length}<span class="text-muted-foreground">
										· rules {e.rules.join(', ')}</span
									>{/if}</span
							><span class="hidden truncate text-muted-foreground md:inline"
								>{e.service || e.host || sourceLabel(e.source)}</span
							><Tag tone={sev[e.severity]}>{e.severity}</Tag>
						</li>
					{:else}
						<li class="px-4 py-6 text-center text-muted-foreground">No events in this period.</li>
					{/each}
				</ul>
			</Panel>
		</div>
	{/if}
</div>
