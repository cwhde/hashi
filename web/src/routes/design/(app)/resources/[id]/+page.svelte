<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import * as Table from '$lib/components/ui/table';
	import { ChevronLeft, GripVertical, Plus, Lock } from '@lucide/svelte';

	let { data } = $props();
	const r = $derived(data.r);
	let advanced = $state(false);
	let policy = $state('sso');
	const policies = [
		{
			v: 'sso',
			t: 'SSO required',
			d: 'Everyone signs in. If Hashi is unreachable, requests are refused.'
		},
		{
			v: 'adaptive',
			t: 'Adaptive',
			d: 'Open normally; sign-in or a challenge only when traffic looks abusive.'
		},
		{ v: 'observe', t: 'Observe', d: 'Decisions are recorded but never enforced.' },
		{ v: 'off', t: 'Off', d: 'No forward auth. Only firewall blocks apply.' }
	];

	const rules = [
		{ p: 100, match: 'CIDR 192.0.2.0/24', action: 'Allow (skip sign-in)', note: 'Office' },
		{
			p: 50,
			match: 'Path prefix /api/hooks',
			action: 'Allow (skip sign-in)',
			note: 'Webhooks from CI'
		},
		{
			p: 10,
			match: 'Country not in CH, DE',
			action: 'Require challenge',
			note: 'Needs GeoIP · database 6 d old'
		},
		{ p: 0, match: 'ASN AS64500', action: 'Deny', note: 'Hosting provider abuse' }
	];
</script>

<a
	href="/design/resources"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Resources</a
>

<!-- Summary strip above tabs (10a §5.3 item 8). -->
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
				>https://{r.domain}</a
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
			<dt class="text-[12px] text-muted-foreground">Enters via</dt>
			<dd>{r.host}</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Sign-in</dt>
			<dd>{r.auth}</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">WAF</dt>
			<dd>{r.waf}</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Sync</dt>
			<dd>{r.sync === 'in sync' ? 'In sync on 2 hosts' : r.sync}</dd>
		</div>
	</dl>
	<HistoryStrip samples={r.history} class="mt-4" />
</div>

<Tabs.Root value="routing">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="routing">Routing</Tabs.Trigger>
		<Tabs.Trigger value="access">Access</Tabs.Trigger>
		<Tabs.Trigger value="rules">Rules</Tabs.Trigger>
		<Tabs.Trigger value="monitoring">Monitoring</Tabs.Trigger>
		<Tabs.Trigger value="config">Rendered config</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="routing" class="grid gap-4 lg:grid-cols-2">
		<Panel title="Public address">
			<div class="space-y-4">
				<div class="space-y-1.5"><Label>Domain</Label><Input value={r.domain} /></div>
				<div class="text-[13px]">
					<div class="mb-1 text-[12px] text-muted-foreground">DNS Hashi will keep</div>
					<Mono>{r.domain}. CNAME on.{r.host}.example.com.</Mono>
				</div>
				<div class="flex items-center justify-between">
					<Label for="ov">Route through another host</Label><Switch id="ov" />
				</div>
			</div>
		</Panel>
		<Panel title="Upstream">
			{#snippet actions()}<div class="flex items-center gap-2 text-[13px]">
					<Label for="adv">Advanced routes</Label><Switch id="adv" bind:checked={advanced} />
				</div>{/snippet}
			{#if !advanced}
				<div class="grid grid-cols-[6rem_1fr_6rem] gap-2">
					<Input value={r.kind} aria-label="Scheme" /><Input
						value={r.target.replace(/^\w+:\/\//, '').split(':')[0]}
						aria-label="Host"
					/><Input value={r.target.split(':').at(-1)} aria-label="Port" />
				</div>
				<p class="mt-2 text-[12.5px] text-muted-foreground">
					Load balancing, path routing and rewrites need advanced routes.
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
							><Table.Cell><Mono>10.20.1.102:2283</Mono> ×2</Table.Cell><Table.Cell
								>strip prefix</Table.Cell
							></Table.Row
						>
						<Table.Row
							><Table.Cell
								><Mono>/</Mono> <span class="text-muted-foreground">prefix</span></Table.Cell
							><Table.Cell><Mono>10.20.1.102:2283</Mono></Table.Cell><Table.Cell
								class="text-muted-foreground">—</Table.Cell
							></Table.Row
						>
					</Table.Body>
				</Table.Root>
				<Button variant="ghost" size="sm" class="mt-2"><Plus />Add route</Button>
			{/if}
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="access" class="grid gap-4 lg:grid-cols-2">
		<Panel title="Sign-in policy" description="Forward auth decided by Hashi for every request">
			<RadioGroup.Root bind:value={policy} class="gap-3">
				{#each policies as { v, t, d } (v)}
					<label
						class="flex cursor-pointer gap-3 rounded-xl border p-3 hover:bg-surface-2/50 {policy ===
						v
							? 'border-primary/60'
							: ''}"
					>
						<RadioGroup.Item value={v} class="mt-0.5" />
						<span
							><span class="block text-[13.5px] font-medium">{t}</span><span
								class="block text-[12.5px] text-muted-foreground">{d}</span
							></span
						>
					</label>
				{/each}
			</RadioGroup.Root>
		</Panel>
		<div class="space-y-4">
			<Panel title="Identity provider">
				<div class="flex items-center justify-between text-[13px]">
					<span>Pocket ID <span class="text-muted-foreground">(default)</span></span><Tag tone="bad"
						>unreachable 6 min</Tag
					>
				</div>
				<p class="mt-2 text-[12.5px] text-muted-foreground">
					Allowed groups: <Mono>family</Mono>, <Mono>admins</Mono>
				</p>
			</Panel>
			<Panel title="Web application firewall">
				<div class="flex gap-2">
					{#each ['off', 'detect', 'block'] as m (m)}<Button
							size="sm"
							variant={r.waf === m ? 'secondary' : 'ghost'}>{m}</Button
						>{/each}
				</div>
				<p class="mt-2 text-[12.5px] text-muted-foreground">
					2 exclusions: rule 920350 on <Mono>/api/upload</Mono>, rule 942100 on <Mono>/search</Mono
					>.
				</p>
			</Panel>
		</div>
	</Tabs.Content>

	<Tabs.Content value="rules">
		<Panel title="Rules" description="Checked top to bottom; the first match decides" flush>
			{#snippet actions()}<Button size="sm" variant="outline"><Plus />Add rule</Button>{/snippet}
			<ol class="divide-y">
				{#each rules as rule (rule.p)}
					<li class="flex items-center gap-3 px-4 py-2.5 text-[13px]">
						<GripVertical
							class="size-4 cursor-grab text-muted-foreground"
							aria-label="Drag to reorder"
						/>
						<span class="tnum w-8 text-muted-foreground">{rule.p}</span>
						<span class="min-w-0 flex-1"
							><span class="font-medium">{rule.match}</span><span
								class="block text-[12.5px] text-muted-foreground">{rule.note}</span
							></span
						>
						<Tag
							tone={rule.action.startsWith('Deny')
								? 'bad'
								: rule.action.startsWith('Allow')
									? 'ok'
									: 'warn'}>{rule.action}</Tag
						>
						<Switch checked aria-label="Rule enabled" />
					</li>
				{/each}
			</ol>
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
			title="10-hashi-http-resources.yml"
			description="Excerpt Hashi renders for this resource on every Traefik host. Read-only."
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
      middlewares: [hashi-headers, hashi-auth-{r.id}, hashi-waf-{r.waf}]
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
