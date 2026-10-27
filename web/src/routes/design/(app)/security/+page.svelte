<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Sparkline from '$lib/components/hashi/Sparkline.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import { securityTop, securityEvents, blocklists, spark } from '$lib/design/mock';
	import { Search, Plus, TriangleAlert } from '@lucide/svelte';

	let range = $state('24h');
	const tiles = [
		{ k: 'Allowed', v: '182,410', tone: 'text-ok-text', s: spark(2) },
		{ k: 'Challenged', v: '1,204', tone: 'text-warn-text', s: spark(5, 48, 20, 15) },
		{ k: 'Blocked', v: '9,931', tone: 'text-bad-text', s: spark(9, 48, 30, 25) },
		{ k: 'WAF detections', v: '312', tone: 'text-info-text', s: spark(12, 48, 10, 9) }
	];
	const sevTone = { ok: 'ok', warn: 'warn', bad: 'bad' } as const;
	const max = Math.max(...securityTop.countries.map((c) => c.n));
</script>

<PageHeader
	title="Security"
	description="Every request to a protected resource is decided by Hashi: allow, sign in, challenge or block. This is what it decided and why."
>
	{#snippet actions()}
		<form class="relative w-72" action="/design/security/subject">
			<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
			<Input
				name="q"
				placeholder="IP, CIDR, ASN or country"
				class="pl-8"
				aria-label="Find a subject"
			/>
		</form>
	{/snippet}
</PageHeader>

<Tabs.Root value="dash">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="dash">Dashboard</Tabs.Trigger>
		<Tabs.Trigger value="blocks">Blocks and allows</Tabs.Trigger>
		<Tabs.Trigger value="lists">Blocklists</Tabs.Trigger>
		<Tabs.Trigger value="captcha">Challenge</Tabs.Trigger>
		<Tabs.Trigger value="sso">Sign-in providers</Tabs.Trigger>
		<Tabs.Trigger value="policy">Policies</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="dash" class="space-y-4">
		<div class="flex flex-wrap items-center gap-2">
			<ToggleGroup.Root type="single" bind:value={range} variant="outline" size="sm"
				>{#each ['1h', '24h', '7d', '30d'] as r (r)}<ToggleGroup.Item value={r}
						>{r}</ToggleGroup.Item
					>{/each}</ToggleGroup.Root
			>
			<span class="text-[12.5px] text-muted-foreground"
				>All resources · all Traefik hosts · all firewall hosts</span
			>
			<span
				class="ml-auto inline-flex items-center gap-1.5 rounded-[10px] border border-warn/30 bg-warn/10 px-2.5 py-1 text-[12.5px] text-warn-text"
				><TriangleAlert class="size-3.5" />FireHOL Level 1 is stale (3 d)</span
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
			<Panel title="Most blocked addresses" class="xl:col-span-2" flush>
				<ul class="divide-y text-[13px]">
					{#each securityTop.ips as ip (ip.v)}
						<li
							class="grid grid-cols-[minmax(0,9rem)_4rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5 lg:grid-cols-[10rem_4rem_3rem_minmax(0,1fr)_minmax(0,1.2fr)_6rem]"
						>
							<a href="/design/security/subject" class="hover:underline"><Mono>{ip.v}</Mono></a>
							<span class="tnum text-right">{ip.count.toLocaleString()}</span>
							<span class="hidden text-muted-foreground lg:inline">{ip.country}</span>
							<span class="hidden truncate text-muted-foreground lg:inline">{ip.asn}</span>
							<span class="truncate">{ip.reason}</span>
							<span class="text-right text-muted-foreground">{ip.until}</span>
						</li>
					{/each}
				</ul>
			</Panel>
			<Panel title="Blocked by country">
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
					Most targeted resources
				</div>
				<ul class="space-y-1.5 text-[13px]">
					{#each securityTop.resources as r (r.v)}<li class="flex justify-between">
							<span>{r.v}</span><span class="tnum">{r.n.toLocaleString()}</span>
						</li>{/each}
				</ul>
			</Panel>
		</div>
		<Panel title="Recent events" flush>
			<ul class="divide-y text-[13px]">
				{#each securityEvents as e (e.t)}
					<li
						class="grid grid-cols-[4.5rem_minmax(0,9rem)_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5 md:grid-cols-[4.5rem_10rem_minmax(0,1fr)_minmax(0,12rem)_6rem]"
					>
						<span class="tnum text-muted-foreground">{e.t}</span><Mono class="truncate"
							>{e.subject}</Mono
						><span class="truncate">{e.type}</span><span
							class="hidden truncate text-muted-foreground md:inline">{e.resource}</span
						><Tag tone={sevTone[e.sev as keyof typeof sevTone]}>{e.decision}</Tag>
					</li>
				{/each}
			</ul>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="blocks">
		<Panel
			title="Blocks and allows"
			description="Firewall blocks are applied through a plan, never straight from this list"
			flush
		>
			{#snippet actions()}<Button size="sm" variant="outline"><Plus />Allow</Button><Button
					size="sm"><Plus />Block</Button
				>{/snippet}
			<ul class="divide-y text-[13px]">
				{#each securityTop.ips as ip (ip.v)}
					<li class="flex flex-wrap items-center gap-3 px-4 py-2.5">
						<Mono class="w-36">{ip.v}</Mono><span class="min-w-0 flex-1">{ip.reason}</span><span
							class="text-muted-foreground">{ip.until}</span
						><Button size="xs" variant="ghost">Extend</Button><Button size="xs" variant="ghost"
							>Expire now</Button
						>
					</li>
				{/each}
				<li class="flex flex-wrap items-center gap-3 px-4 py-2.5">
					<Mono class="w-36">192.0.2.0/24</Mono><span class="min-w-0 flex-1"
						>Manual allow · Office · skips blocking and escalation, not sign-in or challenge</span
					><Tag tone="ok">allow</Tag>
				</li>
			</ul>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="lists">
		<Panel
			title="Blocklists"
			description="All off until you pick them. Each can cause false positives; a failed update keeps the last good copy."
			flush
		>
			{#snippet actions()}<Button size="sm" variant="outline"><Plus />Custom URL</Button>{/snippet}
			<ul class="divide-y text-[13px]">
				{#each blocklists as b (b.name)}
					<li
						class="grid items-center gap-3 px-4 py-2.5 md:grid-cols-[minmax(0,1fr)_9rem_10rem_7rem_auto]"
					>
						<span
							><span class="font-medium">{b.name}</span><span
								class="block text-[12.5px] text-muted-foreground">false-positive risk: {b.fp}</span
							></span
						>
						<span class="tnum">{b.entries ? b.entries.toLocaleString() + ' entries' : '—'}</span>
						<span class="text-muted-foreground">{b.mode}</span>
						<span
							>{#if b.status === 'stale'}<Tag tone="warn">stale · {b.updated}</Tag>{:else}<span
									class="text-muted-foreground">{b.updated}</span
								>{/if}</span
						>
						<Switch checked={b.enabled} aria-label="Enable {b.name}" />
					</li>
				{/each}
			</ul>
		</Panel>
	</Tabs.Content>

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
				Solving clears the challenge but never the offence history. Ignoring 30 challenges in 5 min
				leads to a 10 min soft block.
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

	<Tabs.Content value="policy" class="grid gap-4 lg:grid-cols-2">
		<Panel title="Escalation" description="Defaults; a resource can override">
			<dl class="grid grid-cols-[1fr_auto] gap-y-2.5 text-[13px]">
				<dt>Challenge when a subject exceeds</dt>
				<dd class="tnum">300 req / 5 min</dd>
				<dt>Soft block after ignored challenges</dt>
				<dd class="tnum">30 in 5 min → 10 min</dd>
				<dt>Firewall block while challenged</dt>
				<dd class="tnum">100 in 5 min</dd>
				<dt>Repeat offenders</dt>
				<dd>capped exponential ×2, max 7 d</dd>
			</dl>
		</Panel>
		<Panel title="GeoIP">
			<p class="text-[13px]">
				MaxMind GeoLite2 Country + ASN · updated 6 days ago. Country and ASN rules stop working when
				the database is missing.
			</p>
			<label class="mt-3 flex items-center gap-2 text-[13px]"><Switch checked />Update weekly</label
			>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
