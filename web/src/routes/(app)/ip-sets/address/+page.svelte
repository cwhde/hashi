<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import SetBadge from '#lib/components/hashi/ipsets/SetBadge.svelte';
	import IpSetPicker from '#lib/components/hashi/ipsets/IpSetPicker.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import { ChevronLeft, CircleAlert, Search } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { addressHref, listSets, type IPSet } from '#lib/ipsets.ts';
	import { country, dateTime } from '#lib/format.ts';
	import { eventLabel, sourceLabel } from '#lib/components/hashi/security/labels.ts';

	// Everything Hashi knows about one address (05a §6, bug 135): GeoIP country and AS, set membership with the path
	// through nesting, the firewall verdict per host, the abuse engine's counters and state, and the latest events.
	const address = $derived(page.url.searchParams.get('ip') ?? '');
	let lookup = $state<Schemas['AddressLookup'] | null>(null);
	let sets = $state<IPSet[]>([]);
	let loadError = $state('');
	let outcome = $state('');
	let jump = $state('');

	async function load(a: string) {
		lookup = null;
		loadError = '';
		if (!a) return;
		const [r, all] = await Promise.all([
			api.GET('/api/addresses/{address}', { params: { path: { address: a } } }),
			listSets()
		]);
		if (r.error || !r.data) throw r.error;
		lookup = r.data;
		sets = all;
	}
	$effect(() => {
		const a = address;
		jump = a;
		load(a).catch((e) => (loadError = problemText(e)));
	});

	// 05 §4.2: allowed passes the firewall, blocked is dropped, neutral leaves it to the services' own rules.
	const verdictText = {
		allowed: 'allowed',
		blocked: 'blocked',
		neutral: 'no firewall decision'
	} as const;
	const verdictTone = { allowed: 'ok', blocked: 'bad', neutral: 'neutral' } as const;
	const memberOf = $derived(lookup?.sets.filter((m) => m.set.system_key !== 'all') ?? []);
	const leaf = (m: Schemas['AddressMatch']) => [...m.path, m.label].join(' › ');

	// ---- ban, challenge, lift (bug 64): the engine's own entries, on every firewall host; an allowed address is refused ----
	let busy = $state(false);
	const target64 = () => lookup?.address ?? address;
	async function enter(set: 'sys-dynamic-blocked' | 'sys-dynamic-challenge') {
		busy = true;
		const a = target64();
		const r = await api.POST('/api/ip-sets/{id}/entries', {
			params: { path: { id: set } },
			body: { address: a }
		});
		busy = false;
		outcome = r.error
			? problemText(r.error)
			: set === 'sys-dynamic-blocked'
				? `${a} is banned.`
				: `${a} gets the challenge page now.`;
		if (!r.error) await load(a).catch(() => {});
	}
	async function lift() {
		busy = true;
		const a = target64();
		const was = lookup?.counter?.state;
		const r = await api.DELETE('/api/ip-sets/{id}/entries/{address}', {
			params: { path: { id: 'sys-dynamic-blocked', address: a } }
		});
		busy = false;
		outcome = r.error
			? problemText(r.error)
			: was === 'banned'
				? `The ban on ${a} is lifted.`
				: `The challenge for ${a} has ended.`;
		if (!r.error) await load(a).catch(() => {});
	}
	const sev = { info: 'ok', warning: 'warn', critical: 'bad' } as const;
	const counter = $derived(lookup?.counter);
	const stateDetail = $derived.by(() => {
		const c = counter;
		if (!c || c.state === 'none') return '';
		if (c.state === 'banned') return c.until ? `until ${dateTime(c.until)}` : 'with no end';
		return `${c.failures} of ${c.max_failures} unsolved${c.until ? `, ends ${dateTime(c.until)}` : ''}`;
	});

	// ---- add to a set ----
	let addOpen = $state(false);
	let target = $state('');
	let note = $state('');
	let addError = $state('');
	async function addToSet() {
		addError = '';
		const r = await api.POST('/api/ip-sets/{id}/elements', {
			params: { path: { id: target } },
			body: { item: { type: 'ip', address: lookup?.address ?? address }, note }
		});
		if (r.error) {
			addError = problemText(r.error);
			return;
		}
		addOpen = false;
		outcome = `Added to ${sets.find((s) => s.id === target)?.slug}.`;
		await load(address);
	}
