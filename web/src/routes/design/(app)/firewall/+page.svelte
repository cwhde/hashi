<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Tabs from '$lib/components/ui/tabs';
	import { firewallHosts } from '$lib/design/mock';
	import { Plus, ArrowUpRight } from '@lucide/svelte';
</script>

<PageHeader
	title="Firewall hosts"
	description="Machines with a public IP that forward traffic to Traefik. Hashi writes only its own chains and sets; NetBird and other rules stay untouched."
>
	{#snippet actions()}<Button><Plus />Add firewall host</Button>{/snippet}
</PageHeader>

<Tabs.Root value="hosts">
	<Tabs.List class="mb-4"
		><Tabs.Trigger value="hosts">Hosts</Tabs.Trigger><Tabs.Trigger value="allow"
			>Allowlist</Tabs.Trigger
		></Tabs.List
	>
	<Tabs.Content value="hosts" class="grid gap-4 lg:grid-cols-2">
		{#each firewallHosts as h (h.id)}
			<Panel>
				<div class="flex items-start gap-3">
					<div class="min-w-0 flex-1">
						<div class="flex flex-wrap items-center gap-2 text-[15px] font-semibold">
							{h.name}<StatusBadge state={h.health} /><Tag
								>{h.mode === 'managed' ? 'managed' : 'external NAT'}</Tag
							>
						</div>
						<div class="text-[12.5px] text-muted-foreground">
							<Mono>{h.fqdn}</Mono> · <Mono>{h.publicIp}</Mono>
						</div>
					</div>
					<Button
						variant="ghost"
						size="icon-sm"
						href="/design/firewall/{h.id}"
						aria-label="Open {h.name}"><ArrowUpRight /></Button
					>
				</div>
				<dl class="mt-4 grid grid-cols-[8rem_1fr] gap-y-2 text-[13px]">
					<dt class="text-muted-foreground">Subnets</dt>
					<dd><Mono>{h.subnets.join(', ')}</Mono></dd>
					<dt class="text-muted-foreground">Forwards to</dt>
					<dd>{h.traefik} <Mono class="text-muted-foreground">{h.traefikIp}</Mono></dd>
					<dt class="text-muted-foreground">Public ports</dt>
					<dd class="flex flex-wrap gap-1">
						{#each h.ports as p (p)}<Tag>{p}</Tag>{/each}
					</dd>
					<dt class="text-muted-foreground">NetBird</dt>
					<dd>{h.netbird.mode}{h.netbird.mode !== 'disabled' ? ' · ' + h.netbird.status : ''}</dd>
					<dt class="text-muted-foreground">Last apply</dt>
					<dd>{h.lastApply}</dd>
				</dl>
				{#if h.mode === 'external_nat'}
					<p class="mt-3 rounded-xl bg-surface-2/60 p-3 text-[12.5px]">
						Hashi does not touch this host's firewall. Forward these ports on your router to <Mono
							>{h.traefikIp}</Mono
						>: 80/tcp, 443/tcp.
					</p>
				{/if}
				<HistoryStrip samples={h.history} class="mt-4" />
			</Panel>
		{/each}
	</Tabs.Content>
	<Tabs.Content value="allow">
		<Panel
			title="Always allowed"
			description="Never blocked by Hashi on any host. Firewall host IPs are added automatically."
			flush
		>
			{#snippet actions()}<Button size="sm" variant="outline"><Plus />Add entry</Button>{/snippet}
			<ul class="divide-y text-[13px]">
				<li class="flex items-center gap-3 px-4 py-2.5">
					<Mono class="w-44">192.0.2.0/24</Mono><span class="flex-1">Office</span><span
						class="text-muted-foreground">no expiry</span
					>
				</li>
				<li class="flex items-center gap-3 px-4 py-2.5">
					<Mono class="w-44">198.51.100.200</Mono><span class="flex-1">Uptime checker</span><span
						class="text-muted-foreground">expires 2026-12-31</span
					>
				</li>
				<li class="flex items-center gap-3 px-4 py-2.5">
					<Mono class="w-44">198.51.100.24</Mono><span class="flex-1">edge-fra (automatic)</span
					><Tag>system</Tag>
				</li>
			</ul>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
