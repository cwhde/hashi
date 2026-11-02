<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';

	// The abuse engine's numbers (06 §10), as the mock shows them: read here, defaults until a settings editor exists.
	let e = $state<Schemas['EngineSettings'] | null>(null);
	let error = $state('');
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
				<dd>{e.meters_enabled ? 'on' : 'off'}</dd>
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
