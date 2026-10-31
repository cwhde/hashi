<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import LineChart from '$lib/components/hashi/LineChart.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import { ChevronLeft } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { checkSummary, sourceText, stripHealth, uptimeText } from '$lib/monitor';
	import { dateTime } from '$lib/format';

	// 07 §5: status, response numbers, uptime, charts by range, events, settings.
	const id = $derived(page.params.id ?? '');
	let d = $state<Schemas['MonitorEndpointDetail'] | null>(null);
	let providers = $state<Schemas['NotificationProvider'][]>([]);
	let s = $state<Schemas['MonitorEndpointSettings'] | null>(null);
	let range = $state<'1h' | '24h' | '7d' | '30d'>('24h');
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);

	async function load() {
		const r = await api.GET('/api/monitor/endpoints/{id}', {
			params: { path: { id }, query: { range } }
		});
		if (r.error || !r.data) throw r.error;
		d = r.data;
		if (!s || s.paused !== r.data.endpoint.paused) {
			const e = r.data.endpoint;
			s = {
				group: e.group,
				paused: e.paused,
				public: e.public,
				interval_seconds: e.interval_seconds,
				timeout_seconds: e.timeout_seconds,
				degraded_ms: e.degraded_ms,
				route_provider_id: e.route_provider_id
			};
		}
	}
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
	async function remove() {
		const r = await api.DELETE('/api/monitor/endpoints/{id}', { params: { path: { id } } });
		if (r.error) error = problemText(r.error);
		else goto('/status');
	}
	const ms = (v: number | undefined) => (v === undefined ? 'no data' : `${v} ms`);
	const series = $derived(d?.series ?? []);
	const gaps = $derived(series.flatMap((p) => (p.gap_ms === undefined ? [] : [p.gap_ms])));
</script>

<svelte:head><title>{d?.endpoint.name ?? 'Endpoint'} · Hashi</title></svelte:head>

<a
	href="/status"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Status</a
>
{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}
{#if notice}<p role="status" class="mb-3 text-[13px] text-ok-text">{notice}</p>{/if}
{#if !d}
	{#if !error}<Skeleton class="h-48" />{/if}
{:else}
	{@const m = d.endpoint}
	<div class="mb-4 rounded-2xl border bg-card p-4">
		<div class="flex flex-wrap items-start gap-4">
			<div class="min-w-0 flex-1">
				<h1 class="text-[22px] font-semibold">{m.name}</h1>
				<p class="truncate text-[13.5px] text-muted-foreground">
					{checkSummary(m)} · every {m.interval_seconds} s
				</p>
			</div>
			<StatusBadge state={m.state} />
			{#if s}
				<Button
					variant="outline"
					size="sm"
					disabled={busy}
					onclick={() => s && save({ ...s, paused: !s.paused })}
					>{m.paused ? 'Resume' : 'Pause'}</Button
				>
			{/if}
		</div>
		<dl class="mt-4 grid grid-cols-2 gap-3 text-[13px] sm:grid-cols-4 lg:grid-cols-8">
			{#each [['Last check', m.last_check_at ? dateTime(m.last_check_at) : 'not yet'], ['Response min', ms(d.min_ms)], ['avg', ms(d.avg_ms)], ['max', ms(d.max_ms)], ['Uptime 1 h', uptimeText(d.uptime_1h)], ['24 h', uptimeText(m.uptime_24h)], ['7 d', uptimeText(d.uptime_7d)], ['30 d', uptimeText(d.uptime_30d)]] as [k, v] (k)}
				<div>
					<dt class="text-[12px] text-muted-foreground">{k}</dt>
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
			onValueChange={(v) => v && (range = v as typeof range)}
			variant="outline"
			size="sm"
		>
			{#each ['1h', '24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}>{r}</ToggleGroup.Item
				>{/each}
		</ToggleGroup.Root>
	</div>
	{#if series.length === 0}
		<p class="mb-4 text-center text-[13px] text-muted-foreground">
			Charts appear after the first full minute of checks.
		</p>
	{:else}
		<div class="mb-4 grid gap-4 lg:grid-cols-3">
			<LineChart title="Response time" values={series.map((p) => p.latency_ms)} unit=" ms" />
			{#if gaps.length}
				<LineChart
					title="Public vs internal gap"
					values={gaps.map((g) => Math.max(0, g))}
					current="{gaps.at(-1)} ms"
				/>
			{/if}
			<LineChart
				title="Availability"
				values={series.map((p) => p.availability)}
				current={uptimeText(series.at(-1)?.availability)}
			/>
		</div>
	{/if}
	<div class="grid gap-4 lg:grid-cols-2">
		<Panel title="Events">
			{#if d.events.length === 0}<p class="text-[13px] text-muted-foreground">
					No changes yet.
				</p>{/if}
			<ol class="space-y-2.5 text-[13px]">
				{#each d.events as e (e.id)}
					<li class="grid gap-x-3 sm:grid-cols-[9rem_1fr]">
						<span class="tnum text-muted-foreground">{dateTime(e.at)}</span>
						<span class="min-w-0 break-words"><StatusBadge state={e.to} /> {e.message}</span>
					</li>
				{/each}
			</ol>
		</Panel>
		<Panel
			title="Checks and settings"
			description="{sourceText[m.source_type]}: its checks follow what it watches."
		>
			<ul class="mb-4 space-y-1.5 text-[13px]">
				{#each m.checks as c (c.id)}
					<li class="min-w-0 break-words">
						<span class="font-medium">{c.role ? `${c.role} path` : c.type}</span>
						<span class="text-muted-foreground">{c.target}</span>
						{#if c.last_error}<span class="block text-bad-text">{c.last_error}</span>{/if}
					</li>
				{/each}
			</ul>
			{#if s}
				<div class="grid grid-cols-2 gap-3 text-[13px]">
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
					{#if m.source_type === 'user'}<Button variant="ghost" size="sm" onclick={remove}
							>Delete</Button
						>{/if}
					<Button size="sm" disabled={busy} onclick={() => save()}>Save</Button>
				</div>
			{/if}
		</Panel>
	</div>
{/if}
