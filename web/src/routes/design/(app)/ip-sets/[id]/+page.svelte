<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import IpItemView from '$lib/components/hashi/security/IpItemView.svelte';
	import IpItemEditor from '$lib/components/hashi/security/IpItemEditor.svelte';
	import AddressLink from '$lib/components/hashi/security/AddressLink.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Select from '$lib/components/ui/select';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { counters, ipSets } from '$lib/design/security';
	import { ChevronLeft, Lock, Plus, Search, MoreHorizontal, TriangleAlert } from '@lucide/svelte';

	let { data } = $props();
	const s = $derived(data.set);
	let q = $state('');
	let addOpen = $state(false);
	let entryOpen = $state(false);
	let duration = $state('1h');
	const kindLabel = { user: 'Yours', system: 'System', feed: 'Feed', dynamic: 'Dynamic' };
	const lockReason = {
		user: '',
		system: 'Maintained by Hashi. You can use it anywhere but not change it.',
		feed: 'Owned by the feed: updated on every fetch. Change the feed settings instead.',
		dynamic: 'Filled by the abuse engine. You can remove entries or add one with a duration.'
	};
	const items = $derived(
		s.items.filter((i) =>
			(i.value + (i.note ?? '') + (i.resolved ?? []).join(' '))
				.toLowerCase()
				.includes(q.toLowerCase())
		)
	);
	const isChallenge = $derived(s.slug === 'dynamic-challenge');
	const counterOf = (a: string) => counters.find((c) => c.address === a);
	const usedAsTarget = $derived(s.usedBy.some((u) => u.kind === 'target'));
	// Mock compile: leaves become prefixes with the path they came from (05a §4.1).
	const resolve = (set: typeof s, path: string): [string, string][] =>
		set.items.flatMap((i): [string, string][] => {
			const from = path ? `${path} › ` : '';
			if (i.type === 'ip')
				return [[i.value + (i.value.includes(':') ? '/128' : '/32'), from + i.value]];
			if (i.type === 'cidr') return [[i.value, from + i.value]];
			if (i.type === 'geo')
				return [
					[
						`${i.geoKind === 'asn' ? 'ASN' : 'country'} ${i.value}`,
						from + `${i.label} (GeoIP prefixes)`
					]
				];
			if (i.type === 'set') {
				const sub = ipSets.find((x) => x.slug === i.value);
				return sub ? resolve(sub, from + sub.slug) : [];
			}
			return (i.resolved ?? []).map((r): [string, string] => [
				r + (r.includes(':') ? '/128' : '/32'),
				`${from}${i.value} (${i.type}${i.scope ? ', ' + i.scope : ''})`
			]);
		});
	const resolved = $derived(resolve(s, ''));
</script>

<a
	href="/design/ip-sets"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />IP sets</a
>

