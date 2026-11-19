<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Table from '#lib/components/ui/table/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { Plus } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { dateTime } from '#lib/format.ts';

	// 08, 10b §3: the agents and their addresses. Each name opens the agent; adding one, and its token, is on
	// Connections with every other outside system (10c §1, bug 24).
	type Agent = Schemas['PulseAgent'];
	let agents = $state<Agent[]>([]);
	let error = $state('');
	let loaded = $state(false);

	async function load() {
		const r = await api.GET('/api/pulse/agents');
		if (!r.data) throw r.error;
		agents = r.data.agents;
		loaded = true;
	}
	onMount(() => {
		load().catch((e) => {
			error = problemText(e);
			loaded = true;
		});
		const t = setInterval(() => void load().catch(() => {}), 30000);
		return () => clearInterval(t);
	});
</script>

<svelte:head><title>Pulse · Hashi</title></svelte:head>

<PageHeader title="Pulse">
	{#snippet actions()}<Button href="/connections?add=pulse"><Plus />Add agent</Button>{/snippet}
</PageHeader>

{#if error}<p role="alert" class="mb-3 text-md text-bad-text">{error}</p>{/if}
<div class="rounded-2xl border bg-card">
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head class="pl-4">Agent</Table.Head>
				<Table.Head>Main address</Table.Head>
				<Table.Head class="hidden md:table-cell">Public address</Table.Head>
				<Table.Head class="hidden sm:table-cell">Reachable from Hashi</Table.Head>
				<Table.Head class="pr-4">Last seen</Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each agents as a (a.id)}
				<Table.Row>
					<Table.Cell class="pl-4">
						<a href="/pulse/{a.id}" class="font-medium hover:underline">{a.name}</a>
						<div class="text-xs text-muted-foreground">
							{a.version || 'No heartbeat yet'}{a.hostname ? ` · ${a.hostname}` : ''}
						</div>
						{#if a.version_state === 'older'}<Tag tone="warn">older than {a.expected_version}</Tag>
						{:else if a.version_state === 'newer'}<Tag tone="info"
								>newer than {a.expected_version}</Tag
							>{/if}
					</Table.Cell>
					<Table.Cell
						><Mono>{a.selected_ip || '—'}</Mono>{#if a.pinned_ip}
							<Tag>chosen</Tag>{/if}</Table.Cell
					>
					<Table.Cell class="hidden md:table-cell"
						><Mono>{a.public_ip || '—'}</Mono>{#if a.public_ip_source === 'reported'}
							<Tag>self-reported</Tag>{/if}</Table.Cell
					>
					<Table.Cell class="hidden sm:table-cell">
						{#if a.reachable === true}<Tag tone="ok">yes</Tag>
						{:else if a.reachable === false}<Tag tone="warn">no</Tag>
						{:else}<span class="text-muted-foreground">—</span>{/if}
					</Table.Cell>
					<Table.Cell class="pr-4">
						{#if a.status === 'revoked'}<Tag tone="bad">revoked</Tag>
						{:else if a.status === 'waiting'}<Tag>waiting</Tag>
						{:else if a.status === 'stale'}<Tag tone="warn"
								>stale · {dateTime(a.last_seen_at ?? '')}</Tag
							>
						{:else}<span class="text-md">{dateTime(a.last_seen_at ?? '')}</span>{/if}
					</Table.Cell>
				</Table.Row>
			{:else}
				{#if loaded}
					<Table.Row>
						<Table.Cell colspan={5} class="px-4 py-4 text-md text-muted-foreground"
							>No agents yet.</Table.Cell
						>
					</Table.Row>
				{:else}
					{#each [0, 1, 2, 3] as i (i)}
						<Table.Row>
							<Table.Cell colspan={5} class="px-4"><Skeleton class="h-6 w-full" /></Table.Cell>
						</Table.Row>
					{/each}
				{/if}
			{/each}
		</Table.Body>
	</Table.Root>
</div>
