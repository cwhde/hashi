<script lang="ts">
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';

	// 09 §2: what agents are told, and when one counts as stale.
	let v = $state<Schemas['PulseSettings'] | null>(null);
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);
	const draft = new Draft(() => v);
	guardUnsaved(() => draft.dirty); // bug 40
	onMount(async () => {
		const r = await api.GET('/api/settings/pulse');
		if (r.data) {
			v = r.data;
			draft.mark();
		} else error = problemText(r.error);
	});
	async function save() {
		error = notice = '';
		busy = true;
		const r = await api.PUT('/api/settings/pulse', { body: v! });
		busy = false;
		if (r.data) {
			v = r.data;
			draft.mark();
			notice = 'Saved.';
		} else error = problemText(r.error);
	}
</script>

{#if v}
	<Panel title="Pulse">
		<div class="space-y-3 text-md">
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
			<label class="flex items-start gap-2">
				<Switch bind:checked={v.auto_update} class="mt-0.5" />
				<span>Update agents automatically</span>
			</label>
			<div class="flex items-center justify-end gap-3">
				{#if notice}<span role="status" class="text-ok-text">{notice}</span>{/if}
				<Button size="sm" onclick={save} disabled={busy || !draft.dirty}>Save</Button>
			</div>
		</div>
	</Panel>
{/if}
{#if error}<p role="alert" class="mt-3 text-md text-bad-text">{error}</p>{/if}
