<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import HelpTip from '#lib/components/hashi/HelpTip.svelte';
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import { Pencil } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';

	// The abuse engine's numbers (06 §10), as the mock shows them. Editing saves with the passkey; the firewall and the
	// Traefik hosts are planned again (Pending changes).
	// Cards placed under each column (bug 142): two columns, so a short card leaves no gap.
	let { left, right }: { left?: Snippet; right?: Snippet } = $props();
	let e = $state<Schemas['EngineSettings'] | null>(null);
	let error = $state('');
	let form = $state<Schemas['EngineSettings'] | null>(null);
	let formError = $state('');
	let busy = $state(false);
	async function save() {
		if (!form) return;
		busy = true;
		formError = '';
		try {
			e = await withReauth(() => api.PUT('/api/settings/engine', { body: form! }));
			form = null;
		} catch (err) {
			formError = problemText(err);
		} finally {
			busy = false;
		}
	}
	type Key = keyof Schemas['EngineSettings'];
	// One row per setting: a complete name, the boxes, and their units (bug 140). Durations show in a readable unit and
	// are stored in seconds; factor converts.
	type Box = { key: Key; unit: string; factor?: number; step?: string; joiner?: string };
	type Row = { label: string; boxes: Box[]; note?: string };
	const groups: { title: string; meters?: boolean; rows: Row[] }[] = [
		{
			title: 'Challenge',
			rows: [
				{
					label: 'Request limit',
					boxes: [
						{ key: 'rate_requests', unit: 'requests' },
						{ key: 'rate_window_seconds', unit: 's', joiner: 'in' }
					]
				},
				{
					label: 'Error answers',
					boxes: [
						{ key: 'error_requests', unit: 'errors' },
						{ key: 'error_window_seconds', unit: 's', joiner: 'in' }
					]
				},
				{
					label: 'Web firewall matches',
					boxes: [
						{ key: 'waf_violations', unit: 'matches' },
						{ key: 'waf_window_seconds', unit: 's', joiner: 'in' }
					]
				},
				{
					label: 'Challenge expires after',
					boxes: [{ key: 'challenge_expiry_seconds', unit: 'min', factor: 60 }],
					note: '0 keeps it until solved'
				},
				{
					label: 'Attempts before a ban',
					boxes: [{ key: 'challenge_max_failed', unit: 'attempts' }]
				},
				{
					label: 'CAPTCHA pass lasts',
					boxes: [{ key: 'captcha_clearance_hours', unit: 'h' }]
				}
			]
		},
		{
			title: 'Connection limits per address',
			meters: true,
			rows: [
				{
					label: 'New connections',
					boxes: [{ key: 'tcp_new_per_minute', unit: 'per min' }]
				},
				{ label: 'Brief spikes up to', boxes: [{ key: 'tcp_burst', unit: 'connections' }] },
				{
					label: 'Open connections',
					boxes: [{ key: 'tcp_max_concurrent', unit: 'at once' }]
				}
			]
		},
		{
			title: 'Bans',
			rows: [
				{ label: 'First ban', boxes: [{ key: 'ban_base_seconds', unit: 'min', factor: 60 }] },
				{ label: 'Each next ban', boxes: [{ key: 'ban_k', unit: '× longer', step: '0.1' }] },
				{
					label: 'Longest ban',
					boxes: [{ key: 'ban_max_seconds', unit: 'days', factor: 86400, step: '0.5' }]
				},
				{
					label: 'Permanent after',
					boxes: [{ key: 'ban_permanent_after', unit: 'bans' }],
					note: 'Leave at 0 to never ban permanently'
				},
				{
					label: 'Forget one offence after',
					boxes: [{ key: 'counter_decay_seconds', unit: 'quiet days', factor: 86400, step: '0.5' }]
				}
			]
		}
	];
	const growth: Record<string, string> = {
		constant: 'Same length',
		linear: 'Fixed step',
		exponential: 'Multiplied'
	};
	const growthText = (x: Schemas['EngineSettings']) =>
		x.ban_function === 'exponential'
			? `×${x.ban_k} each time`
			: x.ban_function === 'linear'
				? `+${span(x.ban_base_seconds * x.ban_k)} each time`
				: 'Same length each time';
	const read = (b: Box) => (form![b.key] as number) / (b.factor ?? 1);
	const write = (b: Box, v: number) => {
		const n = Number(v) * (b.factor ?? 1);
		(form as Record<string, unknown>)[b.key] = b.key === 'ban_k' ? n : Math.round(n);
	};
	onMount(async () => {
		const r = await api.GET('/api/settings/engine');
		if (r.error || !r.data) error = problemText(r.error);
		else e = r.data;
	});

	function span(s: number): string {
		if (s % 86400 === 0) return `${s / 86400} d`;
		if (s >= 86400) return `${Math.floor(s / 86400)} d ${Math.round((s % 86400) / 3600)} h`;
		if (s % 3600 === 0) return `${s / 3600} h`;
		if (s >= 120 || s % 60 === 0) return `${Math.round(s / 60)} min`;
		return `${s} s`;
	}
	const bans = $derived.by(() => {
		if (!e) return [];
		const out: string[] = [];
		for (let c = 1; c <= 12; c++) {
			if (e.ban_permanent_after > 0 && c >= e.ban_permanent_after) {
				out.push('permanent');
				break;
			}
			const raw =
				e.ban_function === 'constant'
					? e.ban_base_seconds
					: e.ban_function === 'linear'
						? e.ban_base_seconds * (1 + e.ban_k * (c - 1))
						: e.ban_base_seconds * Math.pow(e.ban_k, c - 1);
			out.push(span(Math.min(raw, e.ban_max_seconds)));
		}
		return out;
	});
