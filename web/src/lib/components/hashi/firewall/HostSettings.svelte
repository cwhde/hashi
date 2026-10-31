<script lang="ts">
	// What validation found, and the settings that depend on it: networks, NetBird, and the choice to drop all other
	// inbound connections (05 §2, §5). Nothing here reaches the host until a plan is approved.
	import { untrack } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import SshTarget from '$lib/components/hashi/SshTarget.svelte';
	import { Check, CircleAlert, TriangleAlert } from '@lucide/svelte';
	import { firewallInput, lines, type FirewallHost, type FirewallInput } from '$lib/firewall';

	type Props = { host: FirewallHost; busy: boolean; save: (b: FirewallInput) => Promise<void> };
	let { host, busy, save }: Props = $props();
	const f = $derived(host.facts);

	let subnets = $state(untrack(() => host.subnets.join(', ')));
	let mode = $state(untrack(() => host.netbird_mode));
	let drop = $state(untrack(() => host.drop_other_inbound));

	const nb = $derived(f?.netbird);
	function submit(e: Event) {
		e.preventDefault();
		const b = firewallInput(host);
		b.subnets = lines(subnets);
		b.netbird_mode = mode;
		b.netbird_iface = mode === 'disabled' ? '' : host.netbird_iface || nb?.iface || '';
		b.netbird_port = mode === 'disabled' ? 0 : host.netbird_port || nb?.port || 0;
		b.drop_other_inbound = drop;
		return save(b);
	}
</script>

<ul class="space-y-1.5 text-[13px]" aria-label="Checks">
	{#if host.health === 'failed'}
		<li class="flex items-start gap-2 text-bad-text">
			<CircleAlert class="mt-0.5 size-4 shrink-0" />{host.last_error}
		</li>
	{:else if f}
		<li class="flex items-center gap-2">
			<Check class="size-4 text-ok-text" />{f.os}, kernel {f.kernel}, nftables {f.nft_version}
		</li>
		<li class="flex items-center gap-2">
			<Check class="size-4 text-ok-text" />Public address <Mono>{host.public_ip}</Mono> on <Mono
				>{host.wan_iface}</Mono
			>
		</li>
		{#if nb}
			<li class="flex items-center gap-2">
				<Check class="size-4 text-ok-text" />NetBird {nb.version} on <Mono>{nb.iface}</Mono>, {nb.connected
					? 'connected'
					: 'not connected'}
			</li>
		{/if}
		{#if f.ssh_from}<li class="flex items-center gap-2">
				<Check class="size-4 text-ok-text" />Hashi reaches it from <Mono>{f.ssh_from}</Mono>
			</li>{/if}
		{#each f.warnings as w (w)}
			<li class="flex items-start gap-2 text-warn-text">
				<TriangleAlert class="mt-0.5 size-4 shrink-0" />{w}
			</li>
		{/each}
	{/if}
</ul>

<form class="mt-5 max-w-[62ch] space-y-4" onsubmit={submit}>
	<div class="space-y-1.5">
		<Label for="fs-sub">Networks behind this host</Label>
		<Input id="fs-sub" bind:value={subnets} class="h-10" />
		{#if f?.candidates.length}
			<p class="text-[12px] text-muted-foreground">
				Found on the host: <Mono>{f.candidates.join(', ')}</Mono>
			</p>
		{/if}
	</div>
	<div class="space-y-1.5">
		<Label for="fs-nb">NetBird</Label>
		<select
			id="fs-nb"
			bind:value={mode}
			class="h-10 w-full rounded-[10px] border bg-transparent px-3 text-[13px]"
		>
			<option value="disabled">Not used on this host</option>
			<option value="peer" disabled={!nb}>This host is a NetBird peer</option>
			<option value="routing" disabled={!nb}
				>This host also routes NetBird traffic to its networks</option
			>
		</select>
		<p class="text-[12px] text-muted-foreground">
			Hashi never changes NetBird's rules. Its interface and port stay open.
		</p>
	</div>
	<label class="flex items-start gap-2.5 text-[13px]">
		<input type="checkbox" bind:checked={drop} class="mt-0.5 accent-primary" />
		<span
			>Block all other inbound connections. Without this, anything listening on the host, such as
			SSH or Proxmox, stays reachable from the internet. People on the host access list and NetBird
			can still connect.</span
		>
	</label>
	<Button type="submit" variant="outline" disabled={busy}>Save settings</Button>
</form>

<div class="mt-4">
	<SshTarget kind="firewall" id={host.id} value={host.ssh_target_set_id} />
</div>