</script>

<svelte:head><title>{lookup?.address ?? 'Address lookup'} · Hashi</title></svelte:head>

<a
	href="/ip-sets"
	class="mb-3 inline-flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />IP sets</a
>

{#if !address}<h1 class="mb-4 text-title leading-tight font-semibold">Address lookup</h1>{/if}

<form
	class="relative mb-5 max-w-80"
	onsubmit={(e) => {
		e.preventDefault();
		// An empty search goes back to the empty page, which also clears an old error (bug 67).
		goto(jump.trim() ? addressHref(jump.trim()) : '/ip-sets/address');
	}}
>
	<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
	<Input
		bind:value={jump}
		placeholder="Look up an address"
		class="pl-8 font-mono"
		aria-label="Address"
	/>
</form>

{#if address && loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-md text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if address && !lookup}
	<Skeleton class="h-60 rounded-2xl" />
{:else if address && lookup}
	<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
		<div class="min-w-64 flex-1">
			<h1 class="font-mono text-title leading-tight font-medium tracking-[-0.02em]">
				{lookup.address}
			</h1>
			<p
				class="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-md text-muted-foreground"
				aria-label="GeoIP"
			>
				{#if lookup.country}<span title={lookup.country}>{country(lookup.country)}</span>{/if}
				{#if lookup.asn}<span
						><Mono>AS{lookup.asn}</Mono>{#if lookup.as_name}
							<span class="text-foreground">{lookup.as_name}</span>{/if}</span
					>{/if}
				{#if lookup.network}<span>network <Mono>{lookup.network}</Mono></span>{/if}
				{#if !lookup.country && !lookup.asn}<span>No GeoIP data for this address.</span>{/if}
			</p>
			{#if outcome}<p role="status" class="mt-1 text-md text-muted-foreground">
					{outcome}
				</p>{/if}
		</div>
		<Button
			variant="outline"
			size="sm"
			onclick={() => {
				target = '';
				note = '';
				addError = '';
				addOpen = true;
			}}>Add to an IP set</Button
		>
		{#if counter && counter.state !== 'none'}
			<Button variant="outline" size="sm" onclick={lift} disabled={busy}
				>{counter.state === 'banned' ? 'Lift the ban' : 'End the challenge'}</Button
			>
		{:else}
			<Button
				variant="outline"
				size="sm"
				onclick={() => enter('sys-dynamic-challenge')}
				disabled={busy}>Challenge now</Button
			>
		{/if}
		{#if counter?.state !== 'banned'}
			<Button
				variant="outline"
				size="sm"
				onclick={() => enter('sys-dynamic-blocked')}
				disabled={busy}>Ban now</Button
			>
		{/if}
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<Panel title="Member of">
			{#if memberOf.length}
				<ul class="space-y-2.5 text-md">
					{#each memberOf as m (m.set.id)}
						<li>
							<SetBadge set={m.set} />
							{#each m.matches as x (x.element_id + x.path.join())}
								<span class="block truncate text-sm text-muted-foreground">{leaf(x)}</span>
							{/each}
						</li>
					{/each}
				</ul>
			{:else}
				<p class="text-md text-muted-foreground">
					No set of yours contains it, only <span class="font-medium text-foreground">all</span>.
				</p>
			{/if}
		</Panel>

		<Panel title="Firewall">
			{#if lookup.firewall.length === 0}
				<p class="text-md text-muted-foreground">No firewall host managed by Hashi.</p>
			{:else}
				<ul class="space-y-2 text-md" aria-label="Firewall verdicts">
					{#each lookup.firewall as v (v.host_id)}
						<li class="flex flex-wrap items-center gap-x-2 gap-y-1">
							<a href="/firewall/{v.host_id}" class="font-medium hover:underline">{v.host_name}</a>
							<Tag tone={verdictTone[v.verdict]}>{verdictText[v.verdict]}</Tag>
							{#if v.set_id}<span class="text-muted-foreground"
									>by <a href="/ip-sets/{v.set_id}" class="hover:underline">{v.set_name}</a>
									<Mono>{v.prefix}</Mono></span
								>{/if}
							{#if v.host_access}<Tag tone="info">may reach the host</Tag>{/if}
						</li>
					{/each}
				</ul>
			{/if}
			{#if memberOf.some((m) => m.set.system_key === 'private_networks')}<p class="mt-2">
					<Tag>private address</Tag>
				</p>{/if}
		</Panel>

		{#if counter}
			<Panel title="Abuse engine">
				<p class="flex flex-wrap items-center gap-2 text-md">
					{#if counter.state === 'banned'}<Tag tone="bad">banned</Tag
						>{:else if counter.state === 'challenged'}<Tag tone="warn">challenged</Tag>{:else}<span
							class="text-muted-foreground">Not banned or challenged now.</span
						>{/if}
					{#if stateDetail}<span class="text-muted-foreground">{stateDetail}</span>{/if}
				</p>
				<dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-md">
					<dt class="text-muted-foreground">Ban counter</dt>
					<dd class="tnum">
						{counter.ban_count}{#if counter.last_ban}<span class="text-muted-foreground">
								· last ban {dateTime(counter.last_ban)}</span
							>{/if}
					</dd>
					<dt class="text-muted-foreground">Challenge counter</dt>
					<dd class="tnum">
						{counter.challenge_count}{#if counter.last_challenge}<span
								class="text-muted-foreground"
							>
								· last challenge {dateTime(counter.last_challenge)}</span
							>{/if}
					</dd>
					{#if counter.last_source}<dt class="text-muted-foreground">Last cause</dt>
						<dd>{sourceLabel(counter.last_source)}</dd>{/if}
				</dl>
				{#if counter.address !== lookup.address}<p class="mt-2 text-sm text-muted-foreground">
						Counted for the whole network <Mono>{counter.address}</Mono>.
					</p>{/if}
				<p class="mt-3 text-sm">
					<a
						href="/security?tab=counters"
						class="text-muted-foreground hover:text-foreground hover:underline"
						>Edit or reset counters on the Security page</a
					>
				</p>
			</Panel>
		{/if}
	</div>

	<Panel title="Recent events" flush class="mt-4">
		<ul class="divide-y text-md" aria-label="Recent events">
			{#each lookup.events as e, i (i)}
				<li
					class="grid grid-cols-[7.5rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5 md:grid-cols-[9rem_minmax(0,1fr)_minmax(0,14rem)_6rem]"
				>
					<span class="tnum text-muted-foreground" title={dateTime(e.at)}>{dateTime(e.at)}</span
					><span class="truncate"
						>{eventLabel(e.type)}{#if e.path}<span class="text-muted-foreground">
								{e.method}
								{e.path}</span
							>{/if}{#if e.rules.length}<span class="text-muted-foreground">
								· rules {e.rules.join(', ')}</span
							>{/if}</span
					><span class="hidden truncate text-muted-foreground md:inline"
						>{e.service || e.host || sourceLabel(e.source)}</span
					><Tag tone={sev[e.severity]}>{e.severity}</Tag>
				</li>
			{:else}
				<li class="px-4 py-6 text-center text-muted-foreground">
					No security events for this address.
				</li>
			{/each}
		</ul>
	</Panel>
{/if}

<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>Add {lookup?.address} to a set</Sheet.Title>
			<Sheet.Description>Takes effect within seconds everywhere the set is used.</Sheet.Description>
		</Sheet.Header>
		<div class="space-y-4 px-4 pb-4">
			<div class="space-y-1.5">
				<Label for="ts">Set</Label>
				<IpSetPicker id="ts" sets={sets.filter((s) => s.editable)} bind:value={target} />
			</div>
			<div class="space-y-1.5">
				<Label for="tn">Note</Label><Input id="tn" bind:value={note} placeholder="Optional" />
			</div>
			{#if addError}<p role="alert" class="text-md text-bad-text">{addError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button onclick={addToSet} disabled={!target}>Add</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
