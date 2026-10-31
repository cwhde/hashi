<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import SetBadge from '$lib/components/hashi/ipsets/SetBadge.svelte';
	import IpSetPicker from '$lib/components/hashi/ipsets/IpSetPicker.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Plus, ArrowUpRight, X } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { listSets, type IPSet } from '$lib/ipsets';
	import { healthState, type FirewallHost } from '$lib/firewall';
	import { saveText } from '$lib/services';

	let hosts = $state<FirewallHost[]>([]);
	let sets = $state<IPSet[]>([]);
	let bind = $state<Schemas['FirewallBindings']>({ allow: [], block: [] });
	let pick = $state({ allow: '', block: '' });
	let note = $state('');
	let error = $state('');
	const setOf = (id: string) => sets.find((s) => s.id === id);

	onMount(async () => {
		try {
			const [h, b] = await Promise.all([
				api.GET('/api/firewall/hosts'),
				api.GET('/api/firewall/bindings')
			]);
			if (h.error || !h.data) throw h.error;
			if (b.error || !b.data) throw b.error;
			hosts = h.data.hosts;
			bind = b.data;
			sets = await listSets();
		} catch (e) {
			error = problemText(e);
		}
	});

	async function put(next: Schemas['FirewallBindings']) {
		error = note = '';
		try {
			const r = await withReauth(() => api.PUT('/api/firewall/bindings', { body: next }));
			bind = next;
			note = saveText({ plans: r.plans, certificate_pending: false }).text;
		} catch (e) {
			error = problemText(e);
		}
	}
</script>

<PageHeader
	title="Firewall hosts"
	description="Machines with a public IP that forward traffic to Traefik. Hashi owns one nftables table on each; NetBird, Proxmox and Docker rules stay untouched."
>
	{#snippet actions()}<Button href="/firewall/new"><Plus />Add firewall host</Button>{/snippet}
</PageHeader>

{#if error}<p
		role="alert"
		class="mb-4 rounded-2xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
	>
		{error}
	</p>{/if}
{#if note}<p role="status" class="mb-4 rounded-2xl border bg-card p-3 text-[13px]">
		{note} <a class="underline" href="/changes">Open changes</a>
	</p>{/if}

<Panel
	title="Global policy"
	description="Applied on every managed host. Where allow and block overlap, the longer prefix wins; on a tie, block."
	class="mb-4"
>
	<div class="grid gap-4 md:grid-cols-2">
		{#each [{ key: 'allow', label: 'Always allow', help: 'Never dropped and never banned by the abuse engine.' }, { key: 'block', label: 'Block', help: 'Dropped before anything else.' }] as side (side.key)}
			{@const k = side.key as 'allow' | 'block'}
			<div>
				<div class="mb-2 text-[13px] font-medium">{side.label}</div>
				<ul class="space-y-1.5 text-[13px]" aria-label={side.label}>
					{#each bind[k] as id (id)}
						<li class="flex items-center gap-2">
							{#if setOf(id)}<SetBadge set={setOf(id)!} />{:else}<Mono>{id}</Mono>{/if}
							<Button
								variant="ghost"
								size="icon-xs"
								aria-label="Remove"
								onclick={() => put({ ...bind, [k]: bind[k].filter((x) => x !== id) })}><X /></Button
							>
						</li>
					{/each}
					{#if k === 'allow'}<li class="text-muted-foreground">hashi-hosts (always)</li>{/if}
				</ul>
				<div class="mt-2 flex gap-2">
					<IpSetPicker bind:value={pick[k]} {sets} exclude={bind[k]} id="pick-{k}" />
					<Button
						size="sm"
						variant="outline"
						disabled={!pick[k]}
						onclick={async () => {
							await put({ ...bind, [k]: [...bind[k], pick[k]] });
							pick[k] = '';
						}}>Add</Button
					>
				</div>
				<p class="mt-2 text-[12.5px] text-muted-foreground">{side.help}</p>
			</div>
		{/each}
	</div>
</Panel>

<div class="grid gap-4 lg:grid-cols-2">
	{#each hosts as h (h.id)}
		<Panel>
			<div class="flex items-start gap-3">
				<div class="min-w-0 flex-1">
					<div class="flex flex-wrap items-center gap-2 text-[15px] font-semibold">
						{h.name}<StatusBadge state={healthState(h)} /><Tag
							>{h.edge_mode === 'managed' ? 'managed' : 'external NAT'}</Tag
						>
					</div>
					<div class="text-[12.5px] text-muted-foreground"><Mono>{h.public_ip}</Mono></div>
				</div>
				<Button variant="ghost" size="icon-sm" href="/firewall/{h.id}" aria-label="Open {h.name}"
					><ArrowUpRight /></Button
				>
			</div>
			<dl class="mt-4 grid grid-cols-[8rem_1fr] gap-y-2 text-[13px]">
				<dt class="text-muted-foreground">Subnets</dt>
				<dd><Mono>{h.subnets.join(', ')}</Mono></dd>
				<dt class="text-muted-foreground">Forwards to</dt>
				<dd><Mono>{h.internal_traefik_ip}</Mono></dd>
				<dt class="text-muted-foreground">NetBird</dt>
				<dd>{h.netbird_mode}</dd>
				<dt class="text-muted-foreground">Last apply</dt>
				<dd>{h.last_applied_at ? new Date(h.last_applied_at).toLocaleString() : 'never'}</dd>
			</dl>
		</Panel>
	{:else}
		<p class="text-[13px] text-muted-foreground">
			No firewall hosts yet. Add the machine that receives your public traffic.
		</p>
	{/each}
</div>
