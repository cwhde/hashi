<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import SetBadge from '$lib/components/hashi/ipsets/SetBadge.svelte';
	import IpSetPicker from '$lib/components/hashi/ipsets/IpSetPicker.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Sheet from '$lib/components/ui/sheet';
	import { ChevronLeft, CircleAlert, Search } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { addressHref, listSets, type IPSet } from '$lib/ipsets';

	// Everything Hashi knows about one address (05a §6). This version shows set membership with the path through
	// nesting; firewall verdicts, dynamic state, counters and the request trace join with their slices.
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

	const memberOf = $derived(lookup?.sets.filter((m) => m.set.system_key !== 'all') ?? []);
	const leaf = (m: Schemas['AddressMatch']) => [...m.path, m.label].join(' › ');

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

<a
	href="/ip-sets"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />IP sets</a
>

<form
	class="relative mb-5 max-w-80"
	onsubmit={(e) => {
		e.preventDefault();
		if (jump.trim()) goto(addressHref(jump.trim()));
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

{#if !address}
	<p class="text-[13px] text-muted-foreground">
		Type an address to see every set that contains it, and through what.
	</p>
{:else if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !lookup}
	<Skeleton class="h-60 rounded-2xl" />
{:else}
	<div class="mb-5 flex flex-wrap items-end gap-x-6 gap-y-3">
		<div class="min-w-64 flex-1">
			<h1 class="font-mono text-[24px] leading-tight font-medium tracking-[-0.02em]">
				{lookup.address}
			</h1>
			{#if outcome}<p role="status" class="mt-1 text-[13.5px] text-muted-foreground">
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
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<Panel title="Member of" description="Sets that contain this address, and through what">
			{#if memberOf.length}
				<ul class="space-y-2.5 text-[13px]">
					{#each memberOf as m (m.set.id)}
						<li>
							<SetBadge set={m.set} />
							{#each m.matches as x (x.element_id + x.path.join())}
								<span class="block truncate text-[12.5px] text-muted-foreground">{leaf(x)}</span>
							{/each}
						</li>
					{/each}
				</ul>
			{:else}
				<p class="text-[13px] text-muted-foreground">
					Only in <span class="font-medium text-foreground">all</span>. No set of yours contains it.
				</p>
			{/if}
		</Panel>

		<Panel title="Firewall" description="Allow and block folded together, per host">
			<p class="text-[13px] text-muted-foreground">
				Verdicts appear here once a firewall host is registered.
			</p>
			{#if memberOf.some((m) => m.set.system_key === 'private_networks')}<p class="mt-2">
					<Tag>private address</Tag>
				</p>{/if}
		</Panel>
	</div>
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
			{#if addError}<p role="alert" class="text-[13px] text-bad-text">{addError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button onclick={addToSet} disabled={!target}>Add</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
