<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Sparkline from '$lib/components/hashi/Sparkline.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import AddressLink from '$lib/components/hashi/security/AddressLink.svelte';
	import Counters from './Counters.svelte';
	import { securityTop, securityEvents, spark } from '$lib/design/mock';
	import { Search, Plus, TriangleAlert } from '@lucide/svelte';

	let range = $state('24h');
	const tiles = [
		{ k: 'Allowed', v: '182,410', tone: 'text-ok-text', s: spark(2) },
		{ k: 'Challenged', v: '1,204', tone: 'text-warn-text', s: spark(5, 48, 20, 15) },
		{ k: 'Banned', v: '9,931', tone: 'text-bad-text', s: spark(9, 48, 30, 25) },
		{ k: 'WAF matches', v: '312', tone: 'text-info-text', s: spark(12, 48, 10, 9) }
	];
	const sevTone = { ok: 'ok', warn: 'warn', bad: 'bad' } as const;
	const max = Math.max(...securityTop.countries.map((c) => c.n));
	const f = securityTop.funnel;
	const funnel = [
		{ k: 'Challenged', n: f.triggered, tone: 'bg-warn/70' },
		{ k: 'Solved', n: f.solved, tone: 'bg-ok/70' },
		{ k: 'Expired unsolved', n: f.expired, tone: 'bg-idle' },
		{ k: 'Banned', n: f.banned, tone: 'bg-bad/70' }
	];
</script>

<PageHeader
	title="Security"
	description="Every web request passes the abuse engine first, then the service’s rules. This is what was let through, challenged and banned, and why."
