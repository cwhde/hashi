<script lang="ts">
	import { page } from '$app/state';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import SetBadge from '$lib/components/hashi/security/SetBadge.svelte';
	import FlattenedRuleList from '$lib/components/hashi/security/FlattenedRuleList.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import { services, securityEvents } from '$lib/design/mock';
	import {
		ipSets,
		counters,
		containsAddress,
		flatten,
		stageA,
		setBySlug,
		inCidr
	} from '$lib/design/security';
	import { ChevronLeft, ShieldAlert, ShieldCheck, Minus } from '@lucide/svelte';

	// Everything Hashi knows and decides about one address (05a §6). The trace is the memorable part: the same
	// list the service's Access tab shows, with the row that decides highlighted.
	const address = $derived(page.url.searchParams.get('ip') ?? '203.0.113.9');
	const c = $derived(
		counters.find((x) => x.address === address) ?? {
			address,
			country: 'CH',
			asn: 'AS64505 Example Mobile CH',
			ban: 0,
			challenge: 0,
			state: 'none' as const,
			stateDetail: '—',
			lastBan: '—',
			lastChallenge: '—',
			nextDecrement: '—',
			source: 'manual' as const
		}
	);
	const memberOf = $derived(
		ipSets.filter((s) => s.kind !== 'dynamic' && s.slug !== 'all' && containsAddress(s, address))
	);
	const pathTo = (slug: string): string => {
		const s = setBySlug(slug)!;
		const nested = s.items.find(
			(i) => i.type === 'set' && setBySlug(i.value) && containsAddress(setBySlug(i.value)!, address)
		);
		if (nested) return `${slug} › ${pathTo(nested.value)}`;
		const leaf = s.items.find(
			(i) =>
				i.value === address ||
				i.resolved?.includes(address) ||
				(i.type === 'cidr' && inCidr(address, i.value))
		);
		return leaf
			? `${slug} › ${leaf.type === 'pulse' || leaf.type === 'host' ? `${leaf.value} (${leaf.type}, ${leaf.scope})` : leaf.value}`
			: slug;
	};
	const allowed = $derived(
		memberOf.some((s) => ['trusted-devices', 'hashi-hosts'].includes(s.slug))
	);
	const blockedBy = $derived(
		memberOf.find((s) => ['feed-spamhaus-drop', 'feed-feodo'].includes(s.slug))
	);

	const prefixIn = (slug: string) => {
		const leaf = pathTo(slug).split(' › ').at(-1) ?? '';
		return leaf.includes('/') ? leaf : address + (address.includes(':') ? '/128' : '/32');
	};
	const allowedBy = $derived(
		memberOf.find((s) => ['trusted-devices', 'hashi-hosts'].includes(s.slug))
	);

	const http = services.filter((s) => !['tcp', 'udp'].includes(s.kind));
	let serviceId = $state('r2');
	let path = $state('/admin');
	const rows = $derived([...stageA, ...flatten(serviceId)]);
	const decides = $derived.by(() => {
		if (!allowed && (c.state === 'banned' || blockedBy)) return 0;
		if (c.state === 'challenged') return 2;
		return rows.findIndex(
			(r, i) =>
				i >= stageA.length &&
				containsAddress(setBySlug(r.appliesTo ?? 'all')!, address) &&
				(!r.condition || path.startsWith(r.condition.pattern))
		);
	});
	let ban = $state(0);
	let challenge = $state(0);
	$effect(() => {
		ban = c.ban;
		challenge = c.challenge;
	});
</script>

<a
	href="/design/ip-sets"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />IP sets</a
>