</script>

{#if e}
	<div class="grid items-start gap-4 lg:grid-cols-2">
		<div class="space-y-4">
			<Panel title="Abuse engine">
				{#snippet help()}<HelpTip topic="the abuse engine"
						><p>
							Addresses in the firewall's allow list and signed-in users never trigger the request
							limit.
						</p>
						<p>
							Error answers are the 4xx answers an address gets, including for names no service has.
							Scanners collect many; people using their apps collect a few.
						</p></HelpTip
					>{/snippet}
				{#snippet actions()}<Button size="sm" variant="outline" onclick={() => (form = { ...e! })}
						><Pencil />Edit</Button
					>{/snippet}
				<dl class="grid grid-cols-[1fr_auto] gap-x-4 gap-y-2.5 text-md">
					<dt>Request limit</dt>
					<dd class="tnum text-right">
						{e.rate_requests} requests in {span(e.rate_window_seconds)}
					</dd>
					<dt>Error answers</dt>
					<dd class="tnum text-right">{e.error_requests} in {span(e.error_window_seconds)}</dd>
					<dt>Web firewall matches</dt>
					<dd class="tnum text-right">{e.waf_violations} in {span(e.waf_window_seconds)}</dd>
					<dt>Challenge expires after</dt>
					<dd class="tnum text-right">
						{e.challenge_expiry_seconds ? span(e.challenge_expiry_seconds) : 'never'}
					</dd>
					<dt>Attempts before a ban</dt>
					<dd class="tnum text-right">{e.challenge_max_failed}</dd>
					<dt>CAPTCHA pass lasts</dt>
					<dd class="tnum text-right">{span(e.captcha_clearance_hours * 3600)}</dd>
					<dt>Forget one offence after</dt>
					<dd class="tnum text-right">{span(e.counter_decay_seconds)} quiet</dd>
					<dt>Default web firewall</dt>
					<dd class="text-right">
						{e.waf_default_mode === 'headers_only' ? 'Headers only' : 'Off'}
					</dd>
					<dt>Connection limits</dt>
					<dd class="tnum text-right">
						{e.meters_enabled
							? `${e.tcp_new_per_minute} new per min, ${e.tcp_max_concurrent} open per address`
							: 'Off'}
					</dd>
					<dt>Blocklists refresh every</dt>
					<dd class="tnum text-right">{span(e.feed_refresh_seconds)}</dd>
				</dl>
			</Panel>
			{@render left?.()}
		</div>
		<div class="space-y-4">
			<Panel
				title="Ban length"
				description="{growthText(e)}, from {span(e.ban_base_seconds)} up to {span(
					e.ban_max_seconds
				)}"
			>
				<ol class="grid grid-cols-[3rem_1fr_4.5rem] items-center gap-x-2 gap-y-1.5 text-md">
					{#each bans as t, i (i)}
						<li class="contents">
							<span class="tnum text-muted-foreground">{i + 1}.</span><span
								class="h-1.5 rounded-full bg-surface-2"
								><span
									class="block h-full rounded-full bg-bad/60"
									style="width:{((i + 1) / bans.length) * 100}%"
								></span></span
							><span class="tnum text-right">{t}</span>
						</li>
					{/each}
				</ol>
			</Panel>
			{@render right?.()}
		</div>
	</div>
{:else if error}
	<p role="alert" class="text-md text-bad-text">{error}</p>
{:else}<Skeleton class="h-40 rounded-2xl" />
{/if}

<Sheet.Root open={!!form} onOpenChange={(o) => !o && (form = null)}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Abuse engine</Sheet.Title>
		</Sheet.Header>
		{#if form}
			<div class="space-y-6 overflow-y-auto px-4 pb-4">
				<div class="flex items-center justify-between gap-4">
					<Label for="eg-waf">Default web firewall</Label>
					<Select.Root type="single" bind:value={form.waf_default_mode}>
						<Select.Trigger id="eg-waf" class="w-40"
							>{form.waf_default_mode === 'headers_only' ? 'Headers only' : 'Off'}</Select.Trigger
						>
						<Select.Content>
							<Select.Item value="headers_only">Headers only</Select.Item>
							<Select.Item value="disabled">Off</Select.Item>
						</Select.Content>
					</Select.Root>
				</div>
				{#each groups as g (g.title)}
					<section class="space-y-3">
						<div class="flex items-center justify-between gap-4 border-b pb-1.5">
							<h3 class="text-base font-semibold">{g.title}</h3>
							{#if g.meters}<Switch
									bind:checked={form.meters_enabled}
									aria-label="Connection limits in the firewall"
								/>{/if}
						</div>
						{#if !g.meters || form.meters_enabled}
							{#each g.rows as r (r.label)}
								<div class="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1">
									<Label for="eg-{r.boxes[0]?.key}" class="leading-snug">{r.label}</Label>
									<div class="flex flex-col items-end gap-1.5 text-md text-muted-foreground">
										{#each r.boxes as b (b.key)}
											<div class="flex items-center gap-1.5">
												{#if b.joiner}<span>{b.joiner}</span>{/if}
												<Input
													id="eg-{b.key}"
													type="number"
													min="0"
													step={b.step ?? '1'}
													class="tnum w-20 text-right"
													aria-label="{r.label} ({b.unit || 'number'})"
													bind:value={() => read(b), (v: number) => write(b, v)}
												/><span class="w-24">{b.unit}</span>
											</div>
										{/each}
									</div>
									{#if r.note}<p class="col-span-2 text-sm text-muted-foreground">{r.note}</p>{/if}
								</div>
							{/each}
						{/if}
					</section>
				{/each}
				<div class="flex items-center justify-between gap-4">
					<Label for="eg-fn">How bans grow</Label>
					<Select.Root type="single" bind:value={form.ban_function}>
						<Select.Trigger id="eg-fn" class="w-40">{growth[form.ban_function]}</Select.Trigger>
						<Select.Content>
							{#each Object.entries(growth) as [v, l] (v)}<Select.Item value={v}>{l}</Select.Item
								>{/each}
						</Select.Content>
					</Select.Root>
				</div>
				{#if formError}<p role="alert" class="text-md text-bad-text">{formError}</p>{/if}
			</div>
			<Sheet.Footer class="flex-row justify-end gap-2 border-t">
				<Button variant="ghost" onclick={() => (form = null)}>Cancel</Button>
				<Button disabled={busy} onclick={save}>Save</Button>
			</Sheet.Footer>
		{/if}
	</Sheet.Content>
</Sheet.Root>