>
	{#snippet actions()}
		<form class="relative w-72" action="/design/ip-sets/address">
			<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
			<Input
				name="ip"
				placeholder="Look up an address"
				class="pl-8 font-mono"
				aria-label="Look up an address"
			/>
		</form>
	{/snippet}
</PageHeader>

<Tabs.Root value="dash">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="dash">Dashboard</Tabs.Trigger>
		<Tabs.Trigger value="counters">Counters</Tabs.Trigger>
		<Tabs.Trigger value="captcha">Challenge</Tabs.Trigger>
		<Tabs.Trigger value="sso">Sign-in providers</Tabs.Trigger>
		<Tabs.Trigger value="engine">Engine</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="dash" class="space-y-4">
		<div class="flex flex-wrap items-center gap-2">
			<ToggleGroup.Root type="single" bind:value={range} variant="outline" size="sm"
				>{#each ['1h', '24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}
						>{r}</ToggleGroup.Item
					>{/each}</ToggleGroup.Root
			>
			<span class="text-[12.5px] text-muted-foreground"
				>All services · all Traefik hosts · all firewall hosts</span
			>
			<span
				class="ml-auto inline-flex items-center gap-1.5 rounded-[10px] border border-warn/30 bg-warn/10 px-2.5 py-1 text-[12.5px] text-warn-text"
				><TriangleAlert class="size-3.5" />feed-dc-ranges is stale (9 h)</span
			>
		</div>
		<div
			class="grid grid-cols-2 divide-x divide-y rounded-2xl border bg-card md:grid-cols-4 md:divide-y-0"
		>
			{#each tiles as t (t.k)}
				<div class="p-4">
					<div class="text-[12.5px] text-muted-foreground">{t.k}</div>
					<div class="tnum text-[22px] font-semibold">{t.v}</div>
					<Sparkline values={t.s} width={200} class="{t.tone} mt-1 w-full" />
				</div>
			{/each}
		</div>
		<div class="grid gap-4 xl:grid-cols-3">
			<Panel title="Most refused addresses" class="xl:col-span-2" flush>
				<ul class="divide-y text-[13px]">
					{#each securityTop.ips as ip (ip.v)}
						<li
							class="grid grid-cols-[minmax(0,9rem)_4rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5 lg:grid-cols-[10rem_4rem_3rem_minmax(0,1fr)_minmax(0,1.2fr)_6rem]"
						>
							<AddressLink address={ip.v} />
							<span class="tnum text-right">{ip.count.toLocaleString()}</span>
							<span class="hidden text-muted-foreground lg:inline">{ip.country}</span>
							<span class="hidden truncate text-muted-foreground lg:inline">{ip.asn}</span>
							<span class="truncate">{ip.reason}</span>
							<span class="text-right text-muted-foreground">{ip.until}</span>
						</li>
					{/each}
				</ul>
			</Panel>
			<Panel title="Refused by country">
				<ul class="space-y-2 text-[13px]">
					{#each securityTop.countries as c (c.v)}
						<li class="grid grid-cols-[2.5rem_1fr_3.5rem] items-center gap-2">
							<span>{c.v}</span><span class="h-1.5 rounded-full bg-surface-2"
								><span
									class="block h-full rounded-full bg-bad/70"
									style="width:{(c.n / max) * 100}%"
								></span></span
							><span class="tnum text-right">{c.n.toLocaleString()}</span>
						</li>
					{/each}
				</ul>
				<div class="mt-4 mb-2 text-[12.5px] font-medium text-muted-foreground">
					Most targeted services
				</div>
				<ul class="space-y-1.5 text-[13px]">
					{#each securityTop.services as r (r.v)}<li class="flex justify-between">
							<span>{r.v}</span><span class="tnum">{r.n.toLocaleString()}</span>
						</li>{/each}
				</ul>
			</Panel>
		</div>
		<div class="grid gap-4 lg:grid-cols-2">
			<Panel
				title="Challenges"
				description="What happened to addresses that were asked to solve one"
			>
				<ul class="space-y-2 text-[13px]">
					{#each funnel as s (s.k)}
						<li class="grid grid-cols-[8.5rem_1fr_3.5rem] items-center gap-2">
							<span>{s.k}</span><span class="h-1.5 rounded-full bg-surface-2"
								><span
									class="block h-full rounded-full {s.tone}"
									style="width:{(s.n / f.triggered) * 100}%"
								></span></span
							><span class="tnum text-right">{s.n.toLocaleString()}</span>
						</li>
					{/each}
				</ul>
				<p class="mt-3 text-[12.5px] text-muted-foreground">
					Many bans compared to solves can mean a background app keeps polling while challenged.
				</p>
			</Panel>
			<Panel title="Kernel meters" description="Addresses caught on the firewall hosts">
				<ul class="divide-y text-[13px]">
					{#each securityTop.meters as m (m.host)}
						<li class="grid grid-cols-[minmax(0,1fr)_repeat(3,5.5rem)] gap-2 py-2">
							<span class="font-medium">{m.host}</span><span class="tnum text-right"
								>{m.tcpRate} <span class="text-muted-foreground">new TCP</span></span
							><span class="tnum text-right"
								>{m.tcpConc} <span class="text-muted-foreground">open TCP</span></span
							><span class="tnum text-right"
								>{m.udp} <span class="text-muted-foreground">UDP</span></span
							>
						</li>
					{/each}
				</ul>
			</Panel>
		</div>
		<Panel title="Recent events" flush>
			<ul class="divide-y text-[13px]">
				{#each securityEvents as e (e.t)}
					<li
						class="grid grid-cols-[4.5rem_minmax(0,9rem)_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5 md:grid-cols-[4.5rem_10rem_minmax(0,1fr)_minmax(0,12rem)_6rem]"
					>
						<span class="tnum text-muted-foreground">{e.t}</span><AddressLink
							address={e.subject}
							class="truncate"
						/><span class="truncate">{e.type}</span><span
							class="hidden truncate text-muted-foreground md:inline">{e.resource}</span
						><Tag tone={sevTone[e.sev as keyof typeof sevTone]}>{e.decision}</Tag>
					</li>
				{/each}
			</ul>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="counters"><Counters /></Tabs.Content>

	<Tabs.Content value="captcha" class="grid gap-4 lg:grid-cols-2">
		<Panel title="Cap challenge" description="Uses your existing Cap instance">
			<div class="space-y-3">
				<div class="space-y-1.5">
					<Label>Challenge address</Label><Input value="https://check.example.com" />
				</div>
				<div class="space-y-1.5">
					<Label>Site key</Label><Input value="a1b2c3d4e5" class="font-mono" />
				</div>
				<div class="space-y-1.5">
					<Label>Secret key</Label><Input value="••••••••••••" disabled />
					<p class="text-[12.5px] text-muted-foreground">
						Stored in the vault. Replacing it asks for your passkey.
					</p>
				</div>
				<div class="flex gap-2 pt-1">
					<Button size="sm" variant="outline">Test verification</Button><Button size="sm"
						>Save</Button
					>
				</div>
			</div>
		</Panel>
		<Panel title="Today">
			<dl class="grid grid-cols-3 gap-3 text-[13px]">
				<div>
					<dt class="text-muted-foreground">Solved</dt>
					<dd class="tnum text-[20px] font-semibold">212</dd>
				</div>
				<div>
					<dt class="text-muted-foreground">Failed</dt>
					<dd class="tnum text-[20px] font-semibold">31</dd>
				</div>
				<div>
					<dt class="text-muted-foreground">Ignored</dt>
					<dd class="tnum text-[20px] font-semibold">960</dd>
				</div>
			</dl>
			<p class="mt-3 text-[12.5px] text-muted-foreground">
				Solving ends the challenge but leaves the counters alone. 15 requests without solving lead
				to a ban. A signed-in user never sees a CAPTCHA.
			</p>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="sso">
		<Panel
			title="Sign-in providers"
			description="One login on .example.com covers every protected subdomain"
			flush
		>
			{#snippet actions()}<Button size="sm" variant="outline"><Plus />Add provider</Button
				>{/snippet}
			<ul class="divide-y text-[13px]">
				<li class="flex items-center gap-3 px-4 py-3">
					<span class="flex-1"
						><span class="font-medium">Pocket ID</span>
						<Tag>default</Tag><span class="block text-muted-foreground"
							>https://sso.example.com · scopes openid email groups</span
						></span
					><Tag tone="bad">discovery failing</Tag>
				</li>
			</ul>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="engine" class="grid gap-4 lg:grid-cols-2">
		<Panel
			title="Abuse engine"
			description="Applies to every service. Saved in Settings → Security."
		>
			<dl class="grid grid-cols-[1fr_auto] gap-y-2.5 text-[13px]">
				<dt>Challenge after WAF matches</dt>
				<dd class="tnum">5 in 60 s</dd>
				<dt>Challenge after requests</dt>
				<dd class="tnum">300 in 10 s</dd>
				<dt>Challenge lasts</dt>
				<dd class="tnum">30 min</dd>
				<dt>Ban after requests without solving</dt>
				<dd class="tnum">15</dd>
				<dt>CAPTCHA pass lasts</dt>
				<dd class="tnum">24 h</dd>
				<dt>Counters drop by one after</dt>
				<dd class="tnum">7 quiet days</dd>
			</dl>
			<p class="mt-3 text-[12.5px] text-muted-foreground">
				Addresses in the firewall allow and signed-in users never trigger the request limit.
			</p>
		</Panel>
		<Panel
			title="Ban length"
			description="Exponential: 15 min × 2 per earlier ban, at most 30 days"
		>
			<ol class="grid grid-cols-[3rem_1fr_4.5rem] items-center gap-x-2 gap-y-1.5 text-[13px]">
				{#each ['15 min', '30 min', '1 h', '2 h', '4 h', '8 h', '16 h', '1 d 8 h', '2 d 16 h', '5 d 8 h', '10 d 16 h', '21 d 8 h'] as t, i (i)}
					<li class="contents">
						<span class="tnum text-muted-foreground">{i + 1}.</span><span
							class="h-1.5 rounded-full bg-surface-2"
							><span
								class="block h-full rounded-full bg-bad/60"
								style="width:{((i + 1) / 12) * 100}%"
							></span></span
						><span class="tnum text-right">{t}</span>
					</li>
				{/each}
			</ol>
		</Panel>
		<Panel title="GeoIP">
			<p class="text-[13px]">
				MaxMind GeoLite2 Country + ASN · updated 6 days ago. Without it, country and ASN items in IP
				sets match nobody.
			</p>
			<label class="mt-3 flex items-center gap-2 text-[13px]"><Switch checked />Update weekly</label
			>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
