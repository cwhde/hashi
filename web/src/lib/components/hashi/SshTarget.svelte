<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Label } from '$lib/components/ui/label';
	import IpSetPicker from '$lib/components/hashi/ipsets/IpSetPicker.svelte';
	import TargetSources from '$lib/components/hashi/TargetSources.svelte';
	import { api, problemText } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import type { IPSet } from '$lib/ipsets';

	// 08 §5: reach a host through an address set, for instance a Pulse agent. Its literal address stays the fallback,
	// and the pinned host key still has to match.
	let { kind, id, value }: { kind: 'firewall' | 'traefik'; id: string; value?: string | null } =
		$props();
	let sets = $state<IPSet[]>([]);
	let chosen = $state(value ?? '');
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);
	let inactive = $state(false);
	onMount(async () => {
		const r = await api.GET('/api/ip-sets', { params: { query: {} } });
		if (r.data) sets = r.data.sets;
	});
	async function save(set_id: string) {
		error = notice = '';
		busy = true;
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
			chosen = set_id;
			notice = 'Saved.';
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-1.5 text-[13px]">
	<Label for="ssh-target-{id}">Reach over SSH through an IP set (optional)</Label>
	<IpSetPicker id="ssh-target-{id}" bind:value={chosen} {sets} addressOnly />
	<TargetSources setId={chosen} />
	<div class="flex items-center gap-2">
		<Button size="sm" variant="outline" onclick={() => save(chosen)} disabled={busy}>Save</Button>
		{#if chosen}<Button size="sm" variant="ghost" onclick={() => save('')} disabled={busy}
				>Use the fixed address</Button
			>{/if}
		{#if notice}<span role="status" class="text-ok-text">{notice}</span>{/if}
	</div>
	{#if error}
		<p role="alert" class="text-bad-text">{error}</p>
		<label class="flex items-center gap-2"
			><Checkbox bind:checked={inactive} />Save even though it does not answer yet</label
		>
	{/if}
</div>
