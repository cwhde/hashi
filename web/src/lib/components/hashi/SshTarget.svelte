<script lang="ts">
	import { onMount } from 'svelte';
	import { Checkbox } from '#lib/components/ui/checkbox/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import Choice from '#lib/components/hashi/Choice.svelte';
	import HelpTip from '#lib/components/hashi/HelpTip.svelte';
	import IpSetPicker from '#lib/components/hashi/ipsets/IpSetPicker.svelte';
	import TargetSources from '#lib/components/hashi/TargetSources.svelte';
	import { api, problemText } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import type { IPSet } from '#lib/ipsets.ts';

	// 08 §5: which address Hashi connects to over SSH, the host's own or the first of an IP set (for instance one that
	// follows a Pulse agent). Part of a host's connection (bug 117): it sits in the Connections drawer and is saved with
	// the drawer's Save through `commit()`.
	let { kind, id, value }: { kind: 'firewall' | 'traefik'; id: string; value?: string | null } =
		$props();
	let sets = $state<IPSet[]>([]);
	// svelte-ignore state_referenced_locally
	let chosen = $state(value ?? '');
	// svelte-ignore state_referenced_locally
	let via = $state<'' | 'set'>(value ? 'set' : '');
	let error = $state('');
	let inactive = $state(false);
	onMount(async () => {
		const r = await api.GET('/api/ip-sets', { params: { query: {} } });
		if (r.data) sets = r.data.sets;
	});

	const target = $derived(via === 'set' ? chosen : '');
	export const dirty = () => target !== (value ?? '') && (via === '' || !!chosen);

	/** Saves the choice; false (with the reason shown here) when the host does not answer through it. */
	export async function commit(): Promise<boolean> {
		error = '';
		const set_id = target;
		try {
			await withReauth(() =>
				kind === 'firewall'
					? api.PUT('/api/firewall/hosts/{id}/ssh-target', {
							params: { path: { id } },
							body: { set_id, save_inactive: inactive }
						})
					: api.PUT('/api/traefik/connections/{id}/ssh-target', {
							params: { path: { id } },
							body: { set_id, save_inactive: inactive }
						})
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
				With an IP set, Hashi uses its first address, for example one that follows a Pulse agent, so
				a new address doesn't break the connection. The SSH address above stays the fallback.
			</p>
		</HelpTip>
	</div>
	<Choice
		id="ssh-via-{id}"
		bind:value={via}
		options={[
			{ value: '', label: 'Its SSH address' },
			{ value: 'set', label: 'The first address of an IP set' }
		]}
	/>
	{#if via === 'set'}
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