<div class="mb-5 rounded-2xl border bg-card p-4">
	<div class="flex flex-wrap items-start gap-x-6 gap-y-3">
		<div class="min-w-0 flex-1">
			<h1 class="flex flex-wrap items-center gap-2 text-[22px] leading-tight font-semibold">
				<span class="font-mono tracking-[-0.02em]">{s.slug}</span>
				{#if s.locked}<Tag><Lock class="size-3" />{kindLabel[s.kind].toLowerCase()}</Tag>{/if}
			</h1>
			<p class="text-[13.5px] text-muted-foreground">{s.description}</p>
		</div>
		<div class="flex gap-2">
			{#if s.kind === 'user'}
				<Button variant="outline" size="sm">Rename</Button>
				<Button size="sm" onclick={() => (addOpen = true)}><Plus />Add item</Button>
			{:else if s.kind === 'dynamic'}
				<Button size="sm" onclick={() => (entryOpen = true)}
					><Plus />{isChallenge ? 'Challenge an address' : 'Ban an address'}</Button
				>
			{:else if s.kind === 'feed'}
				<Button variant="outline" size="sm">Fetch now</Button>
			{/if}
		</div>
	</div>
	{#if s.locked}<p class="mt-2 text-[12.5px] text-muted-foreground">{lockReason[s.kind]}</p>{/if}
	<dl class="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 text-[13px] sm:grid-cols-4">
		<div>
			<dt class="text-[12px] text-muted-foreground">Kind</dt>
			<dd>
				{kindLabel[s.kind]}{#if s.addressSet}<span class="text-muted-foreground">
						· addresses only</span
					>{/if}
			</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Resolves to</dt>
			<dd class="tnum">{s.v4.toLocaleString()} IPv4 · {s.v6.toLocaleString()} IPv6 ranges</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Used by</dt>
			<dd>{s.usedBy.length ? s.usedBy.length + ' places' : 'Not used'}</dd>
		</div>
		<div>
			<dt class="text-[12px] text-muted-foreground">Status</dt>
			<dd>
				{#if s.status === 'partial'}<Tag tone="warn">partial</Tag>{:else}<Tag tone="ok">ok</Tag
					>{/if}
			</dd>
		</div>
	</dl>
	{#each s.problems ?? [] as p (p)}
		<p
			class="mt-3 flex items-start gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-[12.5px] text-warn-text"
		>
			<TriangleAlert class="mt-0.5 size-3.5 shrink-0" />{p}
		</p>
	{/each}
</div>

<Tabs.Root value="items">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="items">{s.kind === 'dynamic' ? 'Entries' : 'Items'}</Tabs.Trigger>
		{#if s.kind !== 'dynamic'}<Tabs.Trigger value="resolved">Resolved</Tabs.Trigger>{/if}
		<Tabs.Trigger value="used">Used by</Tabs.Trigger>
		{#if s.feed}<Tabs.Trigger value="feed">Feed</Tabs.Trigger>{/if}
		<Tabs.Trigger value="history">History</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="items">
		{#if s.kind === 'dynamic'}
			<Panel
				flush
				title="{s.entries?.length} {isChallenge ? 'challenged' : 'banned'} addresses"
				description={isChallenge
					? 'Each has to solve a CAPTCHA. Requests without solving count as failures; at 15 the address is banned.'
					: 'Dropped in the kernel on every firewall host and refused by forward auth.'}
			>
				<ul class="divide-y text-[13px]">
					{#each s.entries ?? [] as e (e.address)}
						{@const c = counterOf(e.address)}
						<li
							class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-4 py-2.5 md:grid-cols-[11rem_minmax(0,1fr)_8rem_9rem_auto]"
						>
							<AddressLink address={e.address} />
							<span class="min-w-0 md:order-none"
								><span class="block truncate">{e.reason}</span><span
									class="text-[12px] text-muted-foreground">{e.source} · since {e.created}</span
								></span
							>
							<span class="hidden md:block"
								>{#if isChallenge}<span class="tnum">{e.failures} of 15</span><span
										class="block text-[12px] text-muted-foreground">failures</span
									>{:else}<span class="tnum">ban {c?.ban ?? 1}</span><span
										class="block text-[12px] text-muted-foreground"
										>challenges {c?.challenge ?? 0}</span
									>{/if}</span
							>
							<span class="hidden text-muted-foreground md:block">{e.expires}</span>
							<DropdownMenu.Root>
								<DropdownMenu.Trigger>
									{#snippet child({ props })}<Button
											{...props}
											variant="ghost"
											size="icon-sm"
											aria-label="Actions for {e.address}"><MoreHorizontal /></Button
										>{/snippet}
								</DropdownMenu.Trigger>
								<DropdownMenu.Content align="end">
									{#if isChallenge}<DropdownMenu.Item>Ban now</DropdownMenu.Item><DropdownMenu.Item
											>Reset failures</DropdownMenu.Item
										>{/if}
									<DropdownMenu.Item>Edit counters</DropdownMenu.Item>
									<DropdownMenu.Item>Add to an IP set…</DropdownMenu.Item>
									<DropdownMenu.Separator />
									<DropdownMenu.Item class="text-bad-text"
										>{isChallenge ? 'End challenge' : 'Unban'}</DropdownMenu.Item
									>
								</DropdownMenu.Content>
							</DropdownMenu.Root>
						</li>
					{/each}
				</ul>
			</Panel>
		{:else}
			<div class="rounded-2xl border bg-card">
				<div class="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
					<div class="relative w-full max-w-80">
						<Search
							class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
						/>
						<Input
							bind:value={q}
							placeholder="Address, prefix or note"
							class="pl-8"
							aria-label="Search items"
						/>
					</div>
					<span class="ml-auto text-[12.5px] text-muted-foreground"
						>{#if s.kind === 'feed'}Showing 3 of {s.v4.toLocaleString()} · line numbers from the last
							fetch{:else}{items.length} items{/if}</span
					>
				</div>
				{#if items.length}
					<ul class="divide-y">
						{#each items as i (i.id)}
							<li
								class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-4 py-2.5 md:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_8rem_auto]"
							>
								<IpItemView item={i} />
								<span class="hidden truncate text-[12.5px] text-muted-foreground md:block"
									>{i.note ?? ''}{#if i.problem}<Tag tone="warn">{i.problem}</Tag>{/if}</span
								>
								<span class="hidden text-[12.5px] text-muted-foreground md:block"
									>{i.expires
										? 'expires ' + i.expires
										: i.added === 'system' || i.added === 'feed'
											? ''
											: 'added ' + i.added}</span
								>
								{#if !s.locked}
									<DropdownMenu.Root>
										<DropdownMenu.Trigger>
											{#snippet child({ props })}<Button
													{...props}
													variant="ghost"
													size="icon-sm"
													aria-label="Actions for {i.value}"><MoreHorizontal /></Button
												>{/snippet}
										</DropdownMenu.Trigger>
										<DropdownMenu.Content align="end">
											<DropdownMenu.Item onclick={() => (addOpen = true)}>Edit</DropdownMenu.Item>
											<DropdownMenu.Item>Set expiry</DropdownMenu.Item>
											<DropdownMenu.Separator />
											<DropdownMenu.Item class="text-bad-text">Remove</DropdownMenu.Item>
										</DropdownMenu.Content>
									</DropdownMenu.Root>
								{:else}<span></span>{/if}
							</li>
						{/each}
					</ul>
				{:else}
					<div class="px-4 py-10 text-center text-[13px] text-muted-foreground">
						{#if q}Nothing in this set matches “{q}”.{:else}This set is empty, so it matches nobody.
							<Button size="sm" variant="outline" class="ml-2" onclick={() => (addOpen = true)}
								>Add item</Button
							>{/if}
					</div>
				{/if}
			</div>
			{#if usedAsTarget}<p class="mt-2 text-[12.5px] text-muted-foreground">
					Used as a DNS value, upstream or SSH target, so only addresses can be added. If it ever
					resolves to nothing, those places keep their last working address.
				</p>{/if}
		{/if}
	</Tabs.Content>

	<Tabs.Content value="resolved">
		<Panel
			flush
			title="What this set means right now"
			description="Merged ranges after resolving Pulse agents, hosts, DNS names and nested sets"
		>
			<ul class="divide-y text-[13px]">
				{#each resolved as [prefix, from] (prefix)}
					<li class="grid grid-cols-[13rem_minmax(0,1fr)] items-center gap-3 px-4 py-2">
						<Mono>{prefix}</Mono><span class="truncate text-muted-foreground">{from}</span>
					</li>
				{/each}
			</ul>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="used">
		<Panel flush title="Used by" description="A set cannot be deleted while anything uses it">
			{#if s.usedBy.length}
				<ul class="divide-y text-[13px]">
					{#each s.usedBy as u (u.label)}
						<li class="grid grid-cols-[10rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5">
							<span class="text-muted-foreground">{u.position}</span>
							<a href={u.href} class="truncate font-medium hover:underline">{u.label}</a>
							<Tag tone={u.kind === 'target' ? 'info' : 'neutral'}
								>{u.kind === 'target' ? 'sends traffic here' : 'matches requests'}</Tag
							>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="px-4 py-6 text-[13px] text-muted-foreground">
					Nothing uses this set yet. Pick it in a rule, the firewall policy, a DNS record or a
					service.
				</p>
			{/if}
		</Panel>
		{#if s.kind === 'user'}
			<div class="mt-4 flex justify-end">
				<Button variant="ghost" class="text-bad-text" disabled={s.usedBy.length > 0}
					>Delete set</Button
				>
			</div>
		{/if}
	</Tabs.Content>

	{#if s.feed}
		<Tabs.Content value="feed" class="grid gap-4 lg:grid-cols-2">
			<Panel title="Source">
				<dl class="grid grid-cols-[8rem_minmax(0,1fr)] gap-y-2 text-[13px]">
					<dt class="text-muted-foreground">URL</dt>
					<dd class="truncate"><Mono>{s.feed.url}</Mono></dd>
					<dt class="text-muted-foreground">Format</dt>
					<dd>{s.feed.format}</dd>
					<dt class="text-muted-foreground">Refresh</dt>
					<dd>{s.feed.refresh}</dd>
					<dt class="text-muted-foreground">Last success</dt>
					<dd>{s.feed.lastSuccess}</dd>
					<dt class="text-muted-foreground">False positives</dt>
					<dd>{s.feed.fp}</dd>
				</dl>
				<p class="mt-3 text-[12.5px] text-muted-foreground">
					A failed fetch keeps the last good copy. After 72 h without success the set matches nobody
					and Overview says so.
				</p>
			</Panel>
			<Panel title="Recent fetches" flush>
				<ul class="divide-y text-[13px]">
					{#each s.feed.runs as r (r.at)}
						<li class="grid grid-cols-[8rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5">
							<span class="text-muted-foreground">{r.at}</span>
							<span class="tnum"
								>{r.parsed.toLocaleString()} parsed · +{r.added} −{r.removed}{#if r.note}<span
										class="text-muted-foreground"
									>
										· {r.note}</span
									>{/if}</span
							>
							<Tag tone={r.status === 'ok' ? 'ok' : 'bad'}>{r.status}</Tag>
						</li>
					{/each}
				</ul>
			</Panel>
		</Tabs.Content>
	{/if}

	<Tabs.Content value="history">
		<Panel flush title="History" description="From the audit log">
			<ul class="divide-y text-[13px]">
				<li class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3 px-4 py-2.5">
					<span class="text-muted-foreground">3 d ago</span><span
						>Added <Mono>vpn.example.org</Mono> (DNS) · by you</span
					>
				</li>
				<li class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3 px-4 py-2.5">
					<span class="text-muted-foreground">2 weeks ago</span><span
						>phone’s public address changed <Mono>203.0.113.12</Mono> → <Mono>203.0.113.54</Mono> · firewall
						updated on edge-fra</span
					>
				</li>
			</ul>
		</Panel>
	</Tabs.Content>
</Tabs.Root>

<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Add item to {s.slug}</Sheet.Title>
			<Sheet.Description
				>{s.usedBy.some((u) => u.position.startsWith('Firewall') || u.position === 'Host access')
					? 'This set is used by the firewall, so saving creates a plan.'
					: 'Takes effect within seconds everywhere this set is used.'}</Sheet.Description
			>
		</Sheet.Header>
		<div class="overflow-y-auto px-4 pb-4"><IpItemEditor {usedAsTarget} /></div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button
				>{s.usedBy.some((u) => u.position.startsWith('Firewall') || u.position === 'Host access')
					? 'Save and review plan'
					: 'Add item'}</Button
			>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

<Sheet.Root bind:open={entryOpen}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{isChallenge ? 'Challenge an address' : 'Ban an address'}</Sheet.Title>
			<Sheet.Description
				>{isChallenge
					? 'The address has to solve a CAPTCHA on every service.'
					: 'Dropped on every firewall host within seconds. Raises its ban counter by one.'}</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-4 px-4 pb-4">
			<div class="space-y-1.5">
				<Label for="ea">Address</Label><Input id="ea" class="font-mono" placeholder="203.0.113.9" />
				<p class="text-[12.5px] text-muted-foreground">IPv6 addresses are handled per /64.</p>
			</div>
			<div class="space-y-1.5">
				<Label>Duration</Label>
				<Select.Root type="single" bind:value={duration}>
					<Select.Trigger class="w-full">{duration}</Select.Trigger>
					<Select.Content>
						{#each isChallenge ? ['30m', '1h', '24h', 'until solved'] : ['1h', '24h', '7d', 'from the ban function', 'permanent'] as d (d)}<Select.Item
								value={d}>{d}</Select.Item
							>{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<div class="space-y-1.5"><Label for="er">Reason</Label><Input id="er" /></div>
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (entryOpen = false)}>Cancel</Button>
			<Button>{isChallenge ? 'Challenge now' : 'Ban now'}</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
