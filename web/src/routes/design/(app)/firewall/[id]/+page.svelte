<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import SetBadge from '$lib/components/hashi/security/SetBadge.svelte';
	import AddressLink from '$lib/components/hashi/security/AddressLink.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Select from '$lib/components/ui/select';
	import { Input } from '$lib/components/ui/input';
	import { firewallHosts } from '$lib/design/mock';
	import { ChevronLeft, LifeBuoy, Cable, Plus, X } from '@lucide/svelte';

	let { data } = $props();
	const h = $derived(data.h);
	let entry = $derived(h.id);
	const entryName = $derived(firewallHosts.find((x) => x.id === entry)?.name ?? h.name);
	// Per-service overrides that start on this host (03-dns.md §5.1); one shared record per entry host.
	const ovr = $derived(
		h.id === 'fh2' ? [{ entry: 'edge-fra', services: ['Media', 'Photos'] }] : []
	);
	const kernelSets = [
		{ name: 'allow4 / allow6', what: 'Folded allow (interval)', size: '7 / 3', owner: 'plan' },
		{ name: 'block4 / block6', what: 'Folded block (interval)', size: '1,503 / 0', owner: 'plan' },
		{ name: 'dyn4 / dyn6', what: 'Bans with timeouts', size: '4 / 0', owner: 'host link' },
		{
			name: 'staging4 / staging6',
			what: 'Caught by a meter, waiting for Hashi',
			size: '0 / 0',
			owner: 'kernel'
		},
		{
			name: 'host_access4 / 6',
			what: 'Who may reach SSH and Proxmox here',
			size: '3 / 2',
			owner: 'plan'
		},
		{ name: 'pub_tcp / pub_udp', what: 'Published ports', size: '4 / 2', owner: 'plan' }
	];
	const meters = [
		['New TCP connections', '150/min per address, burst 30'],
		['Open TCP connections', '250 per address'],
		['New UDP flows', '200/min per address, burst 50 (stream ports only)'],
		['UDP packets', '5000/s per address, burst 1000 (stream ports only)']
	];
</script>

<a
	href="/design/firewall"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Firewall hosts</a
