<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import CardColumns from '#lib/components/hashi/CardColumns.svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import SetBadge from '#lib/components/hashi/ipsets/SetBadge.svelte';
	import IpSetPicker from '#lib/components/hashi/ipsets/IpSetPicker.svelte';
	import CounterEditDialog from '#lib/components/hashi/security/CounterEditDialog.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import { ChevronLeft, CircleAlert, Search } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
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
		outcome = ''; // the last action's line belongs to the address it was about (R4-008)
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
	// Firewall sets need a fresh passkey: ask for it when the server says so, then retry. Returns the error text or null.
	async function confirmed<T>(
		f: () => Promise<{ data?: T; error?: unknown }>,
		then?: (data: T) => void
	): Promise<string | null> {
		try {
			// Not `then?.(await …)`: an optional call skips its arguments, and the request with them, when `then` is unset.
			const data = await withReauth(f);
			then?.(data);
			return null;
		} catch (e) {
			return problemText(e);
		}
	}
	const target64 = () => lookup?.address ?? address;
	async function enter(set: 'sys-dynamic-blocked' | 'sys-dynamic-challenge') {
		busy = true;
		const a = target64();
		const ban = set === 'sys-dynamic-blocked';
		let done = '';
		const err = await confirmed(
			() =>
				api.POST('/api/ip-sets/{id}/entries', {
					params: { path: { id: set } },
					body: { address: a }
				}),
			(r) => {
				// The engine skips an allowed address and one that is already held (06 §3.4); say so (R4-002).
				const who = r.counters.address; // an IPv6 address counts for its /64
				const held = r.counters.state;
				done = r.created
					? ban
						? `${who} is banned.`
						: `${who} gets the challenge page now.`
					: held === 'banned'
						? `${who} is already banned.`
						: held === 'challenged' && !ban
							? `${who} is already challenged.`
							: `Not ${ban ? 'banned' : 'challenged'}: ${who} is in an allowed set.`;
			}
		);
		busy = false;
		outcome = err ?? done;
		if (!err) await load(a).catch(() => {});
	}
	async function lift() {
		busy = true;
		const a = target64();
		const was = lookup?.counter?.state;
		const err = await confirmed(() =>
			api.DELETE('/api/ip-sets/{id}/entries/{address}', {
				params: { path: { id: 'sys-dynamic-blocked', address: a } }
			})
		);
		busy = false;
		outcome =
			err ??
			(was === 'banned' ? `The ban on ${a} is lifted.` : `The challenge for ${a} has ended.`);
		if (!err) await load(a).catch(() => {});
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
	// A new set starts with this address as its first item, in one step (bug 66): a refused item creates nothing.
	let newSet = $state<{ slug: string; description: string } | null>(null);
	// The server's rule for a set's name (05a): 1–63 characters, no dash at either end.
	const slugOk = (v: string) => /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(v) && !slugTaken(v);
	const slugTaken = (v: string) => sets.some((s) => s.slug === v);
	async function addToSet() {
		addError = '';
		if (newSet) return createSet(newSet);
		const into = sets.find((s) => s.id === target)?.slug;
		if (memberOf.some((m) => m.set.id === target)) {
			addError = `${lookup?.address} is already in ${into}.`; // a second identical item helps nobody (R4-009)
			return;
		}
		const err = await confirmed(() =>
			api.POST('/api/ip-sets/{id}/elements', {
				params: { path: { id: target } },
				body: { item: { type: 'ip', address: lookup?.address ?? address }, note }
			})
		);
		if (err) {
			addError = err;
			return;
		}
		addOpen = false;
		outcome = `Added to ${into}.`;
		await load(address);
	}
	async function createSet(f: { slug: string; description: string }) {
		const slug = f.slug.trim().toLowerCase();
		const err = await confirmed(() =>
			api.POST('/api/ip-sets', {
				body: {
					slug,
					name: slug,
					description: f.description,
					first_item: { item: { type: 'ip', address: lookup?.address ?? address }, note }
				}
			})
		);
		if (err) {
			addError = err;
			return;
		}
		addOpen = false;
		outcome = `Added to the new set ${slug}.`;
		await load(address);
	}

	// ---- counters ----
	let editCounters = $state<string | null>(null);
	async function resetCounters() {
		const c = counter;
		if (!c) return;
		busy = true;
		const err = await confirmed(() =>
			api.POST('/api/security/counters/reset', { body: { addresses: [c.address] } })
		);
		busy = false;
		outcome = err ?? 'Counters reset.';
		if (!err) await load(target64()).catch(() => {});
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
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-md wrap-anywhere text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if address && !lookup}
	<Skeleton class="h-60 rounded-2xl" />
{:else if address && lookup}
	<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
		<div class="min-w-0 grow basis-64">
			<h1 class="font-mono text-title leading-tight font-medium tracking-[-0.02em] wrap-anywhere">
				{lookup.address}
			</h1>
			<p
				class="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-md text-muted-foreground"
				aria-label="GeoIP"
			>
				{#if lookup.country}<span title={lookup.country}>{country(lookup.country)}</span>{/if}
				{#if lookup.asn}<span
						><Mono>AS{lookup.asn}</Mono>{#if lookup.as_name}{' '}
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
				newSet = null;
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

	{#snippet firewall()}{#if lookup}
			<Panel title="Firewall">
				{#if lookup.firewall.length === 0}
					<p class="text-md text-muted-foreground">No firewall host managed by Hashi.</p>
				{:else}
					<ul class="space-y-2 text-md" aria-label="Firewall verdicts">
						{#each lookup.firewall as v (v.host_id)}
							<li class="flex flex-wrap items-center gap-x-2 gap-y-1">
								<a href="/firewall/{v.host_id}" class="font-medium hover:underline">{v.host_name}</a
								>
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
		{/if}{/snippet}
	<!-- The tall abuse card gets a column of its own; without it, the two short cards share the row (bug 158). -->
	<CardColumns>
		{#snippet left()}
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
			{#if counter}{@render firewall()}{/if}
		{/snippet}
		{#snippet right()}{#if lookup}
				{#if counter}
					<Panel title="Abuse engine">
						<p class="flex flex-wrap items-center gap-2 text-md">
							{#if counter.state === 'banned'}<Tag tone="bad">banned</Tag
								>{:else if counter.state === 'challenged'}<Tag tone="warn">challenged</Tag
								>{:else}<span data-state class="text-muted-foreground"
									>Not banned or challenged now.</span
								>{/if}
							{#if stateDetail}<span class="text-muted-foreground">{stateDetail}</span>{/if}
						</p>
						<dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-md">
							<dt class="text-muted-foreground">Ban counter</dt>
							<dd class="tnum">
								{counter.ban_count}{#if counter.last_ban}{' '}<span class="text-muted-foreground">
										· last ban {dateTime(counter.last_ban)}</span
									>{/if}
							</dd>
							<dt class="text-muted-foreground">Challenge counter</dt>
							<dd class="tnum">
								{counter.challenge_count}{#if counter.last_challenge}{' '}<span
										class="text-muted-foreground"
									>
										· last challenge {dateTime(counter.last_challenge)}</span
									>{/if}
							</dd>
							{#if counter.last_source}<dt class="text-muted-foreground">Last cause</dt>
								<dd>{sourceLabel(counter.last_source)}</dd>{/if}
						</dl>
						{#if counter.address !== lookup.address}<p
								data-state
								class="mt-2 text-sm text-muted-foreground"
							>
								Counted for the whole network <Mono>{counter.address}</Mono>.
							</p>{/if}
						<div class="mt-3 flex flex-wrap gap-2">
							<Button variant="outline" size="sm" onclick={() => (editCounters = counter!.address)}
								>Edit counters</Button
							>
							<Button
								variant="ghost"
								size="sm"
								disabled={busy || (counter.ban_count === 0 && counter.challenge_count === 0)}
								onclick={resetCounters}>Reset to zero</Button
							>
						</div>
					</Panel>
				{:else}{@render firewall()}{/if}
			{/if}{/snippet}
	</CardColumns>

	<Panel title="Recent events" flush class="mt-4">
		<ul class="divide-y text-md" aria-label="Recent events">
			{#each lookup.events as e, i (i)}
				<li
					class="grid grid-cols-[7.5rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5 md:grid-cols-[9rem_minmax(0,1fr)_minmax(0,14rem)_6rem]"
				>
					<span class="tnum text-muted-foreground" title={dateTime(e.at)}>{dateTime(e.at)}</span
					><span class="truncate"
						>{eventLabel(e.type)}{#if e.path}{' '}<span class="text-muted-foreground">
								{e.method}
								{e.path}</span
							>{/if}{#if e.rules.length}{' '}<span class="text-muted-foreground">
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

<CounterEditDialog
	bind:address={editCounters}
	ban={counter?.ban_count}
	challenge={counter?.challenge_count}
	onsaved={async () => {
		outcome = 'Counters saved.';
		await load(target64()).catch(() => {});
	}}
/>

<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title class="pr-8 wrap-anywhere">Add {lookup?.address} to a set</Sheet.Title>
		</Sheet.Header>
		<form
			class="contents"
			onsubmit={(e) => {
				e.preventDefault();
				if (newSet ? slugOk(newSet.slug) : target) addToSet();
			}}
		>
			<div class="space-y-4 px-4 pb-4">
				{#if newSet}
					<div class="space-y-1.5">
						<div class="flex items-baseline justify-between gap-2">
							<Label for="ns">New set</Label><button
								type="button"
								class="text-sm text-muted-foreground hover:text-foreground hover:underline"
								onclick={() => (newSet = null)}>Pick an existing set</button
							>
						</div>
						<Input
							id="ns"
							class="font-mono"
							bind:value={newSet.slug}
							placeholder="family-devices"
							{@attach (el) => el.focus()}
						/>
						{#if newSet.slug && !slugOk(newSet.slug)}<p class="text-sm text-bad-text">
								{slugTaken(newSet.slug)
									? 'Another set already uses this name.'
									: 'Up to 63 lowercase letters, digits and dashes, starting and ending with a letter or digit.'}
							</p>{/if}
					</div>
					<div class="space-y-1.5">
						<Label for="nd">Description</Label><Input
							id="nd"
							bind:value={newSet.description}
							placeholder="Optional"
						/>
					</div>
				{:else}
					<div class="space-y-1.5">
						<Label for="ts">Set</Label>
						<IpSetPicker
							id="ts"
							sets={sets.filter((s) => s.editable)}
							bind:value={target}
							oncreate={(slug) => (newSet = { slug, description: '' })}
						/>
					</div>
				{/if}
				<div class="space-y-1.5">
					<Label for="tn">Note</Label><Input id="tn" bind:value={note} placeholder="Optional" />
				</div>
				{#if addError}<p role="alert" class="text-md text-bad-text">{addError}</p>{/if}
			</div>
			<Sheet.Footer class="flex-row justify-end gap-2 border-t">
				<Button type="button" variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
				{#if newSet}<Button type="submit" disabled={!slugOk(newSet.slug)}>Create and add</Button
					>{:else}<Button type="submit" disabled={!target}>Add</Button>{/if}
			</Sheet.Footer>
		</form>
	</Sheet.Content>
</Sheet.Root>
