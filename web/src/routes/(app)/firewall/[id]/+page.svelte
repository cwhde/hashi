<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import SetBadge from '$lib/components/hashi/ipsets/SetBadge.svelte';
	import IpSetPicker from '$lib/components/hashi/ipsets/IpSetPicker.svelte';
	import HostSettings from '$lib/components/hashi/firewall/HostSettings.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import * as Tabs from '$lib/components/ui/tabs';
	import { ChevronLeft, LifeBuoy, X } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { listSets, type IPSet } from '$lib/ipsets';
	import {
		firewallInput,
		healthState,
		lines,
		type FirewallHost,
		type FirewallInput
	} from '$lib/firewall';
	import { saveText } from '$lib/services';

	const id = $derived(page.params.id as string);
	const path = $derived({ params: { path: { id } } });
	let host = $state<FirewallHost | null>(null);
	let others = $state<FirewallHost[]>([]);
	let rendered = $state<Schemas['FirewallRendered'] | null>(null);
	let ruleset = $state<Schemas['FirewallRuleset'] | null>(null);
	let sets = $state<IPSet[]>([]);
	let access = $state<string[]>([]);
	let pick = $state('');
	let entry = $state('');
	let tk = $state({ chains: '', rules: '', ipsets: '', cron_match: '', cron_files: '' });
	let busy = $state(false);
	let note = $state('');
	let error = $state('');
	const setOf = (s: string) => sets.find((x) => x.id === s);

	async function act(fn: () => Promise<void>) {
		error = note = '';
		busy = true;
		try {
			await fn();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const planned = (plans: Schemas['SyncPlan'][]) =>
		(note = saveText({ plans, certificate_pending: false }).text);

	async function load() {
		const [h, l, a, t] = await Promise.all([
			api.GET('/api/firewall/hosts/{id}', path),
			api.GET('/api/firewall/hosts'),
			api.GET('/api/firewall/hosts/{id}/host-access', path),
			api.GET('/api/firewall/hosts/{id}/takeover', path)
		]);
		if (h.error || !h.data) throw h.error;
		host = h.data;
		entry = h.data.route_via_host_id;
		others = (l.data?.hosts ?? []).filter((x) => x.id !== id);
		access = a.data?.sets ?? [];
		if (t.data)
			tk = {
				chains: t.data.chains.join('\n'),
				rules: t.data.rules.join('\n'),
				ipsets: t.data.ipsets.join('\n'),
				cron_match: t.data.cron_match.join('\n'),
				cron_files: t.data.cron_files.join('\n')
			};
		sets = await listSets();
		if (h.data.edge_mode === 'managed' && h.data.last_validated_at) {
			const r = await api.GET('/api/firewall/hosts/{id}/rendered', path);
			rendered = r.data ?? null;
		}
	}
	onMount(() => act(load));

	const save = (b: FirewallInput) =>
		act(async () => {
			const r = await api.PUT('/api/firewall/hosts/{id}', { ...path, body: b });
			if (r.error || !r.data) throw r.error;
			host = r.data;
			note = 'Saved.';
		});
	const validate = () =>
		act(async () => {
			const r = await api.POST('/api/firewall/hosts/{id}/validate', path);
			if (r.error) throw r.error;
			await load();
			note = 'Read from the host. Nothing changed there.';
		});
	const confirm = () =>
		act(async () => {
			const r = await api.POST('/api/firewall/hosts/{id}/confirm', path);
			if (r.error) throw r.error;
			await load();
		});
	const plan = () =>
		act(async () => {
			const r = await api.POST('/api/firewall/hosts/{id}/plan', path);
			if (r.error || !r.data) throw r.error;
			note = r.data.plan ? `Plan: ${r.data.plan.status}.` : 'Nothing to change.';
		});
	const readRuleset = () =>
		act(async () => {
			const r = await api.GET('/api/firewall/hosts/{id}/ruleset', path);
			if (r.error || !r.data) throw r.error;
			ruleset = r.data;
		});
	const putAccess = (next: string[]) =>
		act(async () => {
			const r = await withReauth(() =>
				api.PUT('/api/firewall/hosts/{id}/host-access', { ...path, body: { sets: next } })
			);
			access = next;
			planned(r.plans);
		});
	const putTakeover = () =>
		act(async () => {
			const body = {
				chains: lines(tk.chains),
				rules: tk.rules
					.split('\n')
					.map((x) => x.trim())
					.filter(Boolean),
				ipsets: lines(tk.ipsets),
				cron_match: tk.cron_match
					.split('\n')
					.map((x) => x.trim())
					.filter(Boolean),
				cron_files: lines(tk.cron_files)
			};
			const r = await withReauth(() =>
				api.PUT('/api/firewall/hosts/{id}/takeover', { ...path, body })
			);
			planned(r.plans);
		});
	const del = () =>
		act(async () => {
			if (
				!window.confirm(
					`Delete ${host?.name}? Its rules stay on the machine until you remove them.`
				)
			)
				return;
			const r = await api.DELETE('/api/firewall/hosts/{id}', path);
			if (r.error) throw r.error;
			await goto('/firewall');
		});
	const saveEntry = () => host && save({ ...firewallInput(host), route_via_host_id: entry });
	const name = $derived(host?.name ?? '');
	const entryName = $derived(others.find((o) => o.id === entry)?.name ?? name);
</script>

<a
	href="/firewall"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Firewall hosts</a
>
{#if error}<p
		role="alert"
		class="mb-4 rounded-2xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
	>
		{error}
	</p>{/if}
{#if note}<p role="status" class="mb-4 rounded-2xl border bg-card p-3 text-[13px]">
		{note} <a class="underline" href="/changes">Open changes</a>
	</p>{/if}
{#if host}
	<div class="mb-5 flex flex-wrap items-end gap-4">
		<div class="flex-1">
			<h1 class="flex items-center gap-2 text-[22px] font-semibold">
				{host.name}<StatusBadge state={healthState(host)} />
			</h1>
			<p class="text-[13.5px] text-muted-foreground">
				{host.facts?.os ?? 'Not read yet'}{#if host.facts}
					· nft {host.facts.nft_version}{/if} · <Mono>{host.public_ip}</Mono>
			</p>
		</div>
		{#if host.edge_mode === 'managed'}
			<Button variant="outline" size="sm" onclick={validate} disabled={busy}>Validate</Button>
			{#if host.ownership_confirmed}<Button size="sm" onclick={plan} disabled={busy}
					>Plan firewall</Button
				>
			{:else}<Button size="sm" onclick={confirm} disabled={busy || !host.last_validated_at}
					>Let Hashi manage it</Button
				>{/if}
		{/if}
		<Button variant="ghost" size="sm" onclick={del} disabled={busy}>Delete</Button>
	</div>
	{#if host.edge_mode === 'managed'}
		<Panel title="Rescue path" class="mb-4">
			<div class="flex items-start gap-3 text-[13px]">
				<LifeBuoy class="mt-0.5 size-5 shrink-0 text-ok-text" />
				<p>
					Every high-risk apply arms a 180 s timer that restores Hashi’s table unless a <em>new</em> SSH
					login and NetBird check succeed.
				</p>
			</div>
		</Panel>
	{/if}
	<Tabs.Root value="routing">
		<Tabs.List class="mb-4">
			<Tabs.Trigger value="routing">Routing</Tabs.Trigger>
			{#if host.edge_mode === 'managed'}
				<Tabs.Trigger value="ports">Ports</Tabs.Trigger>
				<Tabs.Trigger value="access">Host access</Tabs.Trigger>
				<Tabs.Trigger value="rules" onclick={() => !ruleset && readRuleset()}
					>Live ruleset</Tabs.Trigger
				>
				<Tabs.Trigger value="generated">Generated</Tabs.Trigger>
				<Tabs.Trigger value="script">Old script</Tabs.Trigger>
			{/if}
		</Tabs.List>
		<Tabs.Content value="routing" class="grid gap-4 lg:grid-cols-2">
			<Panel title="Settings">
				{#if host.edge_mode === 'managed'}{#key host.id}<HostSettings {host} {busy} {save} />{/key}
				{:else}<p class="text-[13px]">
						Hashi does not touch this host's firewall. Forward the public ports on your router to <Mono
							>{host.internal_traefik_ip}</Mono
						>.
					</p>{/if}
			</Panel>
			<Panel
				title="Public entry"
				description="Where traffic for services on {name} enters. Changes one DNS record."
			>
				<Label for="entry">Enters through</Label>
				<select
					id="entry"
					bind:value={entry}
					class="mt-1.5 h-10 w-full rounded-[10px] border bg-transparent px-3 text-[13px]"
				>
					<option value="">{name} itself</option>
					{#each others as o (o.id)}<option value={o.id}>{o.name}</option>{/each}
				</select>
				<Button
					class="mt-3"
					size="sm"
					variant="outline"
					onclick={saveEntry}
					disabled={busy || entry === host.route_via_host_id}>Save</Button
				>
				<div class="mt-3 space-y-1 text-[12.5px]">
					<div><Mono>on.{name}</Mono> → <Mono>via.{entryName}</Mono></div>
					<div><Mono>via.{name}</Mono> → <Mono>{name}</Mono></div>
					<div><Mono>{name}</Mono> A <Mono>{host.public_ip}</Mono></div>
				</div>
			</Panel>
		</Tabs.Content>
		<Tabs.Content value="ports">
			<Panel
				flush
				title="Public ports"
				description="From services. Opening or closing one is a high-risk change."
			>
				<ul class="divide-y text-[13px]">
					{#each rendered?.ports ?? [] as p (p)}
						<li class="flex items-center gap-3 px-4 py-2.5">
							<Mono class="flex-1">{p}</Mono><Tag tone="ok"
								>forwarded to {host.internal_traefik_ip}</Tag
							>
						</li>
					{:else}
						<li class="px-4 py-3 text-muted-foreground">
							No public ports yet: they follow your services.
						</li>
					{/each}
				</ul>
			</Panel>
		</Tabs.Content>
		<Tabs.Content value="access">
			<Panel
				title="Who may reach this host itself"
				description="SSH and the Proxmox UI on the host. Loopback, its own subnets and NetBird always can."
			>
				<ul class="space-y-2 text-[13px]" aria-label="Host access">
					{#each access as s (s)}
						<li class="flex items-center gap-2">
							{#if setOf(s)}<SetBadge set={setOf(s)!} />{:else}<Mono>{s}</Mono>{/if}
							<Button
								variant="ghost"
								size="icon-xs"
								aria-label="Remove"
								onclick={() => putAccess(access.filter((x) => x !== s))}><X /></Button
							>
						</li>
					{/each}
				</ul>
				<div class="mt-3 flex max-w-md gap-2">
					<IpSetPicker bind:value={pick} {sets} exclude={access} id="pick-access" />
					<Button
						size="sm"
						variant="outline"
						disabled={!pick}
						onclick={async () => {
							await putAccess([...access, pick]);
							pick = '';
						}}>Add</Button
					>
				</div>
			</Panel>
		</Tabs.Content>
		<Tabs.Content value="rules">
			<Panel
				flush
				title="Observed ruleset"
				description="Every table on the host, read just now. The rest is left alone."
			>
				{#snippet actions()}<Button
						size="sm"
						variant="outline"
						onclick={readRuleset}
						disabled={busy}>Read again</Button
					>{/snippet}
				<pre
					class="max-h-[70vh] overflow-auto p-4 font-mono text-[12.5px] leading-relaxed">{ruleset?.now ??
						'Reading…'}</pre>
			</Panel>
		</Tabs.Content>
		<Tabs.Content value="generated">
			<Panel
				flush
				title="Hashi's table"
				description="What the next apply loads, as one transaction. Read-only."
			>
				<pre
					class="max-h-[70vh] overflow-auto p-4 font-mono text-[12.5px] leading-relaxed text-muted-foreground">{rendered?.table ??
						'Validate the host first.'}</pre>
			</Panel>
		</Tabs.Content>
		<Tabs.Content value="script">
			<Panel
				title="Old firewall script"
				description="What the next plan removes, listed exactly. Rules of NetBird, Docker and Proxmox are refused. If the apply is not confirmed, everything comes back."
			>
				<div class="grid gap-3 md:grid-cols-2">
					{#each [{ k: 'chains', l: 'Its chains' }, { k: 'ipsets', l: 'Its ipsets' }, { k: 'rules', l: 'Its rules in built-in chains ("filter -A FORWARD …")' }, { k: 'cron_match', l: 'Cron lines containing' }, { k: 'cron_files', l: 'Files the crontab is installed from' }] as f (f.k)}
						{@const key = f.k as keyof typeof tk}
						<div class="space-y-1.5" class:md:col-span-2={f.k === 'rules'}>
							<Label for="tk-{f.k}">{f.l}</Label>
							<textarea
								id="tk-{f.k}"
								bind:value={tk[key]}
								rows="4"
								class="w-full rounded-[10px] border bg-transparent px-3 py-2 font-mono text-[12.5px]"
							></textarea>
						</div>
					{/each}
				</div>
				<Button class="mt-3" onclick={putTakeover} disabled={busy}>Save and plan</Button>
			</Panel>
		</Tabs.Content>
	</Tabs.Root>
{/if}
