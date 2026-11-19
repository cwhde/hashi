<script lang="ts">
	// One Pulse agent (08 §2, bug 22): what it reports per interface, which address is its main one, its own settings
	// and what follows it. Its token and install live on Connections, with every other outside system.
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.ts';
	import { ChevronLeft } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { dateTime } from '#lib/format.ts';
	import { consumerOf } from '#lib/consumers.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';

	type Agent = Schemas['PulseAgent'];
	const id = $derived(page.params.id ?? '');
	let agent = $state<Agent | null>(null);
	let uses = $state<Schemas['PulseConsumers'] | null>(null);
	let form = $state({
		name: '',
		dns_enabled: true,
		dns_name: '',
		trust_reported_ip: false,
		reach_port: 0
	});
	let main = $state('');
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);
	let missing = $state(false);
	const draft = new Draft(() => form);
	guardUnsaved(() => draft.dirty);

	function show(a: Agent) {
		agent = a;
		form = {
			name: a.name,
			dns_enabled: a.dns_enabled,
			dns_name: a.dns_name,
			trust_reported_ip: a.trust_reported_ip,
			reach_port: a.reach_port
		};
		main = a.pinned_ip;
		draft.mark();
	}
	async function load() {
		const [l, c] = await Promise.all([
			api.GET('/api/pulse/agents'),
			api.GET('/api/pulse/agents/{id}/consumers', { params: { path: { id } } })
		]);
		if (!l.data) throw l.error;
		const a = l.data.agents.find((x) => x.id === id);
		if (!a) {
			missing = true;
			return;
		}
		show(a);
		uses = c.data ?? null;
	}
	onMount(() => {
		load().catch((e) => (error = problemText(e)));
	});

	async function run(fn: () => Promise<Agent | undefined>, done: string) {
		error = notice = '';
		busy = true;
		try {
			const a = await fn();
			if (a) show(a);
			notice = done;
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const save = () =>
		run(async () => {
			const r = await api.PUT('/api/pulse/agents/{id}', {
				params: { path: { id } },
				body: { ...form, name: form.name.trim() }
			});
			if (r.error) throw r.error;
			return r.data;
		}, 'Saved.');
	const pin = (address: string) =>
		run(
			async () => {
				const r = await api.PUT('/api/pulse/agents/{id}/main-address', {
					params: { path: { id } },
					body: { address }
				});
				if (r.error) throw r.error;
				return r.data;
			},
			address ? `${address} is the main address now.` : 'The agent chooses its main address again.'
		);
	const health = { fresh: 'up', stale: 'degraded', revoked: 'down', waiting: 'unknown' } as const;
	// The pinned address went away: the agent chose another until it comes back.
	const pinGone = $derived(
		!!agent?.pinned_ip && !agent.addresses.some((x) => x.address === agent!.pinned_ip)
	);
</script>

<svelte:head><title>{agent?.name ?? 'Pulse agent'} · Hashi</title></svelte:head>

<a
	href="/pulse"
	class="mb-3 inline-flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Pulse</a
>
{#if error}<p role="alert" class="mb-4 text-md text-bad-text">{error}</p>{/if}
{#if missing}
	<p class="text-md text-muted-foreground">No such agent.</p>
{:else if !agent}
	<Skeleton class="h-10 w-64" /><Skeleton class="mt-4 h-64 rounded-2xl" />
{:else}
	<div class="mb-5 flex flex-wrap items-end gap-4">
		<div class="min-w-0 flex-1">
			<h1 class="flex items-center gap-2 text-title font-semibold">
				{agent.name}<StatusBadge state={health[agent.status]} />
			</h1>
			<p class="text-md text-muted-foreground">
				{[
					agent.hostname,
					agent.version,
					agent.last_seen_at && `seen ${dateTime(agent.last_seen_at)}`
				]
					.filter(Boolean)
					.join(' · ') || 'No heartbeat yet'}
			</p>
		</div>
		<Button variant="outline" size="sm" href="/connections">Token and install</Button>
	</div>
	{#if notice}<p role="status" class="mb-4 text-md text-ok-text">{notice}</p>{/if}

	<div class="grid gap-4 lg:grid-cols-2">
		<Panel title="Main address">
			{#if agent.addresses.length === 0}
				<p class="text-md text-muted-foreground">No address reported yet.</p>
			{:else}
				<RadioGroup.Root
					value={main}
					onValueChange={(v) => {
						main = v;
						void pin(v);
					}}
					class="gap-2 text-md"
					aria-label="Main address"
					disabled={busy}
				>
					<label class="flex items-center gap-3">
						<RadioGroup.Item value="" />
						<span>Let the agent choose</span>
						{#if !agent.pinned_ip}<Mono class="text-muted-foreground">{agent.selected_ip}</Mono
							>{/if}
					</label>
					{#each agent.addresses as a (a.address)}
						<label class="flex items-center gap-3">
							<RadioGroup.Item value={a.address} />
							<Mono>{a.address}</Mono>
							<span class="text-muted-foreground">{a.interface}</span>
							{#if a.address === agent.selected_ip}<Tag tone="info">in use</Tag>{/if}
						</label>
					{/each}
				</RadioGroup.Root>
				{#if pinGone}<p class="mt-3 text-sm text-warn-text">
						The agent no longer reports <Mono>{agent.pinned_ip}</Mono>, so it uses <Mono
							>{agent.selected_ip}</Mono
						> until it does.
					</p>{/if}
			{/if}
			<dl class="mt-4 grid grid-cols-[9rem_minmax(0,1fr)] gap-y-1.5 text-md">
				<dt class="text-muted-foreground">Public address</dt>
				<dd>
					<Mono>{agent.public_ip || '—'}</Mono>{#if agent.public_ip_source === 'reported'}
						<Tag>self-reported</Tag>{/if}
				</dd>
				{#if agent.reach_port}
					<dt class="text-muted-foreground">From Hashi</dt>
					<dd>
						{agent.reachable === true
							? `port ${agent.reach_port} answers`
							: agent.reachable === false
								? `port ${agent.reach_port} does not answer`
								: 'not checked yet'}
					</dd>
				{/if}
			</dl>
		</Panel>

		<Panel title="Settings">
			<div class="space-y-4 text-md">
				<div class="space-y-1.5">
					<Label for="pa-name">Name</Label><Input id="pa-name" bind:value={form.name} />
				</div>
				<label class="flex items-center justify-between gap-3">
					<span>Give it an internal DNS name</span><Switch
						bind:checked={form.dns_enabled}
						aria-label="Give it an internal DNS name"
					/>
				</label>
				{#if form.dns_enabled}
					<div class="space-y-1.5">
						<Label for="pa-dns">DNS name</Label><Input
							id="pa-dns"
							bind:value={form.dns_name}
							placeholder="Uses the agent's name"
						/>
						<p class="text-xs text-muted-foreground">
							Under the <a href="/settings?category=dns" class="underline">internal domain</a>.
						</p>
					</div>
				{/if}
				<div class="space-y-1.5">
					<Label for="pa-reach">Port to test from Hashi</Label><Input
						id="pa-reach"
						type="number"
						min="0"
						max="65535"
						bind:value={form.reach_port}
						class="max-w-32"
					/>
				</div>
				<label class="flex items-center justify-between gap-3">
					<span>Trust its own report of its public address</span><Switch
						bind:checked={form.trust_reported_ip}
						aria-label="Trust its own report of its public address"
					/>
				</label>
				{#if form.trust_reported_ip}<p class="text-xs text-warn-text">
						Only for a machine that reaches Hashi over a private path. With its token, someone could
						name any public address here.
					</p>{/if}
				<div class="flex justify-end">
					<Button size="sm" onclick={save} disabled={busy || !form.name.trim() || !draft.dirty}
						>Save</Button
					>
				</div>
			</div>
		</Panel>

		<Panel title="Used by">
			{#if uses && (uses.set_refs.length || uses.direct.length)}
				<ul class="space-y-1 text-md">
					{#each uses.direct as c (c.type + c.id)}
						{@const l = consumerOf(c)}
						<li>
							<span class="text-muted-foreground">{l.kind}</span>
							{#if l.href}<a href={l.href} class="hover:underline">{c.name}</a>{:else}{c.name}{/if}
						</li>
					{/each}
					{#each uses.set_refs as s (s.id)}
						<li>
							<span class="text-muted-foreground">IP set</span>
							<a href="/ip-sets/{s.id}" class="hover:underline">{s.name}</a>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="text-md text-muted-foreground">Nothing follows this agent yet.</p>
			{/if}
		</Panel>
	</div>
{/if}
