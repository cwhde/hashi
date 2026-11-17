<script lang="ts">
	// A host behind a router (05 §1.1 external_nat): its public address is typed in or follows a Pulse agent on its
	// network (bug 23), and its internal addresses come from the same agent.
	import { onMount, untrack } from 'svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import { pulseAgents } from '#lib/pulseAgents.svelte.ts';
	import { firewallInput, type FirewallHost, type FirewallInput } from '#lib/firewall.ts';

	type Props = { host: FirewallHost; busy: boolean; save: (b: FirewallInput) => Promise<void> };
	let { host, busy, save }: Props = $props();
	onMount(() => void pulseAgents.load());

	let agent = $state(untrack(() => host.address_agent_id ?? ''));
	let typed = $state(untrack(() => host.public_ip_override));
	const changed = $derived(
		agent !== (host.address_agent_id ?? '') || (!agent && typed.trim() !== host.public_ip_override)
	);
	function submit(e: Event) {
		e.preventDefault();
		return save({
			...firewallInput(host),
			address_agent_id: agent || undefined,
			public_ip_override: agent ? '' : typed.trim()
		});
	}
</script>

<form class="space-y-3 text-md" onsubmit={submit}>
	<div class="space-y-1.5">
		<Label for="xa-src">Its address comes from</Label>
		<select
			id="xa-src"
			bind:value={agent}
			class="h-10 w-full rounded-[10px] border bg-transparent px-3 text-md"
		>
			<option value="">What I type here</option>
			{#each pulseAgents.list as a (a.id)}<option value={a.id}>Pulse agent {a.name}</option>{/each}
		</select>
	</div>
	{#if agent}
		<dl class="grid grid-cols-[8rem_minmax(0,1fr)] gap-y-1.5">
			<dt class="text-muted-foreground">Public</dt>
			<dd><Mono>{host.public_ip || '—'}</Mono></dd>
			<dt class="text-muted-foreground">Internal</dt>
			<dd class="truncate"><Mono>{host.internal_ips?.join(', ') || '—'}</Mono></dd>
		</dl>
	{:else}
		<div class="space-y-1.5">
			<Label for="xa-pub">Public address</Label>
			<Input id="xa-pub" bind:value={typed} class="h-10 font-mono" />
		</div>
	{/if}
	<p>
		Hashi does not touch this host's firewall. Forward the public ports on your router to <Mono
			>{host.internal_traefik_ip || 'its Traefik'}</Mono
		>.
	</p>
	<Button type="submit" size="sm" disabled={busy || !changed}>Save</Button>
</form>
