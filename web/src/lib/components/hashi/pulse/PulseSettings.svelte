<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { api, problemText, type Schemas } from '$lib/api/client';

	// 09 §2: what agents are told, and when one counts as stale.
	let v = $state<Schemas['PulseSettings'] | null>(null);
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);
	onMount(async () => {
		const r = await api.GET('/api/settings/pulse');
		if (r.data) v = r.data;
		else error = problemText(r.error);
	});
	async function save() {
		error = notice = '';
		busy = true;
		const r = await api.PUT('/api/settings/pulse', { body: v! });
		busy = false;
		if (r.data) {
			v = r.data;
			notice = 'Saved.';
		} else error = problemText(r.error);
	}
</script>

{#if v}
	<Panel title="Pulse" description="Agents with their own interval in their config keep it.">
		<div class="space-y-3 text-[13px]">
			<div class="grid gap-3 sm:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="ps-int">Heartbeat every (seconds)</Label><Input
						id="ps-int"
						type="number"
						min="10"
						bind:value={v.heartbeat_interval_seconds}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="ps-stale">Stale after missing (heartbeats)</Label><Input
						id="ps-stale"
						type="number"
						min="2"
						bind:value={v.stale_factor}
					/>
				</div>
			</div>
			<div class="flex items-center justify-end gap-3">
				{#if notice}<span role="status" class="text-ok-text">{notice}</span>{/if}
				<Button size="sm" onclick={save} disabled={busy}>Save</Button>
			</div>
		</div>
	</Panel>
{/if}
{#if error}<p role="alert" class="mt-3 text-[13px] text-bad-text">{error}</p>{/if}
