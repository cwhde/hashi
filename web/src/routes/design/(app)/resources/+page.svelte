<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import HistoryStrip from '$lib/components/hashi/HistoryStrip.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Select from '$lib/components/ui/select';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { resources } from '$lib/design/mock';
	import { Plus, Search, MoreHorizontal, Lock } from '@lucide/svelte';

	let q = $state('');
	let kind = $state('all');
	let addOpen = $state(false);
	let newKind = $state('http');
	const rows = $derived(
		resources.filter(
			(r) =>
				(kind === 'all' ||
					(kind === 'http'
						? ['http', 'https', 'h2c'].includes(r.kind)
						: ['tcp', 'udp'].includes(r.kind))) &&
				(r.name + r.domain).toLowerCase().includes(q.toLowerCase())
		)
	);
</script>

<PageHeader
	title="Resources"
	description="Services Hashi publishes. Each one gets DNS, a Traefik route, firewall openings and monitoring from this one record."
>
	{#snippet actions()}
		<Button onclick={() => (addOpen = true)}><Plus />Add resource</Button>
	{/snippet}
</PageHeader>

<div class="rounded-2xl border bg-card">
	<div class="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
		<div class="relative w-full max-w-72">
			<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
			<Input
				bind:value={q}
				placeholder="Filter by name or domain"
				class="pl-8"
				aria-label="Filter resources"
			/>
		</div>
		<ToggleGroup.Root type="single" bind:value={kind} variant="outline" size="sm">
			<ToggleGroup.Item value="all">All</ToggleGroup.Item>
			<ToggleGroup.Item value="http">HTTP</ToggleGroup.Item>
			<ToggleGroup.Item value="stream">TCP/UDP</ToggleGroup.Item>
		</ToggleGroup.Root>
		<span class="ml-auto text-[12.5px] text-muted-foreground"
			>{rows.length} of {resources.length}</span
		>
	</div>
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head class="pl-4">Resource</Table.Head>
				<Table.Head class="hidden md:table-cell">Target</Table.Head>
				<Table.Head class="hidden lg:table-cell">Access</Table.Head>
				<Table.Head class="hidden w-56 xl:table-cell">Last 60 min</Table.Head>
				<Table.Head>Status</Table.Head>
				<Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each rows as r (r.id)}
				<Table.Row class={r.enabled ? '' : 'opacity-60'}>
					<Table.Cell class="pl-4">
						<a href="/design/resources/{r.id}" class="group block">
							<span class="flex items-center gap-2 font-medium group-hover:underline">
								{r.name}
								{#if r.origin === 'system'}<Tag><Lock class="size-3" />system</Tag>{/if}
								{#if r.origin === 'imported'}<Tag>imported</Tag>{/if}
							</span>
							<span class="text-[12.5px] text-muted-foreground"
								>{r.domain}{r.port ? ':' + r.port : ''}</span
							>
						</a>
					</Table.Cell>
					<Table.Cell class="hidden md:table-cell">
						<div class="flex items-center gap-2">
							<Tag>{r.kind.toUpperCase()}</Tag><Mono class="text-muted-foreground">{r.target}</Mono>
						</div>
						<div class="mt-0.5 text-[12px] text-muted-foreground">via {r.host}</div>
					</Table.Cell>
					<Table.Cell class="hidden lg:table-cell">
						<div class="text-[13px]">{r.auth}</div>
						<div class="text-[12px] text-muted-foreground">WAF {r.waf}</div>
					</Table.Cell>
					<Table.Cell class="hidden xl:table-cell"
						><HistoryStrip samples={r.history} labels={false} /></Table.Cell
					>
					<Table.Cell>
						<StatusBadge state={r.health} />
						<div class="text-[12px] text-muted-foreground">
							{#if r.sync === 'pending'}change waiting{:else if r.sync === 'invalid'}<span
									class="text-bad-text">invalid config</span
								>{:else}{r.uptime} · {r.latency ? r.latency + ' ms' : '—'}{/if}
						</div>
					</Table.Cell>
					<Table.Cell>
						<DropdownMenu.Root>
							<DropdownMenu.Trigger>
								{#snippet child({ props })}<Button
										{...props}
										variant="ghost"
										size="icon-sm"
										aria-label="Actions for {r.name}"><MoreHorizontal /></Button
									>{/snippet}
							</DropdownMenu.Trigger>
							<DropdownMenu.Content align="end">
								<DropdownMenu.Item>Edit</DropdownMenu.Item>
								<DropdownMenu.Item>{r.enabled ? 'Disable' : 'Enable'}</DropdownMenu.Item>
								<DropdownMenu.Item>Show rendered config</DropdownMenu.Item>
								<DropdownMenu.Separator />
								<DropdownMenu.Item disabled={r.origin === 'system'} class="text-bad-text"
									>Delete…</DropdownMenu.Item
								>
							</DropdownMenu.Content>
						</DropdownMenu.Root>
					</Table.Cell>
				</Table.Row>
			{/each}
		</Table.Body>
	</Table.Root>
</div>

<!-- Larger forms open in a drawer, the list stays visible (10-ui.md §1). -->
<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Add resource</Sheet.Title>
			<Sheet.Description
				>Saving creates a plan. Nothing is published until it is applied.</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-5 overflow-y-auto px-4 pb-4">
			<div class="grid gap-4 sm:grid-cols-2">
				<div class="space-y-1.5"><Label for="rn">Name</Label><Input id="rn" value="Photos" /></div>
				<div class="space-y-1.5">
					<Label>Protocol</Label>
					<Select.Root type="single" bind:value={newKind}>
						<Select.Trigger class="w-full">{newKind.toUpperCase()}</Select.Trigger>
						<Select.Content>
							{#each ['http', 'https', 'h2c', 'tcp', 'udp'] as k (k)}<Select.Item value={k}
									>{k.toUpperCase()}</Select.Item
								>{/each}
						</Select.Content>
					</Select.Root>
				</div>
			</div>
			<div class="space-y-1.5">
				<Label for="rd">Public domain</Label>
				<div class="flex items-center gap-2">
					<Input id="rd" value="photos" class="max-w-48" /><span
						class="text-[13px] text-muted-foreground">.example.com</span
					>
				</div>
				<p class="text-[12.5px] text-muted-foreground">
					DNS: CNAME to on.edge-home.example.com (target is in edge-home's subnet 10.20.0.0/22).
				</p>
			</div>
			<fieldset class="space-y-2">
				<legend class="mb-1.5 text-[13px] font-medium">Upstream</legend>
				<div class="grid grid-cols-[6rem_1fr_6rem] gap-2">
					<Input value="http" aria-label="Scheme" /><Input
						value="10.20.1.102"
						aria-label="Host"
					/><Input value="2283" aria-label="Port" />
				</div>
				<p class="text-[12.5px] text-muted-foreground">
					Or point at a Pulse agent so the target follows its IP.
				</p>
			</fieldset>
			<div class="space-y-3 rounded-xl border p-3">
				<div class="flex items-center justify-between gap-3">
					<div>
						<div class="text-[13px] font-medium">Sign-in</div>
						<div class="text-[12.5px] text-muted-foreground">
							SSO required, adaptive, observe or off
						</div>
					</div>
					<Tag tone="accent">SSO required</Tag>
				</div>
				<div class="flex items-center justify-between gap-3">
					<Label for="s1">Show on app dashboard</Label><Switch id="s1" checked />
				</div>
				<div class="flex items-center justify-between gap-3">
					<Label for="s2">Monitor</Label><Switch id="s2" checked />
				</div>
				<div class="flex items-center justify-between gap-3">
					<Label for="s3">Internal DNS rewrite (AdGuard)</Label><Switch id="s3" checked />
				</div>
			</div>
			<p class="text-[12.5px] text-muted-foreground">
				Advanced routes, rewrites, TLS options and rules are on the resource page after saving.
			</p>
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button href="/design/changes">Save and review plan</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
