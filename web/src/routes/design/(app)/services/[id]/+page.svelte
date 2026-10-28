<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import IpSetPicker from '$lib/components/hashi/security/IpSetPicker.svelte';
	import RuleEntryList from '$lib/components/hashi/security/RuleEntryList.svelte';
	import RuleItemEditor from '$lib/components/hashi/security/RuleItemEditor.svelte';
	import FlattenedRuleList from '$lib/components/hashi/security/FlattenedRuleList.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Select from '$lib/components/ui/select';
	import { serviceRoots, flatten, stageA, containsAddress, setBySlug } from '$lib/design/security';
	import { ChevronLeft, Plus, Lock, Layers, Unplug } from '@lucide/svelte';

	let { data } = $props();
	const r = $derived(data.r);
	const stream = $derived(['tcp', 'udp'].includes(r.kind));
	const root = $derived(serviceRoots[r.id] ?? { entries: [], fallback: 'none' as const });
	let fallback = $derived<'none' | 'deny'>(root.fallback);
	let advanced = $state(false);
	let entry = $derived(r.entry ?? r.host);
	const chain = $derived(
		entry === r.host
			? [`on.${r.host}`, `via.${r.host}`, r.host]
			: [`${entry}.ovr.on.${r.host}`, `via.${entry}`, entry]
	);
	let upstream = $state('media-upstream');
	let ruleOpen = $state(false);
	let tryIp = $state('203.0.113.54');
	let tryPath = $state('/');
	const rows = $derived([...stageA, ...flatten(r.id)]);
	const decides = $derived(
		rows.findIndex(
			(x, i) =>
				i >= stageA.length &&
				containsAddress(setBySlug(x.appliesTo ?? 'all')!, tryIp) &&
				(!x.condition || tryPath.startsWith(x.condition.pattern))
		)
	);
	const failClosed = $derived(
		fallback === 'deny' ||
			rows.some((x) => x.action === 'deny' || x.action === 'pass_authentication')
	);
	const wafPaths = $derived([
		{ path: '/', type: 'prefix', mode: r.waf, from: 'service' },
		...(r.id === 'r2'
			? [{ path: '/api/v1/repos', type: 'prefix', mode: 'enabled', from: 'path' }]
			: r.id === 'r4'
				? [{ path: '/api/assets', type: 'prefix', mode: 'headers_only', from: 'path' }]
				: [])
	]);
</script>

<a
	href="/design/services"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Services</a
>