>
<div class="mb-5 flex flex-wrap items-end gap-4">
	<div class="flex-1">
		<h1 class="flex items-center gap-2 text-[22px] font-semibold">
			{h.name}<StatusBadge state={h.health} />
		</h1>
		<p class="text-[13.5px] text-muted-foreground">
			{h.os} · nft 1.1.3 · <Mono>{h.publicIp}</Mono>{#if h.publicIp6}
				· <Mono>{h.publicIp6}</Mono>{/if}
		</p>
	</div>
	<Button variant="outline" size="sm">Validate</Button>
	<Button size="sm">Plan firewall</Button>
</div>

<div class="mb-4 grid gap-4 lg:grid-cols-3">
	<Panel title="Rescue path">
		<div class="flex items-start gap-3 text-[13px]">
			<LifeBuoy class="mt-0.5 size-5 shrink-0 text-ok-text" />
			<p>
				Every high-risk apply arms a 180 s timer that restores Hashi’s table unless a <em>new</em>
				SSH login and NetBird check succeed. Last time: {h.rescue}.
			</p>
		</div>
	</Panel>
	<Panel title="NetBird">
		<dl class="grid grid-cols-[6rem_1fr] gap-y-1.5 text-[13px]">
			<dt class="text-muted-foreground">Mode</dt>
			<dd>{h.netbird.mode}</dd>
			<dt class="text-muted-foreground">Interface</dt>
			<dd><Mono>{h.netbird.iface}</Mono> · never filtered by Hashi</dd>
			<dt class="text-muted-foreground">Overlay</dt>
			<dd><Mono>{h.netbird.overlay}</Mono></dd>
			<dt class="text-muted-foreground">Status</dt>
			<dd>{h.netbird.status}</dd>
		</dl>
	</Panel>
	<Panel title="Host link" description="One SSH connection Hashi keeps open">
		{#if h.mode === 'managed'}<div class="flex items-start gap-3 text-[13px]">
				<Cable class="mt-0.5 size-5 shrink-0 text-ok-text" />
				<dl class="grid flex-1 grid-cols-[7rem_1fr] gap-y-1.5">
					<dt class="text-muted-foreground">State</dt>
					<dd>connected for 3 d</dd>
					<dt class="text-muted-foreground">Last drain</dt>
					<dd>2 s ago · 0 waiting</dd>
					<dt class="text-muted-foreground">Last push</dt>
					<dd>13:37 · 1 ban, applied</dd>
					<dt class="text-muted-foreground">Checked</dt>
					<dd>4 min ago · no difference</dd>
				</dl>
			</div>
		{:else}<p class="text-[13px] text-muted-foreground">
				Not used: Hashi does not manage this host’s firewall (external NAT).
			</p>{/if}
	</Panel>
</div>

<Tabs.Root value="routing">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="routing">Routing</Tabs.Trigger>
		<Tabs.Trigger value="ports">Ports</Tabs.Trigger>
		<Tabs.Trigger value="access">Host access</Tabs.Trigger>
		<Tabs.Trigger value="sets">Kernel sets</Tabs.Trigger>
		<Tabs.Trigger value="rules">Live ruleset</Tabs.Trigger>
		<Tabs.Trigger value="generated">Generated</Tabs.Trigger>
	</Tabs.List>
	<Tabs.Content value="routing" class="grid gap-4 lg:grid-cols-2">
		<Panel
			title="Managed subnets"
			description="A service whose target is in one of these runs on {h.name}"
		>
			<ul class="space-y-1.5 text-[13px]">
				{#each h.subnets as n (n)}
					<li class="flex items-center gap-2">
						<Mono class="flex-1">{n}</Mono><Button
							variant="ghost"
							size="icon-xs"
							aria-label="Remove {n}"><X /></Button
						>
					</li>
				{/each}
			</ul>
			<div class="mt-3 flex gap-2">
				<Input class="font-mono" placeholder="10.20.2.0/24" aria-label="Add subnet" /><Button
					variant="outline"
					size="sm">Add</Button
				>
			</div>
			<p class="mt-2 text-[12.5px] text-muted-foreground">
				NetBird-routed ranges and Pulse agents whose addresses fall inside count too.
			</p>
		</Panel>
		<Panel
			title="Public entry"
			description="Where traffic for services on {h.name} enters. Changes one DNS record."
		>
			<Select.Root type="single" bind:value={entry}>
				<Select.Trigger class="w-full"
					>{entry === h.id ? `${h.name} itself` : `through ${entryName}`}</Select.Trigger
				>
				<Select.Content>
					{#each firewallHosts as x (x.id)}<Select.Item value={x.id}
							>{x.id === h.id ? `${x.name} itself` : `through ${x.name}`}</Select.Item
						>{/each}
				</Select.Content>
			</Select.Root>
			<div class="mt-3 space-y-1 text-[12.5px]">
				<div><Mono>on.{h.name}.example.com</Mono> → <Mono>via.{entryName}.example.com</Mono></div>
				<div><Mono>via.{h.name}.example.com</Mono> → <Mono>{h.name}.example.com</Mono></div>
				<div><Mono>{h.name}.example.com</Mono> A <Mono>{h.publicIp}</Mono></div>
			</div>
			{#if ovr.length}
				<div class="mt-4 mb-1.5 text-[12.5px] font-medium">Services entering elsewhere</div>
				<ul class="space-y-1.5 text-[12.5px]">
					{#each ovr as o (o.entry)}
						<li>
							<Mono>{o.entry}.ovr.on.{h.name}.example.com</Mono> → <Mono
								>via.{o.entry}.example.com</Mono
							>
							<span class="block text-muted-foreground"
								>used by {o.services.join(', ')} · removed when no service uses it</span
							>
						</li>
					{/each}
				</ul>
			{/if}
		</Panel>
	</Tabs.Content>
	<Tabs.Content value="ports">
		<Panel
			flush
			title="Public ports"
			description="From services. Opening or closing one is a high-risk change."
		>
			<ul class="divide-y text-[13px]">
				{#each h.ports as p (p)}
					<li class="flex flex-wrap items-center gap-3 px-4 py-2.5">
						<Mono class="w-24">{p}</Mono><span class="min-w-0 flex-1"
							>→ <Mono>{h.traefikIp}</Mono> entry point {p.startsWith('80')
								? 'web'
								: p.startsWith('443')
									? 'websecure'
									: p.replace('/', '-').replace(/(\d+)-(\w+)/, '$2-$1')}</span
						>{#if p === '19132/udp' || p === '222/tcp'}<span
								class="text-[12.5px] text-muted-foreground">kernel meters on</span
							>{/if}<Tag tone="ok">open</Tag>
					</li>
				{/each}
				<li class="flex items-center gap-3 px-4 py-2.5">
					<Mono class="w-24">2022/tcp</Mono><span class="flex-1 text-muted-foreground"
						>Listening on the host (sshd-alt), not managed by Hashi</span
					><Tag tone="warn">conflict check</Tag>
				</li>
			</ul>
		</Panel>
	</Tabs.Content>
	<Tabs.Content value="access">
		<Panel
			title="Who may reach this host itself"
			description="SSH and the Proxmox UI on the host. Loopback, its own subnets and NetBird always can."
		>
			{#snippet actions()}<Button size="sm" variant="outline"><Plus />Add set</Button>{/snippet}
			<ul class="space-y-2 text-[13px]">
				<li><SetBadge slug="hashi-hosts" /></li>
				<li><SetBadge slug="home-wan" /></li>
			</ul>
			<p class="mt-3 text-[12.5px] text-muted-foreground">
				Removing a set that contains your current address (<Mono>203.0.113.88</Mono>) needs a
				passkey and arms the rescue timer.
			</p>
		</Panel>
	</Tabs.Content>
	<Tabs.Content value="sets" class="grid gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
		<Panel flush title="Sets in table inet hashi" description="Read from the host 4 min ago">
			<ul class="divide-y text-[13px]">
				{#each kernelSets as k (k.name)}
					<li class="grid grid-cols-[10rem_minmax(0,1fr)_6rem_6rem] items-center gap-3 px-4 py-2.5">
						<Mono>{k.name}</Mono><span class="truncate text-muted-foreground">{k.what}</span><span
							class="tnum text-right">{k.size}</span
						><span class="text-right text-[12.5px] text-muted-foreground">{k.owner}</span>
					</li>
				{/each}
			</ul>
		</Panel>
		<div class="space-y-4">
			<Panel title="Kernel meters" description="Over the limit → staging, then a ban from Hashi">
				<dl class="grid grid-cols-[minmax(0,1fr)_auto] gap-y-2 text-[13px]">
					{#each meters as [k, v] (k)}<dt class="text-muted-foreground">{k}</dt>
						<dd class="text-right">{v}</dd>{/each}
				</dl>
				<p class="mt-2 text-[12.5px] text-muted-foreground">
					Only on the WAN interface and published ports. Allowed addresses are never metered.
				</p>
			</Panel>
			<Panel title="Caught in the last hour">
				<ul class="space-y-1.5 text-[13px]">
					<li class="flex justify-between">
						<AddressLink address="198.51.100.77" /><span class="text-muted-foreground"
							>open TCP connections · 13:02</span
						>
					</li>
				</ul>
			</Panel>
		</div>
	</Tabs.Content>
	<Tabs.Content value="rules">
		<Panel
			flush
			title="Observed ruleset"
			description="Every table on the host, read just now. Hashi’s table is highlighted; the rest is left alone."
		>
			<pre class="overflow-x-auto p-4 font-mono text-[12.5px] leading-relaxed"><code
					><span class="text-primary">table inet hashi &#123;</span>
<span
						class="text-primary">  chain pre_guard &#123; type filter hook prerouting priority raw; …</span
					>
<span class="text-primary">    iifname "vmbr0" ip saddr @allow4 return</span>
<span
						class="text-primary">    iifname "vmbr0" ip saddr &#123; @block4, @dyn4, @staging4 &#125; drop</span
					>
<span
						class="text-primary">  chain pre_meter &#123; type filter hook prerouting priority -150; …</span
					>
<span class="text-primary">  &#125;</span>
<span class="text-muted-foreground"># iptables-legacy (NetBird, untouched)</span>
<span class="text-muted-foreground">-A POSTROUTING -o wt0 -j NETBIRD-RT-NAT</span></code
				></pre>
		</Panel>
	</Tabs.Content>
	<Tabs.Content value="generated">
		<Panel
			flush
			title="hashi.nft.json"
			description="What the next apply loads with nft -j -f, as one transaction. Read-only."
		>
			<pre
				class="overflow-x-auto p-4 font-mono text-[12.5px] leading-relaxed text-muted-foreground"><code
					>{`{ "nftables": [
  { "add": { "table": { "family": "inet", "name": "hashi" } } },
  { "add": { "set": { "family": "inet", "table": "hashi", "name": "allow4",
      "type": "ipv4_addr", "flags": ["interval"], "auto-merge": true } } },
  …
] }`}</code
				></pre>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
