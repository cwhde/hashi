<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import { Button } from '$lib/components/ui/button';
	import SetBadge from '$lib/components/hashi/security/SetBadge.svelte';
	import { firewallHosts } from '$lib/design/mock';
	import { firewallPolicy as fp } from '$lib/design/security';
	import { Plus, ArrowUpRight, TriangleAlert, Pencil } from '@lucide/svelte';
</script>

<PageHeader
	title="Firewall hosts"
	description="Machines with a public IP that forward traffic to Traefik. Hashi owns one nftables table on each; NetBird, Proxmox and Docker rules stay untouched."
>
	{#snippet actions()}<Button><Plus />Add firewall host</Button>{/snippet}
</PageHeader>

<Panel
	title="Global policy"
	description="Applied on every managed host. Where allow and block overlap, the longer prefix wins; on a tie, block."
	class="mb-4"
>
	{#snippet actions()}<Button size="sm" variant="outline"><Pencil />Edit sets</Button>{/snippet}
	<div class="grid gap-4 md:grid-cols-2">
		<div>
			<div class="mb-2 flex items-baseline justify-between text-[13px]">
				<span class="font-medium">Always allow</span><span class="tnum text-muted-foreground"
					>{fp.folded.allow.v4} v4 · {fp.folded.allow.v6} v6 ranges</span
				>
			</div>
			<ul class="space-y-1.5 text-[13px]">
				{#each fp.allow as s (s)}<li><SetBadge slug={s} /></li>{/each}
			</ul>
			<p class="mt-2 text-[12.5px] text-muted-foreground">
				Never dropped and never banned by the abuse engine. Also skips the kernel meters.
			</p>
		</div>
		<div>
			<div class="mb-2 flex items-baseline justify-between text-[13px]">
				<span class="font-medium">Block</span><span class="tnum text-muted-foreground"
					>{fp.folded.block.v4.toLocaleString()} v4 · {fp.folded.block.v6} v6 ranges</span
				>
			</div>
			<ul class="space-y-1.5 text-[13px]">
				{#each fp.block as s (s)}<li><SetBadge slug={s} /></li>{/each}
			</ul>
			<p class="mt-2 text-[12.5px] text-muted-foreground">
				Dropped before anything else. Bans from the abuse engine are added separately, within
				seconds.
			</p>
		</div>
	</div>
	{#each fp.warnings as w (w)}
		<p
			class="mt-4 flex items-start gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-[12.5px] text-warn-text"
		>
			<TriangleAlert class="mt-0.5 size-3.5 shrink-0" />{w}
		</p>
	{/each}
</Panel>

<div class="grid gap-4 lg:grid-cols-2">
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
				<dt class="text-muted-foreground">Host link</dt>
				<dd>
					{#if h.mode === 'managed'}connected · last drain 2 s ago · 0 waiting{:else}<span
							class="text-muted-foreground">not used (external NAT)</span
						>{/if}
				</dd>
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
</div>
