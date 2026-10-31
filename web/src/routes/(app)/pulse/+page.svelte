<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Table from '$lib/components/ui/table';
	import * as Tabs from '$lib/components/ui/tabs';
	import { Copy, KeyRound, Pencil, Plus, Ban, Trash2 } from '@lucide/svelte';
	import { Switch } from '$lib/components/ui/switch';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { dateTime } from '$lib/format';

	// 08, 10b §3: agents and their addresses; the add drawer shows the install once, with the token.
	type Agent = Schemas['PulseAgent'];
	let agents = $state<Agent[]>([]);
	let hashiUrl = $state('');
	let error = $state('');
	let busy = $state(false);

	async function load() {
		const r = await api.GET('/api/pulse/agents');
		if (!r.data) throw r.error;
		agents = r.data.agents;
		hashiUrl = r.data.hashi_url;
	}
	onMount(() => {
		load().catch((e) => (error = problemText(e)));
		const t = setInterval(() => void load().catch(() => {}), 30000);
		return () => clearInterval(t);
	});

	let open = $state(false);
	let name = $state('');
	let formError = $state('');
	let shown = $state<{ agent: Agent; token: string } | null>(null);
	function openAdd() {
		name = '';
		shown = null;
		formError = '';
		open = true;
	}
	async function create() {
		formError = '';
		busy = true;
		try {
			const r = await api.POST('/api/pulse/agents', { body: { name } });
			if (!r.data) throw r.error;
			shown = r.data;
			await load();
		} catch (e) {
			formError = problemText(e);
		} finally {
			busy = false;
		}
	}
	async function act(fn: () => Promise<void>) {
		error = '';
		busy = true;
		try {
			await fn();
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const rotate = (a: Agent) =>
		act(async () => {
			shown = await withReauth(() =>
				api.POST('/api/pulse/agents/{id}/rotate-token', { params: { path: { id: a.id } } })
			);
			formError = '';
			open = true;
		});
	const revoke = (a: Agent) =>
		act(async () => {
			await withReauth(() =>
				api.POST('/api/pulse/agents/{id}/revoke', { params: { path: { id: a.id } } })
			);
		});
	const remove = (a: Agent) =>
		act(async () => {
			await withReauth(() =>
				api.DELETE('/api/pulse/agents/{id}', { params: { path: { id: a.id } } })
			);
		});

	// ---- edit ----
	let editing = $state<{
		id: string;
		name: string;
		dns_enabled: boolean;
		dns_name: string;
		trust_reported_ip: boolean;
	} | null>(null);
	let editError = $state('');
	async function saveEdit() {
		if (!editing) return;
		editError = '';
		busy = true;
		try {
			const { id, ...body } = editing;
			const r = await api.PUT('/api/pulse/agents/{id}', { params: { path: { id } }, body });
			if (r.error) throw r.error;
			editing = null;
			await load();
		} catch (e) {
			editError = problemText(e);
		} finally {
			busy = false;
		}
	}

	const linux = $derived(
		shown
			? `curl -fsSL ${hashiUrl}/api/pulse/install.sh | sudo sh -s -- --url ${hashiUrl} --id ${shown.agent.id} --token ${shown.token}`
			: ''
	);
	const compose = $derived(
		shown
			? `services:
  pulse:
    image: git.juzo.io/juzo/hashi-pulse:latest
    restart: unless-stopped
    network_mode: host
    environment:
      HASHI_URL: ${hashiUrl}
      HASHI_AGENT_ID: ${shown.agent.id}
      HASHI_TOKEN: ${shown.token}`
			: ''
	);
	const copy = (t: string) => void navigator.clipboard?.writeText(t);
</script>

<PageHeader
	title="Pulse"
	description="Small agents that report a machine's current addresses. Put an agent in an IP set and everything using that set follows it."
>
	{#snippet actions()}<Button onclick={openAdd}><Plus />Add agent</Button>{/snippet}
</PageHeader>

{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}
<div class="rounded-2xl border bg-card">
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head class="pl-4">Agent</Table.Head>
				<Table.Head>Selected IP</Table.Head>
				<Table.Head class="hidden md:table-cell">Public IP</Table.Head>
				<Table.Head class="hidden lg:table-cell">Other candidates</Table.Head>
				<Table.Head>Last seen</Table.Head>
				<Table.Head><span class="sr-only">Actions</span></Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each agents as a (a.id)}
				<Table.Row>
					<Table.Cell class="pl-4">
						<div class="font-medium">{a.name}</div>
						<div class="text-[12px] text-muted-foreground">
							{a.version ? `v${a.version}` : 'No heartbeat yet'}{a.hostname
								? ` · ${a.hostname}`
								: ''}
						</div>
					</Table.Cell>
					<Table.Cell><Mono>{a.selected_ip || '—'}</Mono></Table.Cell>
					<Table.Cell class="hidden md:table-cell"
						><Mono>{a.public_ip || '—'}</Mono>{#if a.public_ip_source === 'reported'}
							<Tag>self-reported</Tag>{/if}</Table.Cell
					>
					<Table.Cell class="hidden text-muted-foreground lg:table-cell"
						><Mono>{a.candidates.filter((c) => c !== a.selected_ip).join(', ') || '—'}</Mono
						></Table.Cell
					>
					<Table.Cell>
						{#if a.status === 'revoked'}<Tag tone="bad">revoked</Tag>
						{:else if a.status === 'waiting'}<Tag>waiting</Tag>
						{:else if a.status === 'stale'}<Tag tone="warn"
								>stale · {dateTime(a.last_seen_at ?? '')}</Tag
							>
						{:else}<span class="text-[13px]">{dateTime(a.last_seen_at ?? '')}</span>{/if}
					</Table.Cell>
					<Table.Cell class="text-right whitespace-nowrap">
						<Button
							variant="ghost"
							size="icon-sm"
							aria-label="Edit {a.name}"
							onclick={() => {
								editError = '';
								editing = {
									id: a.id,
									name: a.name,
									dns_enabled: a.dns_enabled,
									dns_name: a.dns_name,
									trust_reported_ip: a.trust_reported_ip
								};
							}}
							disabled={busy}><Pencil /></Button
						>
						<Button
							variant="ghost"
							size="icon-sm"
							aria-label="New token for {a.name}"
							onclick={() => rotate(a)}
							disabled={busy}><KeyRound /></Button
						>
						{#if a.status !== 'revoked'}<Button
								variant="ghost"
								size="icon-sm"
								aria-label="Revoke {a.name}"
								onclick={() => revoke(a)}
								disabled={busy}><Ban /></Button
							>{/if}
						<Button
							variant="ghost"
							size="icon-sm"
							aria-label="Remove {a.name}"
							onclick={() => remove(a)}
							disabled={busy}><Trash2 /></Button
						>
					</Table.Cell>
				</Table.Row>
			{:else}
				<Table.Row>
					<Table.Cell colspan={6} class="px-4 py-4 text-[13px] text-muted-foreground"
						>No agents yet. Add one to a machine whose address can change.</Table.Cell
					>
				</Table.Row>
			{/each}
		</Table.Body>
	</Table.Root>
</div>

<Sheet.Root bind:open>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>{shown ? `Install ${shown.agent.name}` : 'Add a Pulse agent'}</Sheet.Title>
			<Sheet.Description
				>The token is shown once. It can only send this agent's heartbeat.</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-4 overflow-y-auto px-4">
			{#if !shown}
				<div class="space-y-1.5">
					<Label for="agent-name">Name</Label><Input id="agent-name" bind:value={name} />
				</div>
				{#if formError}<p role="alert" class="text-[13px] text-bad-text">{formError}</p>{/if}
			{:else}
				<Tabs.Root value="linux">
					<Tabs.List>
						<Tabs.Trigger value="linux">Linux</Tabs.Trigger>
						<Tabs.Trigger value="docker">Docker</Tabs.Trigger>
					</Tabs.List>
					<Tabs.Content value="linux">
						<div class="relative mt-2 rounded-xl border bg-surface-2/60 p-3 pr-10">
							<Mono class="break-all">{linux}</Mono>
							<Button
								variant="ghost"
								size="icon-sm"
								class="absolute top-2 right-2"
								aria-label="Copy command"
								onclick={() => copy(linux)}><Copy /></Button
							>
						</div>
						<p class="mt-2 text-[12.5px] text-muted-foreground">
							Downloads the binary from Hashi, checks its SHA-256 and installs a service.
						</p>
					</Tabs.Content>
					<Tabs.Content value="docker">
						<div class="relative mt-2 rounded-xl border bg-surface-2/60 p-3 pr-10">
							<pre class="overflow-x-auto font-mono text-[12.5px]">{compose}</pre>
							<Button
								variant="ghost"
								size="icon-sm"
								class="absolute top-2 right-2"
								aria-label="Copy snippet"
								onclick={() => copy(compose)}><Copy /></Button
							>
						</div>
					</Tabs.Content>
				</Tabs.Root>
			{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			{#if shown}
				<Button onclick={() => (open = false)}>Done</Button>
			{:else}
				<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
				<Button onclick={create} disabled={busy || !name.trim()}>Create agent</Button>
			{/if}
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

<Sheet.Root open={!!editing} onOpenChange={(o) => !o && (editing = null)}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>Edit agent</Sheet.Title>
			<Sheet.Description>Its token stays the same.</Sheet.Description>
		</Sheet.Header>
		{#if editing}
			<div class="space-y-4 overflow-y-auto px-4 text-[13px]">
				<div class="space-y-1.5">
					<Label for="edit-name">Name</Label><Input id="edit-name" bind:value={editing.name} />
				</div>
				<label class="flex items-center justify-between gap-3">
					<span>Give it an internal DNS name</span><Switch
						bind:checked={editing.dns_enabled}
						aria-label="Give it an internal DNS name"
					/>
				</label>
				<div class="space-y-1.5">
					<Label for="edit-dns">DNS name (optional)</Label><Input
						id="edit-dns"
						bind:value={editing.dns_name}
						placeholder="Uses the agent's name"
					/>
					<p class="text-[12px] text-muted-foreground">Needed only if two agents would clash.</p>
				</div>
				<label class="flex items-center justify-between gap-3">
					<span>Trust its own report of its public address</span><Switch
						bind:checked={editing.trust_reported_ip}
						aria-label="Trust its own report of its public address"
					/>
				</label>
				<p class="text-[12px] text-muted-foreground">
					For a machine that reaches Hashi only over a private path. The agent then asks Cloudflare
					and OpenDNS every few minutes, and this address can reach firewall rules through IP sets.
					If its token were stolen, someone could name any public address here.
				</p>
				{#if editError}<p role="alert" class="text-bad-text">{editError}</p>{/if}
			</div>
			<Sheet.Footer class="flex-row justify-end gap-2 border-t">
				<Button variant="outline" onclick={() => (editing = null)}>Cancel</Button>
				<Button onclick={saveEdit} disabled={busy || !editing.name.trim()}>Save changes</Button>
			</Sheet.Footer>
		{/if}
	</Sheet.Content>
</Sheet.Root>
