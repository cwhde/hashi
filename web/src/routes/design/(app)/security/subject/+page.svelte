<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { ChevronLeft, Check, Minus } from '@lucide/svelte';

	// The decision trace: every step the engine evaluated, in order, and the one that decided (06 §3.2).
	const trace = [
		{ step: 'Request metadata valid', r: 'pass' },
		{ step: 'Manual block', r: 'none' },
		{
			step: 'Firewall block',
			r: 'decided',
			detail: 'Active since 12:59, expires in 6 d 23 h (repeat offender ×3)'
		},
		{ step: 'Blocklist with firewall enforcement', r: 'skipped' },
		{ step: 'Soft block', r: 'skipped' },
		{ step: 'Challenge required', r: 'skipped' },
		{ step: 'Resource rules', r: 'skipped' },
		{ step: 'Manual allow', r: 'skipped' },
		{ step: 'Resource default', r: 'skipped' }
	];
	const timeline = [
		['12:59', 'Firewall block applied on edge-fra', 'bad'],
		['12:58', 'Escalated: 100 requests in 5 min while challenged', 'bad'],
		['12:41', 'Challenge required on git.example.com (rate)', 'warn'],
		['12:40', 'First seen · NL · AS64500 Example Hosting', 'neutral']
	] as const;
</script>

<a
	href="/design/security"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Security</a
>
<div class="mb-5 flex flex-wrap items-end gap-3">
	<div class="flex-1">
		<h1 class="font-mono text-[22px] font-semibold">192.0.2.201</h1>
		<p class="text-[13.5px] text-muted-foreground">
			NL · AS64500 Example Hosting · first seen today 12:40
		</p>
	</div>
	<Tag tone="bad">firewall blocked</Tag>
	<Button variant="outline" size="sm">Shorten</Button><Button variant="outline" size="sm"
		>Make permanent</Button
	><Button size="sm">Remove block…</Button>
</div>
<div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
	<Panel title="Why it is blocked" description="Decision order for a request right now" flush>
		<ol class="divide-y text-[13px]">
			{#each trace as t, i (i)}
				<li
					class="flex items-start gap-3 px-4 py-2 {t.r === 'decided'
						? 'bg-bad/[0.06]'
						: ''} {t.r === 'skipped' ? 'text-muted-foreground' : ''}"
				>
					<span class="mt-0.5 w-4 shrink-0"
						>{#if t.r === 'pass' || t.r === 'none'}<Check
								class="size-4 text-ok-text"
							/>{:else if t.r === 'decided'}<span
								class="block size-2.5 translate-y-1 rounded-full bg-bad"
							></span>{:else}<Minus class="size-4" />{/if}</span
					>
					<span class="flex-1"
						>{t.step}{#if t.detail}<span class="block text-foreground">{t.detail}</span>{/if}</span
					>
					<span class="text-[12px]"
						>{t.r === 'decided' ? 'decided: block' : t.r === 'skipped' ? 'not reached' : t.r}</span
					>
				</li>
			{/each}
		</ol>
	</Panel>
	<div class="space-y-4">
		<Panel title="Where the block is applied">
			<ul class="space-y-1.5 text-[13px]">
				<li class="flex justify-between">
					<span>edge-fra</span><span class="text-ok-text">applied 12:59 · 4,812 drops</span>
				</li>
				<li class="flex justify-between">
					<span>edge-home</span><span class="text-muted-foreground"
						>external NAT, edge decision only</span
					>
				</li>
			</ul>
		</Panel>
		<Panel title="Timeline">
			<ol class="space-y-2.5 text-[13px]">
				{#each timeline as [t, what, tone] (t)}<li class="grid grid-cols-[3.5rem_1fr] gap-2">
						<span class="tnum text-muted-foreground">{t}</span><span
							class={tone === 'bad' ? 'text-bad-text' : tone === 'warn' ? 'text-warn-text' : ''}
							>{what}</span
						>
					</li>{/each}
			</ol>
		</Panel>
		<Panel title="History"
			><p class="text-[13px]">
				3 firewall blocks and 5 challenges in 30 days. Offence counts are kept when a block expires
				or a challenge is solved.
			</p>
			<p class="mt-1 text-[12.5px] text-muted-foreground">
				Covered by <Mono>192.0.2.0/24</Mono>? No allow entry matches.
			</p></Panel
		>
	</div>
</div>
