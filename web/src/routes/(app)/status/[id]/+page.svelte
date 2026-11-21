<script lang="ts">
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '#lib/components/hashi/HistoryStrip.svelte';
	import LineChart from '#lib/components/hashi/LineChart.svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { ChevronLeft } from '@lucide/svelte';
	import { withReauth } from '#lib/auth/reauth.ts';
	import ConfirmDelete from '#lib/components/hashi/ConfirmDelete.svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { checkSummary, sourceText, stripHealth, uptimeText } from '#lib/monitor.ts';
	import { dateTime } from '#lib/format.ts';

	// 07 §5: status, response numbers, uptime, charts by range, events, settings.
	const id = $derived(page.params.id ?? '');
	let d = $state<Schemas['MonitorEndpointDetail'] | null>(null);
	let providers = $state<Schemas['NotificationProvider'][]>([]);
	let s = $state<Schemas['MonitorEndpointSettings'] | null>(null);
	let range = $state<'1h' | '24h' | '7d' | '30d'>('24h');
	let rangeChosen = false; // a new check opens on 1h while 24h has nothing to chart yet (bug 88)
	// Each range charts closed buckets of its own size, so each starts at its own time (07 §5).
	const startsAfter = { '1h': 'minute', '24h': '5 minutes', '7d': 'hour', '30d': 'hour' } as const;
	let error = $state('');
	let confirming = $state(false);
	let notice = $state('');
	let busy = $state(false);

	async function load() {
		const r = await api.GET('/api/monitor/endpoints/{id}', {
			params: { path: { id }, query: { range } }
		});
		if (r.error || !r.data) throw r.error;
		if (!rangeChosen && range === '24h' && r.data.series.length === 0) {
			rangeChosen = true;
			range = '1h';
			return;
		}
		d = r.data;
		if (!s) {
			const e = r.data.endpoint;
			s = {
				group: e.group,
				paused: e.paused,
				public: e.public,
				interval_seconds: e.interval_seconds,
				timeout_seconds: e.timeout_seconds,
				degraded_ms: e.degraded_ms,
				route_provider_id: e.route_provider_id,
				version: e.version
			};
			draft.mark();
		}
	}
	// Unsaved edits: leaving the page asks first (bug 40).
	const draft = new Draft(() => s);
	guardUnsaved(() => draft.dirty);
	$effect(() => {
		void range;
		void id;
		load().catch((e) => (error = problemText(e)));
	});
	$effect(() => {
		api.GET('/api/notifications/providers').then((r) => (providers = r.data?.providers ?? []));
	});

	async function save(next = s) {
		if (!next) return;
		error = notice = '';
		busy = true;
		try {
			const r = await api.PUT('/api/monitor/endpoints/{id}', {
				params: { path: { id } },
				body: { ...next, route_provider_id: next.route_provider_id || undefined }
			});
			if (r.error) throw r.error;
			s = null;
			await load();
			notice = 'Saved.';
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	// Pause and Resume change only that, from what is stored: unsaved edits are neither sent nor lost (bug 59).
	async function togglePause() {
		if (!d || !s) return;
		const e = d.endpoint;
		const clean = !draft.dirty;
		error = notice = '';
		busy = true;
		try {
			const r = await api.PUT('/api/monitor/endpoints/{id}', {
				params: { path: { id } },
				body: {
					group: e.group,
					paused: !e.paused,
					public: e.public,
					interval_seconds: e.interval_seconds,
					timeout_seconds: e.timeout_seconds,
					degraded_ms: e.degraded_ms,
					route_provider_id: e.route_provider_id || undefined,
					version: e.version
				}
			});
			if (r.error) throw r.error;
			await load();
			s.paused = d.endpoint.paused;
			s.version = d.endpoint.version;
			if (clean) draft.mark();
		} catch (err) {
			error = problemText(err);
		} finally {
			busy = false;
		}
	}
	async function remove() {
		try {
			await withReauth(() =>
				api.DELETE('/api/monitor/endpoints/{id}', { params: { path: { id } } })
			);
			goto('/status');
		} catch (e) {
			error = problemText(e);
		}
	}
	const ms = (v: number | undefined) => (v === undefined ? 'no data' : `${v} ms`);
	const series = $derived(d?.series ?? []);
	// Public minus internal response time, with the moment of each (bug 156); negative when the public path is faster.
	const gapPoints = $derived(series.filter((p) => p.gap_ms !== undefined && p.gap_ms !== null));
	const gaps = $derived(gapPoints.map((p) => p.gap_ms ?? 0));
	// Bug 64: a check made from something links back to it.
	const sourceHref: Record<string, (id: string) => string> = {
		service: (id) => `/services/${id}`,
		firewall_host: (id) => `/firewall/${id}`,
		traefik: () => '/traefik',
		dns_record: () => '/dns?tab=records'
	};
	const sourceLink: Record<string, string> = {
		service: 'Open service',
		firewall_host: 'Open host',
		traefik: 'Open Traefik',
		dns_record: 'Open DNS'
	};
</script>

<svelte:head><title>{d?.endpoint.name ?? 'Endpoint'} · Hashi</title></svelte:head>

<a
	href="/status"
	class="mb-3 inline-flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Status</a
>
{#if error}<p role="alert" class="mb-3 text-md text-bad-text">{error}</p>{/if}
{#if notice}<p role="status" class="mb-3 text-md text-ok-text">{notice}</p>{/if}
{#if !d}
	{#if !error}<Skeleton class="h-48" />{/if}
{:else}
	{@const m = d.endpoint}
	<div class="mb-4 rounded-2xl border bg-card p-4">
		<div class="flex flex-wrap items-start gap-4">
			<div class="min-w-0 flex-1">
				<h1 class="text-title font-semibold">{m.name}</h1>
				<p class="truncate text-md text-muted-foreground">
					{checkSummary(m)} · every {m.interval_seconds} s
				</p>
			</div>
			<StatusBadge state={m.state} />
			{#if m.source_id && sourceHref[m.source_type]}<Button
					variant="outline"
					size="sm"
					href={sourceHref[m.source_type]!(m.source_id)}>{sourceLink[m.source_type]}</Button
				>{/if}
			{#if s}
				<Button variant="outline" size="sm" disabled={busy} onclick={togglePause}
					>{m.paused ? 'Resume' : 'Pause'}</Button
				>
			{/if}
		</div>
		<dl class="mt-4 grid grid-cols-2 gap-3 text-md sm:grid-cols-4 lg:grid-cols-8">
			{#each [['Last check', m.last_check_at ? dateTime(m.last_check_at) : 'not yet'], ['Response min', ms(d.min_ms)], ['avg', ms(d.avg_ms)], ['max', ms(d.max_ms)], ['Uptime 1 h', uptimeText(d.uptime_1h)], ['24 h', uptimeText(m.uptime_24h)], ['7 d', uptimeText(d.uptime_7d)], ['30 d', uptimeText(d.uptime_30d)]] as [k, v] (k)}
				<div>
					<dt class="text-xs text-muted-foreground">{k}</dt>
					<dd class="tnum">{v}</dd>
				</div>
			{/each}
		</dl>
		<HistoryStrip samples={stripHealth(m.strip)} class="mt-4" />
	</div>
	<div class="mb-3 flex justify-center">
		<ToggleGroup.Root
			type="single"
			value={range}
			onValueChange={(v) => {
				if (!v) return;
				rangeChosen = true;
				range = v as typeof range;
			}}
			variant="outline"
			size="sm"
		>
			{#each ['1h', '24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}>{r}</ToggleGroup.Item
				>{/each}
		</ToggleGroup.Root>
	</div>
	{#if series.length === 0}
		<p class="mb-4 text-center text-md text-muted-foreground">
			The {range} chart starts after the first full {startsAfter[range]} of checks.
		</p>
	{:else}
		<div class="mb-4 grid gap-4 lg:grid-cols-3">
			<LineChart
				title="Response time"
				values={series.map((p) => p.latency_ms)}
				unit=" ms"
				labels={series.map((p) => dateTime(p.at))}
			/>
			{#if gaps.length}
				<LineChart
					title="Public vs internal gap"
					values={gaps}
					unit=" ms"
					labels={gapPoints.map((p) => dateTime(p.at))}
				/>
			{/if}
			<LineChart
				title="Availability"
				values={series.map((p) => p.availability)}
				current={uptimeText(series.at(-1)?.availability)}
				labels={series.map((p) => dateTime(p.at))}
				format={(v) => uptimeText(v)}
			/>
		</div>
	{/if}
	<div class="grid gap-4 lg:grid-cols-2">
		<Panel title="Events">
			{#if d.events.length === 0}<p class="text-md text-muted-foreground">No changes yet.</p>{/if}
			<ol class="space-y-2.5 text-md">
				{#each d.events as e (e.id)}
					<li class="grid gap-x-3 sm:grid-cols-[9rem_1fr]">
						<span class="tnum text-muted-foreground">{dateTime(e.at)}</span>
						<span class="min-w-0 break-words"><StatusBadge state={e.to} /> {e.message}</span>
					</li>
				{/each}
			</ol>
		</Panel>
		<Panel title="Checks and settings" description={sourceText[m.source_type]}>
			<ul class="mb-4 space-y-1.5 text-md">
				{#each m.checks as c (c.id)}
					<li class="min-w-0 break-words">
						<span class="font-medium">{c.role ? `${c.role} path` : c.type}</span>
						<span class="text-muted-foreground">{c.target}</span>
						{#if c.last_error}<span class="block text-bad-text">{c.last_error}</span>{/if}
					</li>
				{/each}
			</ul>
			{#if s}
				<div class="grid grid-cols-2 gap-3 text-md">
					<div class="col-span-2 space-y-1.5">
						<Label for="ep-group">Group</Label><Input id="ep-group" bind:value={s.group} />
					</div>
					<div class="space-y-1.5">
						<Label for="ep-int">Every (seconds)</Label><Input
							id="ep-int"
							type="number"
							min="10"
							bind:value={s.interval_seconds}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="ep-to">Timeout (seconds)</Label><Input
							id="ep-to"
							type="number"
							min="1"
							max="60"
							bind:value={s.timeout_seconds}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="ep-slow">Degraded over (ms, 0 = off)</Label><Input
							id="ep-slow"
							type="number"
							min="0"
							bind:value={s.degraded_ms}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="ep-route">Alerts go to</Label>
						<Select.Root
							type="single"
							value={s.route_provider_id ?? ''}
							onValueChange={(v) => s && (s.route_provider_id = v || undefined)}
						>
							<Select.Trigger id="ep-route" class="w-full"
								>{providers.find((p) => p.id === s?.route_provider_id)?.name ??
									'Default route'}</Select.Trigger
							>
							<Select.Content>
								<Select.Item value="">Default route</Select.Item>
								{#each providers as p (p.id)}<Select.Item value={p.id}>{p.name}</Select.Item>{/each}
							</Select.Content>
						</Select.Root>
					</div>
					<label class="col-span-2 flex items-center justify-between gap-3">
						<span>Show on the public status page</span><Switch
							bind:checked={s.public}
							aria-label="Show on the public status page"
						/>
					</label>
				</div>
				<div class="mt-3 flex justify-end gap-2">
					{#if m.source_type === 'user'}<Button
							variant="ghost"
							size="sm"
							class="text-bad-text"
							onclick={() => (confirming = true)}>Delete</Button
						>{/if}
					<Button size="sm" disabled={busy || !draft.dirty} onclick={() => save()}>Save</Button>
				</div>
			{/if}
		</Panel>
	</div>
{/if}

{#if d}<ConfirmDelete
		bind:open={confirming}
		title="Delete {d.endpoint.name}?"
		body="The check and all its history are deleted; this cannot be undone."
		verb="Delete check"
		onconfirm={remove}
	/>{/if}
