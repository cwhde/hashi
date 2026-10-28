<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Select from '$lib/components/ui/select';
	import { monitors } from '$lib/design/mock';
	import { Plus, Search, Send } from '@lucide/svelte';

	let groupBy = $state('group');
	const groups = $derived(
		Object.entries(Object.groupBy(monitors, (m) => (groupBy === 'state' ? m.state : m.group)))
	);
</script>

<PageHeader
	title="Status"
	description="Built-in monitoring for services, hosts, connections and your own checks."
>
	{#snippet actions()}<Button><Plus />Add check</Button>{/snippet}
</PageHeader>

<Tabs.Root value="endpoints">
	<Tabs.List class="mb-4"
		><Tabs.Trigger value="endpoints">Endpoints</Tabs.Trigger><Tabs.Trigger value="notify"
			>Notifications</Tabs.Trigger
		></Tabs.List
	>
	<Tabs.Content value="endpoints">
		<div class="mb-4 flex flex-wrap items-center gap-2">
			<div class="relative w-full max-w-72">
				<Search
					class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
				/><Input placeholder="Search endpoints" class="pl-8" aria-label="Search endpoints" />
			</div>
			<Select.Root type="single" bind:value={groupBy}>
				<Select.Trigger class="w-44"
					>Group by {groupBy === 'state' ? 'status' : 'group'}</Select.Trigger
				>
				<Select.Content
					><Select.Item value="group">Group by group</Select.Item><Select.Item value="state"
						>Group by status</Select.Item
					></Select.Content
				>
			</Select.Root>
		</div>
		<div class="space-y-6">
			{#each groups as [name, items] (name)}
				{@const failing = (items ?? []).filter(
					(m) => m.state === 'down' || m.state === 'degraded'
				).length}
				<section>
					<h2 class="mb-2 flex items-center gap-2 text-[14px] font-semibold capitalize">
						{name}{#if failing}<Tag tone="bad">{failing} failing</Tag>{/if}
					</h2>
					<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
						{#each items ?? [] as m (m.id)}
							<a
								href="/design/status/{m.id}"
								class="block rounded-2xl border bg-card p-3.5 transition-colors hover:border-muted-foreground/40"
							>
								<div class="mb-2.5 flex items-start gap-3">
									<div class="min-w-0 flex-1">
										<div class="truncate text-[13.5px] font-medium">{m.name}</div>
										<div class="truncate text-[12px] text-muted-foreground">
											{m.kind} · {m.address}
										</div>
									</div>
									<StatusBadge state={m.state} />
								</div>
								<HistoryStrip samples={m.history} />
								<div class="mt-1.5 flex justify-between text-[12px] text-muted-foreground">
									<span>{m.source}</span><span class="tnum"
										>{m.latency ? m.latency + ' ms' : ''} · {m.uptime24}</span
									>
								</div>
							</a>
						{/each}
					</div>
				</section>
			{/each}
		</div>
	</Tabs.Content>
	<Tabs.Content value="notify" class="grid gap-4 lg:grid-cols-2">
		<Panel title="Where alerts go" description="Default route, then per-endpoint overrides">
			<ul class="divide-y text-[13px]">
				<li class="flex items-center gap-3 py-2.5">
					<span class="flex-1">Everything (default)</span><span class="text-muted-foreground"
						>Telegram “Ops” · down + recovery · 10 min cooldown</span
					>
				</li>
				<li class="flex items-center gap-3 py-2.5">
					<span class="flex-1">Infrastructure</span><span class="text-muted-foreground"
						>Telegram + Email · also degraded</span
					>
				</li>
				<li class="flex items-center gap-3 py-2.5">
					<span class="flex-1">Pending high-risk changes</span><span class="text-muted-foreground"
						>Telegram</span
					>
				</li>
			</ul>
		</Panel>
		<Panel title="Recent notifications">
			<ul class="space-y-2.5 text-[13px]">
				<li class="flex items-center gap-3">
					<Send class="size-4 text-muted-foreground" /><span class="flex-1"
						>Home Assistant is down</span
					><span class="text-muted-foreground">Telegram · 13:35 · delivered</span>
				</li>
				<li class="flex items-center gap-3">
					<Send class="size-4 text-muted-foreground" /><span class="flex-1">Media is degraded</span
					><span class="text-muted-foreground">Telegram · 13:12 · delivered</span>
				</li>
				<li class="flex items-center gap-3">
					<Send class="size-4 text-muted-foreground" /><span class="flex-1"
						>Weekly certificate report</span
					><span class="text-bad-text">Email · failed, retry 13:50</span>
				</li>
			</ul>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