<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
	<div class="min-w-64 flex-1">
		<h1 class="font-mono text-[24px] leading-tight font-medium tracking-[-0.02em]">{address}</h1>
		<p class="mt-1 flex flex-wrap items-center gap-2 text-[13.5px] text-muted-foreground">
			<span>{c.country} · {c.asn}</span>
			{#if c.state === 'banned'}<Tag tone="bad">banned {c.stateDetail}</Tag
				>{:else if c.state === 'challenged'}<Tag tone="warn">challenged · {c.stateDetail}</Tag
				>{:else if allowed}<Tag tone="ok">always allowed</Tag>{/if}
		</p>
	</div>
	<div class="flex flex-wrap gap-2">
		<Button variant="outline" size="sm">Add to an IP set</Button>
		{#if c.state !== 'none'}<Button variant="outline" size="sm"
				>{c.state === 'banned' ? 'Unban' : 'End challenge'}</Button
			>{/if}
		{#if c.state !== 'challenged'}<Button variant="outline" size="sm">Challenge now</Button>{/if}
		{#if c.state !== 'banned'}<Button size="sm" disabled={allowed}>Ban now</Button>{/if}
	</div>
</div>

<div class="mb-4 grid gap-4 lg:grid-cols-3">
	<Panel title="Member of" description="Sets that contain this address, and through what">
		{#if memberOf.length}
			<ul class="space-y-2.5 text-[13px]">
				{#each memberOf as s (s.id)}
					<li>
						<SetBadge slug={s.slug} />
						<span class="block truncate text-[12.5px] text-muted-foreground">{pathTo(s.slug)}</span>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="text-[13px] text-muted-foreground">
				Only in <span class="font-medium text-foreground">all</span>. No set of yours or feed
				contains it.
			</p>
		{/if}
	</Panel>

	<Panel title="Firewall" description="Allow and block folded together, per host">
		<ul class="space-y-3 text-[13px]">
			{#each ['edge-fra', 'edge-home'] as h (h)}
				<li class="flex items-start gap-2.5">
					{#if h === 'edge-home'}<Minus class="mt-0.5 size-4 text-muted-foreground" />
					{:else if allowed}<ShieldCheck class="mt-0.5 size-4 text-ok-text" />
					{:else if blockedBy || c.state === 'banned'}<ShieldAlert
							class="mt-0.5 size-4 text-bad-text"
						/>
					{:else}<Minus class="mt-0.5 size-4 text-muted-foreground" />{/if}
					<span class="min-w-0">
						<span class="font-medium">{h}</span>
						<span class="block text-[12.5px] text-muted-foreground">
							{#if h === 'edge-home'}External NAT: Hashi does not filter here
							{:else if allowed}Allowed by <Mono>{prefixIn(allowedBy!.slug)}</Mono> ({allowedBy!
									.slug}). Never banned.
							{:else if blockedBy}Dropped by <Mono>{prefixIn(blockedBy.slug)}</Mono> ({blockedBy.slug})
							{:else if c.state === 'banned'}Dropped by the ban in <Mono>dyn</Mono> · pushed 13:37
							{:else}Neither allowed nor blocked: passes to Traefik{/if}
						</span>
					</span>
				</li>
			{/each}
		</ul>
	</Panel>

	<Panel title="Counters" description="Decide how long the next ban lasts">
		<div class="grid grid-cols-2 gap-3">
			<div class="space-y-1.5">
				<Label for="cb">Ban counter</Label><Input
					id="cb"
					type="number"
					min="0"
					bind:value={ban}
					class="tnum"
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="cc">Challenge counter</Label><Input
					id="cc"
					type="number"
					min="0"
					bind:value={challenge}
					class="tnum"
				/>
			</div>
		</div>
		<p class="mt-2 text-[12.5px] text-muted-foreground">
			The next ban would last {[
				'15 min',
				'30 min',
				'1 h',
				'2 h',
				'4 h',
				'8 h',
				'16 h',
				'1 d 8 h',
				'2 d 16 h'
			][Math.min(ban, 8)]}. Each counter drops by one per 7 quiet days{c.nextDecrement !== '—'
				? ` (${c.nextDecrement})`
				: ''}.
		</p>
		<div class="mt-3 flex gap-2">
			<Button size="sm" disabled={ban === c.ban && challenge === c.challenge}>Save counters</Button>
			<Button size="sm" variant="ghost" onclick={() => ((ban = 0), (challenge = 0))}
				>Reset both</Button
			>
		</div>
	</Panel>
</div>

<Panel
	flush
	title="Try a request"
	description="What Hashi would do with a request from this address right now"
	class="mb-4"
>
	<div class="flex flex-wrap gap-2 border-b px-4 py-2.5">
		<Select.Root type="single" bind:value={serviceId}>
			<Select.Trigger class="w-44" aria-label="Service"
				>{http.find((s) => s.id === serviceId)?.name}</Select.Trigger
			>
			<Select.Content>
				{#each http as s (s.id)}<Select.Item value={s.id}>{s.name}</Select.Item>{/each}
			</Select.Content>
		</Select.Root>
		<Input bind:value={path} class="w-36 font-mono" aria-label="Path" />
	</div>
	<FlattenedRuleList {rows} {decides} />
</Panel>

<Panel flush title="Timeline" description="Events for this address, newest first">
	<ul class="divide-y text-[13px]">
		{#each securityEvents.filter((e) => e.subject === address) as e (e.t)}
			<li class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3 px-4 py-2.5">
				<span class="tnum text-muted-foreground">{e.t}</span><span
					>{e.type} · <span class="font-medium">{e.resource}</span></span
				>
			</li>
		{/each}
		{#if address === '203.0.113.9'}
			<li class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3 px-4 py-2.5">
				<span class="tnum text-muted-foreground">9 d ago</span><span
					>Banned 15 min (1st ban) · 15 requests without solving</span
				>
			</li>
		{/if}
		{#if !securityEvents.some((e) => e.subject === address)}
			<li class="px-4 py-6 text-muted-foreground">
				No events for this address in the last 30 days.
			</li>
		{/if}
	</ul>
</Panel>