<div class="mb-5 rounded-2xl border bg-card p-4">
	<div class="flex flex-wrap items-start gap-x-6 gap-y-3">
		<div class="min-w-0 flex-1">
			<h1 class="flex items-center gap-2 text-[22px] leading-tight font-semibold">
				{r.name}
				{#if r.origin === 'system'}<Tag
						><Lock class="size-3" />system · edit in Settings → General</Tag
					>{/if}
			</h1>
			<a class="text-[13.5px] text-muted-foreground hover:underline" href="https://{r.domain}"
				>{stream ? r.domain + ':' + r.port : 'https://' + r.domain}</a
			>
		</div>
		<div class="flex gap-2">
			<Button variant="outline" size="sm">{r.enabled ? 'Disable' : 'Enable'}</Button>
			<Button size="sm">Save changes</Button>
		</div>
	</div>
	<dl class="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 text-[13px] sm:grid-cols-3 lg:grid-cols-6">
		<div>
			<dt class="text-[12px] text-muted-foreground">Status</dt>
			<dd><StatusBadge state={r.health} /></dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Uptime 24 h</dt>
			<dd class="tnum">{r.uptime}</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Runs on · enters via</dt>
			<dd>{r.host} · {entry}</dd>
		</div>
		<div class="col-span-2 sm:col-span-1">
			<dt class="text-[12px] text-muted-foreground">Access</dt>
			<dd class="truncate">{r.access}</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">WAF</dt>
			<dd>{stream ? '—' : r.waf.replace('_', ' ')}</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Sync</dt>
			<dd>{r.sync === 'in sync' ? 'In sync on 2 hosts' : r.sync}</dd>
		</div>
	</dl>
	<HistoryStrip samples={r.history} class="mt-4" />
</div>

<Tabs.Root value={stream ? 'routing' : 'access'}>
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="routing">Routing</Tabs.Trigger>
		{#if !stream}<Tabs.Trigger value="access">Access</Tabs.Trigger><Tabs.Trigger value="waf"
				>WAF</Tabs.Trigger
			>{/if}
		<Tabs.Trigger value="monitoring">Monitoring</Tabs.Trigger>
		<Tabs.Trigger value="config">Rendered config</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="routing" class="grid gap-4 lg:grid-cols-2">
		<Panel title="Public address">
			<div class="space-y-4">
				<div class="space-y-1.5"><Label>Domain</Label><Input value={r.domain} /></div>
				<div class="space-y-1.5">
					<Label>Enter through</Label>
					<Select.Root type="single" bind:value={entry}>
						<Select.Trigger class="w-full"
							>{entry === r.host
								? `${r.host} (where it runs, host setting)`
								: entry}</Select.Trigger
						>
						<Select.Content>
							{#each ['edge-fra', 'edge-home'] as h (h)}<Select.Item value={h}
									>{h === r.host ? `${h} (where it runs, host setting)` : h}</Select.Item
								>{/each}
						</Select.Content>
					</Select.Root>
					<p class="text-[12.5px] text-muted-foreground">
						Runs on {r.host}: its target is in {r.host}’s subnets. Choosing another host overrides
						the host’s public entry for this service only.
					</p>
				</div>
				<div class="text-[13px]">
					<div class="mb-1 text-[12px] text-muted-foreground">DNS chain Hashi will keep</div>
					<ol class="space-y-0.5">
						<li><Mono>{r.domain}</Mono></li>
						{#each chain as c, i (c)}<li class="text-muted-foreground">
								→ <Mono class="text-foreground">{c}.example.com</Mono>{#if i === 2}<span>
										A</span
									>{/if}
							</li>{/each}
					</ol>
					{#if entry !== r.host}<p class="mt-1.5 text-[12.5px] text-muted-foreground">
							Shared with other {r.host} services entering through {entry}; removed when none does.
						</p>{/if}
				</div>
				{#if stream}<p class="text-[12.5px] text-muted-foreground">
						TCP and UDP services cannot use forward auth. They are protected by the firewall’s allow
						and block sets and the kernel meters for new connections and packet rates.
					</p>{/if}
			</div>
		</Panel>
		<Panel title="Upstream">
			{#snippet actions()}{#if !stream}<div class="flex items-center gap-2 text-[13px]">
						<Label for="adv">Advanced routes</Label><Switch id="adv" bind:checked={advanced} />
					</div>{/if}{/snippet}
			{#if !advanced}
				<div class="grid grid-cols-[6rem_1fr_6rem] gap-2">
					<Input value={r.kind} aria-label="Scheme" />
					{#if r.upstreamSet}<IpSetPicker bind:value={upstream} addressOnly />{:else}<Input
							value={r.target.replace(/^\w+:\/\//, '').split(':')[0]}
							aria-label="Target"
						/>{/if}
					<Input value={r.target.split(':').at(-1)} aria-label="Port" />
				</div>
				<p class="mt-2 text-[12.5px] text-muted-foreground">
					{#if r.upstreamSet}Now <Mono>10.20.1.102</Mono> (home-docker, internal). If the set ever resolves
						to nothing, Traefik keeps the last working address.{:else}A typed address becomes a
						one-item IP set. Pick a set instead to follow a Pulse agent, host or DNS name.{/if}
				</p>
			{:else}
				<Table.Root>
					<Table.Header
						><Table.Row
							><Table.Head>Path</Table.Head><Table.Head>Targets</Table.Head><Table.Head
								>Rewrite</Table.Head
							></Table.Row
						></Table.Header
					>
					<Table.Body>
						<Table.Row
							><Table.Cell
								><Mono>/api</Mono> <span class="text-muted-foreground">prefix</span></Table.Cell
							><Table.Cell><span class="font-medium">media-upstream</span> ×2</Table.Cell
							><Table.Cell>strip prefix</Table.Cell></Table.Row
						>
						<Table.Row
							><Table.Cell
								><Mono>/</Mono> <span class="text-muted-foreground">prefix</span></Table.Cell
							><Table.Cell><span class="font-medium">media-upstream</span></Table.Cell><Table.Cell
								class="text-muted-foreground">—</Table.Cell
							></Table.Row
						>
					</Table.Body>
				</Table.Root>
				<Button variant="ghost" size="sm" class="mt-2"><Plus />Add route</Button>
			{/if}
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="access" class="space-y-4">
		<Panel
			flush
			title="This service’s rules"
			description="Checked first. Then the global rules, then the fallback."
		>
			{#snippet actions()}
				<Button size="sm" variant="outline"><Layers />Attach rule set</Button>
				<Button size="sm" onclick={() => (ruleOpen = true)}><Plus />Add rule</Button>
			{/snippet}
			{#if root.entries.length === 0}
				<p class="border-b px-4 py-3 text-[13px] text-muted-foreground">
					No rules of its own. Everyone not stopped by the abuse engine or the global rules gets the
					fallback.
				</p>
			{/if}
			<RuleEntryList entries={root.entries} bind:fallback onedit={() => (ruleOpen = true)} />
		</Panel>

		<Panel
			flush
			title="What every request goes through"
			description="The abuse engine first, then all rules flattened in order. Try an address and a path."
		>
			<div class="flex flex-wrap gap-2 border-b px-4 py-2.5">
				<Input bind:value={tryIp} class="w-40 font-mono" aria-label="Address to try" />
				<Input bind:value={tryPath} class="w-32 font-mono" aria-label="Path to try" />
			</div>
			<FlattenedRuleList {rows} {decides} />
		</Panel>

		<p class="flex items-start gap-2 text-[12.5px] text-muted-foreground">
			<Unplug class="mt-0.5 size-3.5 shrink-0" />
			{#if failClosed}If Hashi is unreachable this service refuses requests, because its rules can
				deny or ask for sign-in.{:else}If Hashi is unreachable this service stays open: CAPTCHA and
				the abuse engine pause, firewall bans keep working.{/if}
		</p>
	</Tabs.Content>

	<Tabs.Content value="waf" class="grid gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
		<Panel
			flush
			title="Mode per path"
			description="The most specific path wins. Saving creates a plan for every Traefik host."
		>
			{#snippet actions()}<Button size="sm" variant="outline"><Plus />Add path</Button>{/snippet}
			<ul class="divide-y text-[13px]">
				{#each wafPaths as w (w.path)}
					<li class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5">
						<span
							><Mono>{w.path}</Mono>
							<span class="text-muted-foreground"
								>{w.from === 'service' ? 'whole service' : w.type}</span
							></span
						>
						<Select.Root type="single" value={w.mode}>
							<Select.Trigger class="w-40" aria-label="WAF mode for {w.path}"
								>{w.mode.replace('_', ' ')}</Select.Trigger
							>
							<Select.Content>
								<Select.Item value="enabled">enabled</Select.Item>
								<Select.Item value="headers_only">headers only</Select.Item>
								<Select.Item value="disabled">disabled</Select.Item>
							</Select.Content>
						</Select.Root>
					</li>
				{/each}
			</ul>
			<p class="border-t px-4 py-3 text-[12.5px] text-muted-foreground">
				Headers only skips request bodies: use it for uploads, git pushes, WebDAV and large JSON
				APIs.
			</p>
		</Panel>
		<Panel flush title="Exclusions" description="Rules switched off where they misfire">
			{#snippet actions()}<Button size="sm" variant="ghost"><Plus />Add</Button>{/snippet}
			<ul class="divide-y text-[13px]">
				<li class="flex items-center gap-3 px-4 py-2.5">
					<Mono>920350</Mono><span class="flex-1 text-muted-foreground"
						>on <Mono>/api/upload</Mono></span
					>
				</li>
				<li class="flex items-center gap-3 px-4 py-2.5">
					<Mono>942100</Mono><span class="flex-1 text-muted-foreground"
						>on <Mono>/search</Mono></span
					>
				</li>
			</ul>
			<p class="border-t px-4 py-3 text-[12.5px] text-muted-foreground">
				Matches count towards a challenge (5 in 60 s). Security → Counters resets an address that
				was caught by a false positive.
			</p>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="monitoring">
		<Panel title="Checks" description="Public URL and internal upstream are checked separately">
			<dl class="grid gap-3 text-[13px] sm:grid-cols-3">
				<div>
					<dt class="text-[12px] text-muted-foreground">Public</dt>
					<dd>HTTPS {r.domain} · 200 · {r.latency} ms</dd>
				</div>
				<div>
					<dt class="text-[12px] text-muted-foreground">Internal</dt>
					<dd>HTTP <Mono>{r.target}</Mono> · 200</dd>
				</div>
				<div>
					<dt class="text-[12px] text-muted-foreground">Certificate</dt>
					<dd>expires in 41 days</dd>
				</div>
			</dl>
			<p class="mt-3 text-[12.5px] text-muted-foreground">
				Full history and latency charts are on the <a
					class="underline"
					href="/design/status/m-{r.id}">status page for {r.name}</a
				>.
			</p>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="config">
		<Panel
			title={stream ? '20-hashi-stream-services.yml' : '10-hashi-http-services.yml'}
			description="Excerpt Hashi renders for this service on every Traefik host. Read-only."
			flush
		>
			<pre
				class="overflow-x-auto rounded-b-2xl bg-surface-2/40 p-4 font-mono text-[12.5px] leading-relaxed"><code
					>http:
  routers:
    {r.id}-{r.name.toLowerCase().replace(/\W+/g, '-')}:
      rule: Host(`{r.domain}`)
      entryPoints: [websecure]
      service: {r.id}
      middlewares: [hashi-headers, hashi-forward-auth, hashi-waf-{r.waf.replace('_', '-')}]
      tls:
        certResolver: hashi-gts
  services:
    {r.id}:
      loadBalancer:
        servers:
          - url: {r.target}</code
				></pre>
		</Panel>
	</Tabs.Content>
</Tabs.Root>

<Sheet.Root bind:open={ruleOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Rule for {r.name}</Sheet.Title>
			<Sheet.Description>Takes effect within seconds. Nothing changes on Traefik.</Sheet.Description
			>
		</Sheet.Header>
		<div class="overflow-y-auto px-4 pb-4"><RuleItemEditor /></div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (ruleOpen = false)}>Cancel</Button>
			<Button>Save rule</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
