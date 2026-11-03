<script lang="ts">
	// The entries of dynamic-challenge or dynamic-blocked (05a §8): filled by the abuse engine; an entry can be added
	// by hand with an optional duration, ended, or (challenged) banned now or given its failures back.
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import AddressLink from '$lib/components/hashi/security/AddressLink.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { MoreHorizontal } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { dateTime } from '$lib/format';
	import { sourceLabel } from './labels';

	type SetID = 'sys-dynamic-challenge' | 'sys-dynamic-blocked';
	let { setId }: { setId: SetID } = $props();
	const isChallenge = $derived(setId === 'sys-dynamic-challenge');

	let rows = $state<Schemas['CounterRow'][]>([]);
	let total = $state(0);
	let address = $state('');
	let minutes = $state<number | undefined>();
	let busy = $state(false);
	let error = $state('');

	async function load() {
		const r = await api.GET('/api/security/counters', {
			params: { query: { state: isChallenge ? 'challenged' : 'banned', limit: 500 } }
		});
		if (r.error || !r.data) {
			error = problemText(r.error);
			return;
		}
		rows = r.data.rows;
		total = r.data.total;
	}
	onMount(load);

	async function act(f: () => Promise<{ data?: unknown; error?: unknown }>) {
		busy = true;
		error = '';
		try {
			await withReauth(f);
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const addr = (k: string) => k.split('/')[0] ?? k;
	const add = (e: Event) => {
		e.preventDefault();
		return act(() =>
			api.POST('/api/ip-sets/{id}/entries', {
				params: { path: { id: setId } },
				body: {
					address: address.trim(),
					duration_seconds: minutes ? Math.round(minutes * 60) : undefined
				}
			})
		).then(() => {
			if (!error) address = '';
		});
	};
	const end = (r: Schemas['CounterRow']) =>
		act(() =>
			api.DELETE('/api/ip-sets/{id}/entries/{address}', {
				params: { path: { id: setId, address: addr(r.address) } }
			})
		);
	const banNow = (r: Schemas['CounterRow']) =>
		act(() =>
			api.POST('/api/ip-sets/{id}/entries', {
				params: { path: { id: 'sys-dynamic-blocked' } },
				body: { address: addr(r.address) }
			})
		);
	const resetFailures = (r: Schemas['CounterRow']) =>
		act(() =>
			api.PATCH('/api/addresses/{address}/counters', {
				params: { path: { address: addr(r.address) } },
				body: { reset_failures: true }
			})
		);
</script>

<div class="space-y-3">
	{#if error}<p
			role="alert"
			class="rounded-2xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
		>
			{error}
		</p>{/if}
	<form class="flex flex-wrap items-end gap-2" onsubmit={add}>
		<div class="w-full space-y-1.5 sm:w-56">
			<Label for="dyn-address">Address</Label>
			<Input id="dyn-address" bind:value={address} placeholder="198.51.100.7" class="font-mono" />
		</div>
		<div class="w-32 space-y-1.5">
			<Label for="dyn-minutes">Minutes</Label>
			<Input
				id="dyn-minutes"
				type="number"
				min="1"
				bind:value={minutes}
				placeholder={isChallenge ? 'default' : 'by counter'}
			/>
		</div>
		<Button type="submit" disabled={busy || !address.trim()}
			>{isChallenge ? 'Challenge now' : 'Ban now'}</Button
		>
	</form>
	<p class="text-[12.5px] text-muted-foreground">
		{isChallenge
			? 'Without minutes the challenge lasts as long as the engine setting says.'
			: 'Without minutes the ban lasts as long as the address’s ban counter says.'} Allowed addresses
		are never added.
	</p>
	<Panel
		flush
		title="{total} {isChallenge ? 'challenged' : 'banned'} {total === 1 ? 'address' : 'addresses'}"
		description={isChallenge
			? 'Each has to solve a CAPTCHA. Requests without solving count as failures; at the limit the address is banned.'
			: 'Dropped in the kernel on every firewall host and refused by forward auth.'}
	>
		<ul class="divide-y text-[13px]">
			{#each rows as r (r.address)}
				<li
					class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-4 py-2.5 md:grid-cols-[11rem_minmax(0,1fr)_8rem_10rem_auto]"
				>
					<AddressLink address={r.address} />
					<span class="order-last col-span-2 min-w-0 md:order-none md:col-span-1"
						><span class="text-[12px] text-muted-foreground"
							>{sourceLabel(r.last_source)}{#if r.country}, {r.country}{/if}</span
						></span
					>
					<span class="hidden md:block"
						>{#if isChallenge}<span class="tnum">{r.failures} of {r.max_failures}</span><span
								class="block text-[12px] text-muted-foreground">unsolved</span
							>{:else}<span class="tnum">ban {r.ban_count}</span><span
								class="block text-[12px] text-muted-foreground">challenges {r.challenge_count}</span
							>{/if}</span
					>
					<span class="hidden text-muted-foreground md:block"
						>{r.until ? `until ${dateTime(r.until)}` : 'no end'}</span
					>
					<DropdownMenu.Root>
						<DropdownMenu.Trigger>
							{#snippet child({ props })}<Button
									{...props}
									variant="ghost"
									size="icon-sm"
									disabled={busy}
									aria-label="Actions for {r.address}"><MoreHorizontal /></Button
								>{/snippet}
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="end">
							{#if isChallenge}<DropdownMenu.Item onclick={() => banNow(r)}
									>Ban now</DropdownMenu.Item
								><DropdownMenu.Item onclick={() => resetFailures(r)}
									>Reset failures</DropdownMenu.Item
								><DropdownMenu.Separator />{/if}
							<DropdownMenu.Item class="text-bad-text" onclick={() => end(r)}
								>{isChallenge ? 'End challenge' : 'Unban'}</DropdownMenu.Item
							>
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				</li>
			{:else}
				<li class="px-4 py-8 text-center text-muted-foreground">
					{isChallenge ? 'Nobody is challenged right now.' : 'Nobody is banned right now.'}
				</li>
			{/each}
		</ul>
	</Panel>
</div>
