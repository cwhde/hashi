<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Select from '$lib/components/ui/select';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { rrsets, zone, agents, type Rrset } from '$lib/design/mock';
	import { Plus, Search, Pencil, Lock, RefreshCw, ChevronDown } from '@lucide/svelte';

	let showGenerated = $state(false);
	let q = $state('');
	let editing = $state<string | null>('d12');
	const rows = $derived(
		rrsets.filter(
			(r) =>
				(showGenerated || !r.generated) &&
				(r.name + r.values.join(' ')).toLowerCase().includes(q.toLowerCase())
		)
	);

	const originTone = (o: Rrset['origin']) =>
		o === 'system' || o === 'managed' ? 'accent' : o === 'unmanaged' ? 'neutral' : 'neutral';
	let decisions = $state<Record<string, string>>({
		d15: 'prune',
		d3: 'import',
		d4: 'import',
		d14: 'import',
		d16: 'leave'
	});
</script>

<PageHeader
	title="DNS"
	description="Hashi manages one zone. Records it does not own are shown but never changed outside the import and prune flow."
>
	{#snippet actions()}
		<span class="text-[12.5px] text-muted-foreground"
			>{zone.name} · {zone.provider} · synced {zone.lastSync}</span
		>
		<Button variant="outline" size="sm"><RefreshCw />Refresh</Button>
	{/snippet}
</PageHeader>

<Tabs.Root value="records">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="records">Records</Tabs.Trigger>
		<Tabs.Trigger value="import">Import <Tag class="ml-1">5 new</Tag></Tabs.Trigger>
		<Tabs.Trigger value="adguard">AdGuard rewrites</Tabs.Trigger>
		<Tabs.Trigger value="agents">Internal agent names</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="records">
		<div class="rounded-2xl border bg-card">
			<div class="flex flex-wrap items-center gap-3 border-b px-3 py-2.5">
				<div class="relative w-full max-w-72">
					<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
					<Input
						bind:value={q}
						placeholder="Filter records"
						class="pl-8"
						aria-label="Filter records"
					/>
				</div>
				<label class="flex items-center gap-2 text-[13px]"
					><Switch bind:checked={showGenerated} />Show generated</label
				>
				<Button size="sm" class="ml-auto"><Plus />Add record</Button>
			</div>
			<div role="table" aria-label="DNS records" class="text-[13px]">
				<div
					role="row"
					class="grid grid-cols-[minmax(8rem,1fr)_4.5rem_4.5rem_minmax(10rem,2fr)_7rem_2.5rem] gap-3 border-b px-4 py-2 text-[12px] font-medium text-muted-foreground"
				>
					<span role="columnheader">Name</span><span role="columnheader">Type</span><span
						role="columnheader">TTL</span
					><span role="columnheader">Value</span><span role="columnheader">Owner</span><span></span>
				</div>
				{#each rows as r (r.id)}
					<div
						role="row"
						class="grid grid-cols-[minmax(8rem,1fr)_4.5rem_4.5rem_minmax(10rem,2fr)_7rem_2.5rem] items-center gap-3 border-b px-4 py-2 last:border-b-0 hover:bg-surface-2/40 {editing ===
						r.id
							? 'bg-surface-2/40'
							: ''}"
					>
						<Mono class="truncate">{r.name}</Mono>
						<span class="font-medium">{r.type}</span>
						<span class="tnum text-muted-foreground">{r.ttl === 'zone' ? 'zone' : r.ttl}</span>
						<span class="min-w-0"
							>{#each r.values as v, i (i)}<Mono class="block truncate">{v}</Mono>{/each}</span
						>
						<span>
							{#if r.protected}<Tag><Lock class="size-3" />protected</Tag>{:else}<Tag
									tone={originTone(r.origin)}>{r.origin}</Tag
								>{/if}
						</span>
						<span>
							{#if !r.protected && !r.generated && r.origin !== 'unmanaged'}
								<Button
									variant="ghost"
									size="icon-sm"
									aria-label="Edit {r.name} {r.type}"
									onclick={() => (editing = editing === r.id ? null : r.id)}><Pencil /></Button
								>
							{/if}
						</span>
					</div>
					{#if editing === r.id}
						<!-- Inline editor under the row, full width (10a §5.2 items 9–10). -->
						<div class="border-b bg-surface-2/40 px-4 pt-1 pb-4">
							<div class="grid gap-3 sm:grid-cols-[minmax(8rem,1fr)_6rem_6rem_minmax(10rem,2fr)]">
								<div class="space-y-1">
									<Label class="text-[12px]">Name</Label><Input value={r.name} />
								</div>
								<div class="space-y-1">
									<Label class="text-[12px]">Type</Label>
									<Select.Root type="single" value={r.type}
										><Select.Trigger class="w-full">{r.type}</Select.Trigger><Select.Content
											>{#each ['A', 'AAAA', 'CNAME', 'MX', 'TXT'] as t (t)}<Select.Item value={t}
													>{t}</Select.Item
												>{/each}</Select.Content
										></Select.Root
									>
								</div>
								<div class="space-y-1">
									<Label class="text-[12px]">TTL</Label><Input value={String(r.ttl)} />
								</div>
								<div class="space-y-1">
									<Label class="text-[12px]">Value</Label><Input
										value={r.values[0]}
										class="font-mono"
									/>
								</div>
							</div>
							<Collapsible.Root class="mt-3">
								<Collapsible.Trigger
									class="flex items-center gap-1 text-[12.5px] text-muted-foreground hover:text-foreground"
									><ChevronDown class="size-3.5" />Display, monitoring and dashboard</Collapsible.Trigger
								>
								<Collapsible.Content class="mt-2 grid gap-3 sm:grid-cols-3">
									<div class="space-y-1">
										<Label class="text-[12px]">Display name</Label><Input value="NAS" />
									</div>
									<label class="flex items-center gap-2 pt-5 text-[13px]"
										><Switch checked />Monitor</label
									>
									<label class="flex items-center gap-2 pt-5 text-[13px]"
										><Switch />Show on app dashboard</label
									>
								</Collapsible.Content>
							</Collapsible.Root>
							<div class="mt-3 flex items-center gap-2">
								<Button variant="ghost" size="sm" class="text-bad-text">Delete record</Button>
								<span class="flex-1"></span>
								<Button variant="ghost" size="sm" onclick={() => (editing = null)}>Cancel</Button>
								<Button size="sm">Save · creates a plan</Button>
							</div>
						</div>
					{/if}
				{/each}
			</div>
		</div>
		<p class="mt-2 text-[12.5px] text-muted-foreground">
			NS and SOA are never changed by Hashi. Generated records (hosts, services, ACME) are edited
			through the service or host that owns them.
		</p>
	</Tabs.Content>

	<Tabs.Content value="import">
		<Panel
			title="Records Hashi does not own yet"
			description="Leave them alone (default), take them over as managed entries, or mark them for removal"
			flush
		>
			{#snippet actions()}<Button size="sm">Save decisions</Button>{/snippet}
			<ul class="divide-y">
				{#each rrsets.filter((r) => r.origin === 'imported' || r.origin === 'unmanaged') as r (r.id)}
					<li
						class="grid items-center gap-3 px-4 py-2.5 text-[13px] md:grid-cols-[minmax(0,1fr)_auto]"
					>
						<div class="min-w-0">
							<span class="font-medium"><Mono>{r.name}</Mono> {r.type}</span>
							<span class="block truncate text-muted-foreground"
								><Mono>{r.values.join(', ')}</Mono></span
							>
						</div>
						{#if r.protected}
							<span class="text-[12.5px] text-muted-foreground"
								>NS and SOA cannot be imported or removed</span
							>
						{:else}
							<div
								class="inline-flex rounded-[10px] border bg-surface-2 p-0.5"
								role="radiogroup"
								aria-label="Decision for {r.name} {r.type}"
							>
								{#each [['leave', 'Leave'], ['import', 'Import'], ['prune', 'Remove']] as [v, label] (v)}
									<button
										role="radio"
										aria-checked={(decisions[r.id] ?? 'leave') === v}
										onclick={() => (decisions[r.id] = v ?? 'leave')}
										class="h-7 rounded-lg px-2.5 text-[12.5px] {(decisions[r.id] ?? 'leave') === v
											? v === 'prune'
												? 'bg-bad/15 text-bad-text'
												: 'bg-card text-foreground shadow-sm'
											: 'text-muted-foreground'}">{label}</button
									>
								{/each}
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		</Panel>
		<p class="mt-2 text-[12.5px] text-muted-foreground">
			Removals become a separate high-risk plan that lists every record and asks for your passkey.
		</p>
	</Tabs.Content>

	<Tabs.Content value="adguard" class="grid gap-4 lg:grid-cols-2">
		{#each ['adguard-home', 'adguard-fra'] as inst, i (inst)}
			<Panel
				title={inst}
				description={i === 0
					? 'http://10.20.1.100:3000 · 31 rewrites, 18 owned by Hashi'
					: 'Agent-bound: follows nas · 18 owned'}
			>
				{#snippet actions()}<StatusBadge
						state={i === 0 ? 'up' : 'degraded'}
						label={i === 0 ? 'In sync' : '1 change'}
					/>{/snippet}
				<ul class="space-y-1.5 text-[13px]">
					<li class="flex justify-between gap-3">
						<Mono>grafana.example.com</Mono><Mono class="text-muted-foreground">10.20.1.11</Mono>
					</li>
					<li class="flex justify-between gap-3">
						<Mono>git.example.com</Mono><Mono class="text-muted-foreground">10.20.4.11</Mono>
					</li>
					<li class="flex justify-between gap-3">
						<Mono>printer.lan</Mono><span class="text-muted-foreground">manual · untouched</span>
					</li>
				</ul>
			</Panel>
		{/each}
	</Tabs.Content>

	<Tabs.Content value="agents">
		<Panel
			title="hashi.home.arpa"
			description="DNS only, not reverse proxy: stable internal names for Pulse agents"
		>
			{#snippet actions()}<Button size="sm" variant="outline">Settings</Button>{/snippet}
			<ul class="divide-y text-[13px]">
				{#each agents as a (a.id)}
					<li class="flex items-center gap-3 py-2">
						<Mono class="flex-1">{a.dns}</Mono><Mono class="text-muted-foreground"
							>{a.selected}</Mono
						>{#if a.status === 'stale'}<Tag tone="warn">stale · kept</Tag>{:else}<Tag tone="ok"
								>fresh</Tag
							>{/if}
					</li>
				{/each}
			</ul>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
