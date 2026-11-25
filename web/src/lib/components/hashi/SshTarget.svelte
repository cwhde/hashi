<script lang="ts">
	import { onMount } from 'svelte';
	import { Checkbox } from '#lib/components/ui/checkbox/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import Choice from '#lib/components/hashi/Choice.svelte';
	import HelpTip from '#lib/components/hashi/HelpTip.svelte';
	import IpSetPicker from '#lib/components/hashi/ipsets/IpSetPicker.svelte';
	import TargetSources from '#lib/components/hashi/TargetSources.svelte';
	import { api, problemText } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { pulseAgents } from '#lib/pulseAgents.svelte.ts';
	import type { IPSet } from '#lib/ipsets.ts';

	// 08 §5: which address Hashi connects to over SSH: the host's own, a Pulse agent's main address (bug 198), or the
	// first of an IP set. Part of a host's connection (bug 117): it sits in the Connections drawer and is saved with
	// the drawer's Save through `commit()`.
	let {
		kind,
		id,
		value,
		agent
	}: { kind: 'firewall' | 'traefik'; id: string; value?: string | null; agent?: string | null } =
		$props();
	let sets = $state<IPSet[]>([]);
	// svelte-ignore state_referenced_locally
	let chosen = $state(agent ? '' : (value ?? ''));
	// svelte-ignore state_referenced_locally
	let chosenAgent = $state(agent ?? '');
	// svelte-ignore state_referenced_locally
	let via = $state<'' | 'agent' | 'set'>(agent ? 'agent' : value ? 'set' : '');
	let error = $state('');
	let inactive = $state(false);
	onMount(async () => {
		void pulseAgents.load();
		const r = await api.GET('/api/ip-sets', { params: { query: {} } });
		if (r.data) sets = r.data.sets;
	});

	const target = $derived(
		via === 'agent'
			? { set_id: '', agent_id: chosenAgent }
			: { set_id: via === 'set' ? chosen : '' }
	);
	// svelte-ignore state_referenced_locally
	const before = agent ? `agent:${agent}` : (value ?? '');
	export const dirty = () => {
		const now = target.agent_id ? `agent:${target.agent_id}` : target.set_id;
		return now !== before && (via === '' || !!(via === 'agent' ? chosenAgent : chosen));
	};

	/** Saves the choice; false (with the reason shown here) when the host does not answer through it. */
	export async function commit(): Promise<boolean> {
		error = '';
		const body = { ...target, save_inactive: inactive };
		try {
			await withReauth(() =>
				kind === 'firewall'
					? api.PUT('/api/firewall/hosts/{id}/ssh-target', { params: { path: { id } }, body })
					: api.PUT('/api/traefik/connections/{id}/ssh-target', { params: { path: { id } }, body })
			);
			return true;
		} catch (e) {
			error = problemText(e);
			return false;
		}
	}
</script>

<div class="space-y-1.5">
	<div class="flex items-center gap-1">
		<Label for="ssh-via-{id}">Hashi connects to</Label>
		<HelpTip topic="where Hashi connects">
			<p>
				With a Pulse agent, Hashi uses the agent's main address; with an IP set, its first address.
				A new address then doesn't break the connection. The SSH address above stays the fallback.
			</p>
		</HelpTip>
	</div>
	<Choice
		id="ssh-via-{id}"
		bind:value={via}
		options={[
			{ value: '', label: 'Its SSH address' },
			{ value: 'agent', label: 'A Pulse agent' },
			{ value: 'set', label: 'The first address of an IP set' }
		]}
	/>
	{#if via === 'agent'}
		<Select.Root type="single" bind:value={chosenAgent}>
			<Select.Trigger aria-label="Pulse agent" class="w-full"
				>{pulseAgents.list.find((a) => a.id === chosenAgent)?.name ??
					'Choose an agent'}</Select.Trigger
			>
			<Select.Content>
				{#each pulseAgents.list as a (a.id)}<Select.Item value={a.id}>{a.name}</Select.Item>{/each}
			</Select.Content>
		</Select.Root>
	{:else if via === 'set'}
		<IpSetPicker id="ssh-target-{id}" bind:value={chosen} {sets} addressOnly />
		{#if chosen}<TargetSources setId={chosen} />{/if}
	{/if}
	{#if error}
		<p role="alert" class="text-bad-text">{error}</p>
		<label class="flex items-center gap-2"
			><Checkbox bind:checked={inactive} />Save even though it does not answer yet</label
		>
	{/if}
</div>
