<script lang="ts">
	// 10b §2, 10c §1 (bug 24): every outside system Hashi talks to is added and reached from here. What belongs to one
	// host or agent (ports, host access, its main address) stays on that object's page; each name links there.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import StatusBadge from '#lib/components/hashi/StatusBadge.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import AdGuardSheet from '#lib/components/hashi/connections/AdGuardSheet.svelte';
	import TraefikHostSheet from '#lib/components/hashi/connections/TraefikHostSheet.svelte';
	import FirewallHostSheet from '#lib/components/hashi/connections/FirewallHostSheet.svelte';
	import PulseAgentSheet from '#lib/components/hashi/connections/PulseAgentSheet.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.ts';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.ts';
	import { ChevronDown, MoreHorizontal, Plus } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { pulseAgents } from '#lib/pulseAgents.svelte.ts';
	import { firewallHosts } from '#lib/firewallHosts.svelte.ts';
	import { dateTime } from '#lib/format.ts';
	import type { FirewallHost } from '#lib/firewall.ts';
	import type { Health } from '#lib/design/mock.ts';

	type Traefik = Schemas['TraefikConnection'];
	type AdGuard = Schemas['AdGuardConnection'];
	type Agent = Schemas['PulseAgent'];
	let zones = $state<Schemas['DNSZone'][]>([]);
	let traefik = $state<Traefik[]>([]);
	let firewall = $state<FirewallHost[]>([]);
	let agents = $state<Agent[]>([]);
	let adguard = $state<AdGuard[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let busy = $state(false);

	async function load() {
		const [z, t, f, p, a] = await Promise.all([
			api.GET('/api/dns/zones'),
			api.GET('/api/traefik/connections'),
			api.GET('/api/firewall/hosts'),
			api.GET('/api/pulse/agents'),
			api.GET('/api/adguard/connections')
		]);
		if (!z.data || !t.data || !f.data || !p.data || !a.data)
			throw z.error ?? t.error ?? f.error ?? p.error ?? a.error;
		zones = z.data.zones;
		traefik = t.data.connections;
		firewall = f.data.hosts;
		agents = p.data.agents;
		adguard = a.data.connections;
		// The shared lists other pages pick from follow what changed here.
		pulseAgents.list = agents;
		firewallHosts.list = firewall.map((h) => ({ id: h.id, name: h.name }));
		loaded = true;
	}
	const reload = () => load().catch((e) => (error = problemText(e)));
	async function act(fn: () => Promise<unknown>) {
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

	// ---- drawers; ?add=<kind> opens one, so feature pages can link here ----
	type Kind = 'traefik' | 'firewall' | 'pulse' | 'adguard';
	let traefikOpen = $state(false);
	let traefikConn = $state<Traefik | null>(null);
	let firewallOpen = $state(false);
	let firewallHost = $state<FirewallHost | null>(null);
	let pulseOpen = $state(false);
	let pulseShown = $state<{ agent: Agent; token: string } | null>(null);
	let adguardOpen = $state(false);
	let adguardConn = $state<AdGuard | null>(null);
	function add(k: Kind) {
		if (k === 'traefik') [traefikConn, traefikOpen] = [null, true];
		if (k === 'firewall') [firewallHost, firewallOpen] = [null, true];
		if (k === 'pulse') [pulseShown, pulseOpen] = [null, true];
		if (k === 'adguard') [adguardConn, adguardOpen] = [null, true];
	}
	onMount(() => {
		reload();
		const k = page.url.searchParams.get('add');
		if (k && ['traefik', 'firewall', 'pulse', 'adguard'].includes(k)) {
			add(k as Kind);
			void goto('/connections', { replace: true, reset: false });
		}
	});

	// ---- rows ----
	// A destructive action asks first and names what goes (10b §3, 10c §5).
	type Confirm = { title: string; body: string; verb: string };
	type Action = { label: string; run: () => void; danger?: boolean; confirm?: Confirm };
	let asking = $state<Action | null>(null);
	const choose = (a: Action) => (a.confirm ? (asking = a) : a.run());
	type Row = {
		key: string;
		name: string;
		href?: string;
		detail: string;
		bad?: boolean;
		checked?: string | null;
		health: Health;
		tags?: string[];
		actions: Action[];
	};
	const hostHealth = (h: string): Health =>
		h === 'healthy' ? 'up' : h === 'degraded' ? 'degraded' : h === 'failed' ? 'down' : 'unknown';
	const agentHealth = {
		fresh: 'up',
		stale: 'degraded',
		revoked: 'down',
		waiting: 'unknown'
	} as const;
	const adguardMode = { read_only: 'read-only', active: 'active', disabled: 'disabled' } as const;
	// Hashi needs one DNS provider, one Traefik host and one firewall host (10b §2).
	const required = (n: number) => (n === 1 ? ['required'] : []);

	const sections = $derived<{ title: string; empty: string; kind?: Kind; rows: Row[] }[]>([
		{
			title: 'DNS provider',
			empty: 'Connected during setup.',
			rows: zones.map((z) => ({
				key: z.id,
				name: z.provider.type === 'hetzner' ? 'Hetzner DNS' : z.provider.type,
				href: '/dns',
				detail: z.last_refresh_error || `zone ${z.name}, ${z.rrset_count} records`,
				bad: !!z.last_refresh_error,
				checked: z.last_refreshed_at,
				health: z.last_refresh_error ? 'down' : 'up',
				tags: required(zones.length),
				actions: []
			}))
		},
		{
			title: 'Traefik hosts',
			kind: 'traefik',
			empty: 'No Traefik host yet.',
			rows: traefik.map((c) => ({
				key: c.id,
				name: c.name,
				href: '/traefik',
				detail: c.last_error || `${c.user}@${c.host}:${c.port}`,
				bad: !!c.last_error && c.health === 'failed',
				checked: c.last_validated_at,
				health: hostHealth(c.health),
				tags: required(traefik.length),
				actions: [
					{ label: 'Edit', run: () => ([traefikConn, traefikOpen] = [c, true]) },
					{
						label: 'Remove',
						danger: true,
						confirm: {
							title: `Remove ${c.name}?`,
							body: 'Hashi stops managing this Traefik host. Nothing on the host changes: its services, files and certificates stay.',
							verb: 'Remove host'
						},
						run: () =>
							act(() =>
								withReauth(() =>
									api.DELETE('/api/traefik/hosts/{id}', { params: { path: { id: c.id } } })
								)
							)
					}
				]
			}))
		},
		{
			title: 'Firewall hosts',
			kind: 'firewall',
			empty: 'No firewall host yet.',
			rows: firewall.map((h) => ({
				key: h.id,
				name: h.name,
				href: `/firewall/${h.id}`,
				detail:
					h.edge_mode === 'managed'
						? h.last_error || `${h.user}@${h.host}:${h.port}`
						: `behind a router${h.address_agent_id ? `, address from ${pulseAgents.list.find((a) => a.id === h.address_agent_id)?.name ?? 'a Pulse agent'}` : ''}`,
				bad: h.health === 'failed',
				checked: h.last_validated_at,
				health: h.edge_mode === 'managed' ? hostHealth(h.health) : 'unknown',
				tags: required(firewall.length),
				actions: [{ label: 'Edit', run: () => ([firewallHost, firewallOpen] = [h, true]) }]
			}))
		},
		{
			title: 'Pulse agents',
			kind: 'pulse',
			empty: 'No agent yet. Add one to a machine whose address can change.',
			rows: agents.map((a) => ({
				key: a.id,
				name: a.name,
				href: `/pulse/${a.id}`,
				detail:
					[a.selected_ip, a.hostname, a.version].filter(Boolean).join(' · ') || 'no heartbeat yet',
				checked: a.last_seen_at,
				health: agentHealth[a.status],
				actions: [
					{
						label: 'New token',
						run: () =>
							act(async () => {
								pulseShown = await withReauth(() =>
									api.POST('/api/pulse/agents/{id}/rotate-token', {
										params: { path: { id: a.id } }
									})
								);
								pulseOpen = true;
							})
					},
					...(a.status !== 'revoked'
						? [
								{
									label: 'Revoke',
									danger: true,
									confirm: {
										title: `Revoke ${a.name}?`,
										body: 'Its token stops working, and whatever follows this agent resolves to nothing until it has a new token.',
										verb: 'Revoke agent'
									},
									run: () =>
										act(() =>
											withReauth(() =>
												api.POST('/api/pulse/agents/{id}/revoke', {
													params: { path: { id: a.id } }
												})
											)
										)
								}
							]
						: []),
					{
						label: 'Remove',
						danger: true,
						confirm: {
							title: `Remove ${a.name}?`,
							body: 'The agent and its addresses are deleted; this cannot be undone. Hashi refuses while something still uses it.',
							verb: 'Remove agent'
						},
						run: () =>
							act(() =>
								withReauth(() =>
									api.DELETE('/api/pulse/agents/{id}', { params: { path: { id: a.id } } })
								)
							)
					}
				]
			}))
		},
		{
			title: 'AdGuard Home',
			kind: 'adguard',
			empty: 'No AdGuard instance yet. A new one starts read-only, so adding it changes nothing.',
			rows: adguard.map((c) => ({
				key: c.id,
				name: c.name,
				href: `/dns?tab=adguard&instance=${c.id}`,
				detail: c.last_error || c.last_version || 'not checked yet',
				bad: !!c.last_error,
				checked: c.last_checked_at,
				health:
					c.mode === 'disabled'
						? 'unknown'
						: c.last_error
							? 'down'
							: c.last_version
								? 'up'
								: 'unknown',
				tags: [adguardMode[c.mode]],
				actions: [
					{
						label: 'Test',
						run: () =>
							act(() =>
								api.POST('/api/adguard/connections/{id}/test', { params: { path: { id: c.id } } })
							)
					},
					{ label: 'Edit', run: () => ([adguardConn, adguardOpen] = [c, true]) },
					{
						label: 'Remove',
						danger: true,
						confirm: {
							title: `Remove ${c.name}?`,
							body: 'Hashi forgets this AdGuard instance. Its rewrites stay on the instance.',
							verb: 'Remove connection'
						},
						run: () =>
							act(() =>
								withReauth(() =>
									api.DELETE('/api/adguard/connections/{id}', { params: { path: { id: c.id } } })
								)
							)
					}
				]
			}))
		}
	]);
	const kinds: [Kind, string][] = [
		['traefik', 'Traefik host'],
		['firewall', 'Firewall host'],
		['pulse', 'Pulse agent'],
		['adguard', 'AdGuard Home']
	];
</script>

<svelte:head><title>Connections · Hashi</title></svelte:head>

<PageHeader title="Connections">
	{#snippet actions()}
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}<Button {...props}><Plus />Add connection<ChevronDown /></Button
					>{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content align="end">
				{#each kinds as [k, label] (k)}<DropdownMenu.Item onclick={() => add(k)}
						>{label}</DropdownMenu.Item
					>{/each}
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	{/snippet}
</PageHeader>

{#if error}<p role="alert" class="mb-3 text-md text-bad-text">{error}</p>{/if}
{#if !loaded}
	<Skeleton class="h-80 rounded-2xl" />
{:else}
	<div class="divide-y rounded-2xl border bg-card">
		{#each sections as s (s.title)}
			<section class="px-4 py-3" aria-label={s.title}>
				<h2 class="mb-1 text-sm font-medium text-muted-foreground">{s.title}</h2>
				{#if s.rows.length === 0}
					<p class="py-2 text-md text-muted-foreground">
						{s.empty}
						{#if s.kind}<button
								class="font-medium text-foreground underline underline-offset-2"
								onclick={() => add(s.kind!)}>Add one</button
							>{/if}
					</p>
				{/if}
				<ul>
					{#each s.rows as r (r.key)}
						<li class="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-md">
							{#if r.href}<a href={r.href} class="min-w-40 font-medium hover:underline">{r.name}</a
								>{:else}<span class="min-w-40 font-medium">{r.name}</span>{/if}
							<span
								class="min-w-0 flex-1 basis-48 truncate {r.bad
									? 'text-bad-text'
									: 'text-muted-foreground'}">{r.detail}</span
							>
							{#each r.tags ?? [] as t (t)}<Tag>{t}</Tag>{/each}
							<span class="hidden w-40 text-right text-sm text-muted-foreground md:inline"
								>{r.checked ? dateTime(r.checked) : ''}</span
							>
							<span class="w-24"><StatusBadge state={r.health} /></span>
							{#if r.actions.length}
								<DropdownMenu.Root>
									<DropdownMenu.Trigger>
										{#snippet child({ props })}<Button
												{...props}
												variant="ghost"
												size="icon-sm"
												aria-label="Actions for {r.name}"
												disabled={busy}><MoreHorizontal /></Button
											>{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end">
										{#each r.actions as a (a.label)}<DropdownMenu.Item
												onclick={() => choose(a)}
												variant={a.danger ? 'destructive' : 'default'}>{a.label}</DropdownMenu.Item
											>{/each}
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							{:else}<span class="size-8"></span>{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>
{/if}

<TraefikHostSheet bind:open={traefikOpen} conn={traefikConn} onsaved={reload} />
<FirewallHostSheet bind:open={firewallOpen} host={firewallHost} onsaved={reload} />
<PulseAgentSheet bind:open={pulseOpen} bind:shown={pulseShown} onsaved={reload} />
<AdGuardSheet
	bind:open={adguardOpen}
	conn={adguardConn}
	onsaved={(id) =>
		act(() => api.POST('/api/adguard/connections/{id}/test', { params: { path: { id } } }))}
/>

<AlertDialog.Root open={!!asking} onOpenChange={(o) => !o && (asking = null)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{asking?.confirm?.title}</AlertDialog.Title>
			<AlertDialog.Description>{asking?.confirm?.body}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action
				onclick={() => {
					const a = asking;
					asking = null;
					a?.run();
				}}>{asking?.confirm?.verb}</AlertDialog.Action
			>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
