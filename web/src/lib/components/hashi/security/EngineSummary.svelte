<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Pencil } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';

	// The abuse engine's numbers (06 §10), as the mock shows them. Editing saves with the passkey; the firewall and the
	// Traefik hosts are planned again (Pending changes).
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
	const num = (label: string, key: keyof Schemas['EngineSettings'], unit = '') => ({
		label,
		key,
		unit
	});
	const fields = [
		num('Challenge after requests', 'rate_requests'),
		num('… within', 'rate_window_seconds', 's'),
		num('Challenge after WAF matches', 'waf_violations'),
		num('… within', 'waf_window_seconds', 's'),
		num('Challenge lasts (0: until solved)', 'challenge_expiry_seconds', 's'),
		num('Ban after requests without solving', 'challenge_max_failed'),
		num('CAPTCHA pass lasts', 'captcha_clearance_hours', 'h'),
		num('New connections per minute from one address', 'tcp_new_per_minute'),
		num('… burst', 'tcp_burst'),
		num('Open connections from one address', 'tcp_max_concurrent'),
		num('First ban', 'ban_base_seconds', 's'),
		num('Ban factor', 'ban_k'),
		num('Longest ban', 'ban_max_seconds', 's'),
		num('Permanent from ban number (0: never)', 'ban_permanent_after'),
		num('Counters drop by one after quiet', 'counter_decay_seconds', 's')
	];
	onMount(async () => {
		const r = await api.GET('/api/settings/engine');
		if (r.error || !r.data) error = problemText(r.error);
		else e = r.data;
	});

	function span(s: number): string {
		if (s % 86400 === 0) return `${s / 86400} d`;
		if (s >= 86400) return `${Math.floor(s / 86400)} d ${Math.round((s % 86400) / 3600)} h`;
		if (s % 3600 === 0) return `${s / 3600} h`;
		if (s >= 60) return `${Math.round(s / 60)} min`;
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
	<div class="grid gap-4 lg:grid-cols-2">
		<Panel title="Abuse engine" description="Applies to every service">
			{#snippet actions()}<Button size="sm" variant="outline" onclick={() => (form = { ...e! })}
					><Pencil />Edit</Button
				>{/snippet}
			<dl class="grid grid-cols-[1fr_auto] gap-y-2.5 text-[13px]">
				<dt>Challenge after WAF matches</dt>
				<dd class="tnum">{e.waf_violations} in {span(e.waf_window_seconds)}</dd>
				<dt>Challenge after requests</dt>
				<dd class="tnum">{e.rate_requests} in {span(e.rate_window_seconds)}</dd>
				<dt>Challenge lasts</dt>
				<dd class="tnum">
					{e.challenge_expiry_seconds ? span(e.challenge_expiry_seconds) : 'until solved'}
				</dd>
				<dt>Ban after requests without solving</dt>
				<dd class="tnum">{e.challenge_max_failed}</dd>
				<dt>CAPTCHA pass lasts</dt>
				<dd class="tnum">{span(e.captcha_clearance_hours * 3600)}</dd>
				<dt>Counters drop by one after</dt>
				<dd class="tnum">{span(e.counter_decay_seconds)} quiet</dd>
				<dt>Web firewall by default</dt>
				<dd>{e.waf_default_mode.replace('_', ' ')}</dd>
				<dt>Connection limits in the firewall</dt>
				<dd>
					{e.meters_enabled
						? `${e.tcp_new_per_minute}/min, ${e.tcp_max_concurrent} open per address`
						: 'off'}
				</dd>
				<dt>Blocklists refresh every</dt>
				<dd class="tnum">{span(e.feed_refresh_seconds)}</dd>
			</dl>
			<p class="mt-3 text-[12.5px] text-muted-foreground">
				Addresses in the firewall allow and signed-in users never trigger the request limit.
			</p>
		</Panel>
		<Panel
			title="Ban length"
			description="{e.ban_function} from {span(e.ban_base_seconds)}, at most {span(
				e.ban_max_seconds
			)}"
		>
			<ol class="grid grid-cols-[3rem_1fr_4.5rem] items-center gap-x-2 gap-y-1.5 text-[13px]">
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
	</div>
{:else if error}
	<p role="alert" class="text-[13px] text-bad-text">{error}</p>
{/if}

<Sheet.Root open={!!form} onOpenChange={(o) => !o && (form = null)}>
	<Sheet.Content class="w-full sm:max-w-lg">
		<Sheet.Header>
			<Sheet.Title>Abuse engine</Sheet.Title>
			<Sheet.Description
				>Saving asks for your passkey. Firewall and Traefik changes then wait under Pending changes.</Sheet.Description
			>
		</Sheet.Header>
		{#if form}
			<div class="space-y-4 overflow-y-auto px-4 pb-4">
				<div class="space-y-1.5">
					<Label for="eg-waf">Web firewall for services without their own setting</Label>
					<Select.Root type="single" bind:value={form.waf_default_mode}>
						<Select.Trigger id="eg-waf" class="w-full"
							>{form.waf_default_mode === 'headers_only' ? 'headers only' : 'off'}</Select.Trigger
						>
						<Select.Content>
							<Select.Item value="headers_only">headers only</Select.Item>
							<Select.Item value="disabled">off</Select.Item>
						</Select.Content>
					</Select.Root>
				</div>
				<label class="flex items-center gap-2 text-[13px]"
					><Switch bind:checked={form.meters_enabled} />Connection limits in the firewall</label
				>
				<div class="grid gap-3 sm:grid-cols-2">
					{#each fields as f (f.key + f.label)}
						<div class="space-y-1.5">
							<Label for="eg-{f.key}">{f.label}{f.unit ? ` (${f.unit})` : ''}</Label>
							<Input
								id="eg-{f.key}"
								type="number"
								step={f.key === 'ban_k' ? '0.1' : '1'}
								bind:value={form[f.key] as number}
							/>
						</div>
					{/each}
				</div>
				<div class="space-y-1.5">
					<Label for="eg-fn">Ban length grows</Label>
					<Select.Root type="single" bind:value={form.ban_function}>
						<Select.Trigger id="eg-fn" class="w-full">{form.ban_function}</Select.Trigger>
						<Select.Content>
							<Select.Item value="constant">constant</Select.Item>
							<Select.Item value="linear">linear</Select.Item>
							<Select.Item value="exponential">exponential</Select.Item>
						</Select.Content>
					</Select.Root>
				</div>
				{#if formError}<p role="alert" class="text-[13px] text-bad-text">{formError}</p>{/if}
			</div>
			<Sheet.Footer class="flex-row justify-end gap-2 border-t">
				<Button variant="ghost" onclick={() => (form = null)}>Cancel</Button>
				<Button disabled={busy} onclick={save}>Save</Button>
			</Sheet.Footer>
		{/if}
	</Sheet.Content>
</Sheet.Root>
