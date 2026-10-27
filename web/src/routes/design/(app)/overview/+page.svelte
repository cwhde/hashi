<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import StatusDot from '$lib/components/hashi/StatusDot.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import Sparkline from '$lib/components/hashi/Sparkline.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import {
		resources,
		firewallHosts,
		traefikHosts,
		plans,
		securityEvents,
		audit,
		spark,
		zone
	} from '$lib/design/mock';
	import { Settings2, ArrowUpRight } from '@lucide/svelte';

	const counts = {
		up: resources.filter((r) => r.health === 'up').length,
		degraded: resources.filter((r) => r.health === 'degraded').length,
		down: resources.filter((r) => r.health === 'down').length
	};
	const attention = resources.filter(
		(r) => r.health === 'down' || r.health === 'degraded' || r.sync === 'invalid'
	);
</script>

<PageHeader
	title="Overview"
	description="What needs you now, then everything else. Widgets can be hidden and reordered in Settings → Appearance."
>
	{#snippet actions()}
		<Button variant="outline" size="sm" href="/design/settings"><Settings2 />Arrange widgets</Button
		>
	{/snippet}
</PageHeader>

<!-- Pending high-risk plans are surfaced first after login (10-ui.md §3). -->
<div class="mb-5 grid gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
	<PlanReview {...plans[0]!} compact />
	<Panel title="Needs attention" description="{attention.length} resources">
		<ul class="divide-y">
			{#each attention as r (r.id)}
				<li class="flex items-center gap-3 py-2.5">
					<StatusDot state={r.health === 'paused' ? 'unknown' : r.health} />
					<div class="min-w-0 flex-1">
						<div class="truncate text-[13.5px] font-medium">{r.name}</div>
						<div class="truncate text-[12.5px] text-muted-foreground">
							{#if r.sync === 'invalid'}Not rendered: port 19132/udp has no firewall opening yet{:else if r.health === 'down'}Down
								for 7 min · 3 failed checks{:else}Slow: p95 412 ms over 300 ms threshold{/if}
						</div>
					</div>
					<Button
						variant="ghost"
						size="icon-sm"
						href="/design/resources/{r.id}"
						aria-label="Open {r.name}"><ArrowUpRight /></Button
					>
				</li>
			{/each}
		</ul>
	</Panel>
</div>

<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
	<Panel title="Resource health" description="{resources.length} resources · last 60 min">
		<div class="mb-3 flex gap-5 text-[13px]">
			<span><span class="tnum text-[20px] font-semibold text-ok-text">{counts.up}</span> up</span>
			<span
				><span class="tnum text-[20px] font-semibold text-warn-text">{counts.degraded}</span> degraded</span
			>
			<span
				><span class="tnum text-[20px] font-semibold text-bad-text">{counts.down}</span> down</span
			>
		</div>
		<div class="space-y-2">
			{#each resources.slice(1, 6) as r (r.id)}
				<div class="grid grid-cols-[7rem_1fr] items-center gap-3">
					<span class="truncate text-[12.5px]">{r.name}</span>
					<HistoryStrip samples={r.history} labels={false} />
				</div>
			{/each}
		</div>
	</Panel>

	<Panel title="Edge hosts" description="Firewall and Traefik">
		<ul class="space-y-3">
			{#each firewallHosts as h (h.id)}
				<li class="flex items-start gap-3">
					<StatusDot state={h.health} class="mt-1.5" />
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2 text-[13.5px] font-medium">
							{h.name}<Tag>{h.mode === 'managed' ? 'managed' : 'external NAT'}</Tag>
						</div>
						<div class="text-[12.5px] text-muted-foreground">
							<Mono>{h.publicIp}</Mono> · applied {h.lastApply}
						</div>
					</div>
				</li>
			{/each}
			{#each traefikHosts as t (t.id)}
				<li class="flex items-start gap-3">
					<StatusDot state={t.health} class="mt-1.5" />
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2 text-[13.5px] font-medium">
							{t.name}<Tag tone="info">update {t.update}</Tag>
						</div>
						<div class="text-[12.5px] text-muted-foreground">
							{t.routers} routers, 0 errors · verified {t.lastVerify}
						</div>
					</div>
				</li>
			{/each}
		</ul>
	</Panel>

	<Panel title="Sync state" description="Desired vs observed">
		<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2.5 text-[13px]">
			<dt class="text-muted-foreground">DNS</dt>
			<dd>
				<StatusBadge state="degraded" label="1 change waiting" />
				<span class="text-muted-foreground">· {zone.name}</span>
			</dd>
			<dt class="text-muted-foreground">Traefik</dt>
			<dd><StatusBadge state="degraded" label="1 file differs" /></dd>
			<dt class="text-muted-foreground">Firewall</dt>
			<dd><StatusBadge state="degraded" label="1 high-risk plan" /></dd>
			<dt class="text-muted-foreground">AdGuard</dt>
			<dd><StatusBadge state="up" label="In sync" /></dd>
			<dt class="text-muted-foreground">Last reconcile</dt>
			<dd>13:00, no changes · next 14:00</dd>
		</dl>
	</Panel>

	<Panel title="Security, last 24 h" description="Edge decisions">
		<div class="grid grid-cols-3 gap-3">
			<div>
				<div class="tnum text-[20px] font-semibold">182k</div>
				<div class="text-[12px] text-muted-foreground">allowed</div>
				<Sparkline values={spark(2)} width={90} class="mt-1 w-full text-ok-text" />
			</div>
			<div>
				<div class="tnum text-[20px] font-semibold">1,204</div>
				<div class="text-[12px] text-muted-foreground">challenged</div>
				<Sparkline values={spark(5, 48, 20, 15)} width={90} class="mt-1 w-full text-warn-text" />
			</div>
			<div>
				<div class="tnum text-[20px] font-semibold">9,931</div>
				<div class="text-[12px] text-muted-foreground">blocked</div>
				<Sparkline values={spark(9, 48, 30, 25)} width={90} class="mt-1 w-full text-bad-text" />
			</div>
		</div>
		<ul class="mt-3 divide-y border-t">
			{#each securityEvents.slice(0, 3) as e (e.t)}
				<li class="flex items-center gap-2 py-2 text-[12.5px]">
					<Mono class="w-28 shrink-0 truncate">{e.subject}</Mono><span
						class="min-w-0 flex-1 truncate text-muted-foreground">{e.type}</span
					><span class="tnum text-muted-foreground">{e.t}</span>
				</li>
			{/each}
		</ul>
	</Panel>

	<Panel title="Certificates" description="Earliest expiry per Traefik host">
		<ul class="space-y-2.5 text-[13px]">
			<li class="flex items-center justify-between gap-3">
				<span class="truncate">media.example.com</span><Tag tone="warn">18 days</Tag>
			</li>
			<li class="flex items-center justify-between gap-3">
				<span class="truncate">grafana.example.com</span><Tag>41 days</Tag>
			</li>
			<li class="flex items-center justify-between gap-3">
				<span class="truncate">28 more</span><span class="text-muted-foreground">over 60 days</span>
			</li>
		</ul>
	</Panel>

	<Panel title="Recent activity" description="Audit log">
		<ul class="space-y-2.5">
			{#each audit.slice(0, 4) as a, i (i)}
				<li class="grid grid-cols-[3rem_1fr] gap-2 text-[12.5px]">
					<span class="tnum text-muted-foreground">{a.t}</span><span class="min-w-0"
						><span class="font-medium">{a.what}</span><span
							class="block truncate text-muted-foreground">{a.result}</span
						></span
					>
				</li>
			{/each}
		</ul>
	</Panel>
</div>
