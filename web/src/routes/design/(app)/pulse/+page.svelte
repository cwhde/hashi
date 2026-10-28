<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Tabs from '$lib/components/ui/tabs';
	import { agents } from '$lib/design/mock';
	import { Plus, Copy } from '@lucide/svelte';

	let open = $state(false);
</script>

<PageHeader
	title="Pulse"
	description="Small agents that report a machine's current addresses. Put an agent in an IP set and firewall rules, DNS values, upstreams and connections follow its address."
>
	{#snippet actions()}<Button onclick={() => (open = true)}><Plus />Add agent</Button>{/snippet}
</PageHeader>

<div class="rounded-2xl border bg-card">
	<Table.Root>
		<Table.Header
			><Table.Row
				><Table.Head class="pl-4">Agent</Table.Head><Table.Head>Selected IP</Table.Head><Table.Head
					class="hidden md:table-cell">Public IP</Table.Head
				><Table.Head class="hidden lg:table-cell">Other candidates</Table.Head><Table.Head
					>Reachable from Hashi</Table.Head
				><Table.Head>Last seen</Table.Head></Table.Row
			></Table.Header
		>
		<Table.Body>
			{#each agents as a (a.id)}
				<Table.Row>
					<Table.Cell class="pl-4"
						><div class="font-medium">{a.name}</div>
						<div class="text-[12px] text-muted-foreground">
							v{a.version} · {a.install}
						</div></Table.Cell
					>
					<Table.Cell><Mono>{a.selected}</Mono></Table.Cell>
					<Table.Cell class="hidden md:table-cell"><Mono>{a.publicIp}</Mono></Table.Cell>
					<Table.Cell class="hidden text-muted-foreground lg:table-cell"
						><Mono>{a.candidates.slice(1).join(', ') || '—'}</Mono></Table.Cell
					>
					<Table.Cell
						>{#if a.reach === 'reachable'}<Tag tone="ok">yes</Tag>{:else}<Tag tone="warn">no</Tag
							>{/if}</Table.Cell
					>
					<Table.Cell
						>{#if a.status === 'stale'}<Tag tone="warn">stale · {a.lastSeen}</Tag>{:else}<span
								class="text-[13px]">{a.lastSeen}</span
							>{/if}</Table.Cell
					>
				</Table.Row>
			{/each}
		</Table.Body>
	</Table.Root>
</div>

<Sheet.Root bind:open>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header
			><Sheet.Title>Add a Pulse agent</Sheet.Title><Sheet.Description
				>The token is shown once. It can only send this agent's heartbeat.</Sheet.Description
			></Sheet.Header
		>
		<div class="space-y-4 px-4">
			<div class="space-y-1.5"><Label for="an">Name</Label><Input id="an" value="media-box" /></div>
			<Tabs.Root value="linux">
				<Tabs.List
					><Tabs.Trigger value="linux">Linux</Tabs.Trigger><Tabs.Trigger value="docker"
						>Docker</Tabs.Trigger
					></Tabs.List
				>
				<Tabs.Content value="linux">
					<div class="relative mt-2 rounded-xl border bg-surface-2/60 p-3 pr-10">
						<Mono class="break-all"
							>curl -fsSL https://hashi.example.com/api/pulse/agents/a4/install.sh | sudo sh -s --
							--token ••••••••</Mono
						>
						<Button
							variant="ghost"
							size="icon-sm"
							class="absolute top-2 right-2"
							aria-label="Copy command"><Copy /></Button
						>
					</div>
					<p class="mt-2 text-[12.5px] text-muted-foreground">
						Downloads the binary, checks its SHA-256, installs a systemd service.
					</p>
				</Tabs.Content>
				<Tabs.Content value="docker"
					><pre class="mt-2 rounded-xl border bg-surface-2/60 p-3 font-mono text-[12.5px]">services:
  pulse:
    image: git.example.com/hashi-pulse:latest
    network_mode: host
    environment:
      HASHI_URL: https://hashi.example.com</pre></Tabs.Content
				>
			</Tabs.Root>
		</div>
		<Sheet.Footer class="flex-row justify-end border-t"
			><Button onclick={() => (open = false)}>Done</Button></Sheet.Footer
		>
	</Sheet.Content>
</Sheet.Root>
