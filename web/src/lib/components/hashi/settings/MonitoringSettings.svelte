<script lang="ts">
	// Settings → Monitoring and notifications (10c §1.2): what a new check starts with, how long results stay, and where
	// alerts go. Deliveries are history and live in Activity.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import Notifications from '#lib/components/hashi/status/Notifications.svelte';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';

	let m = $state<Schemas['MonitoringSettings'] | null>(null);
	let error = $state('');
	const draft = new Draft(() => m);
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);
	onMount(async () => {
		const r = await api.GET('/api/settings/monitoring');
		if (!r.data) error = problemText(r.error);
		else {
			m = r.data;
			draft.mark();
		}
	});
	const save = () =>
		saver.run(
			async () => {
				const r = await api.PUT('/api/settings/monitoring', { body: m! });
				if (!r.data) throw r.error;
				m = r.data;
			},
			'Saved. Checks that exist keep their own values.',
			draft
		);
</script>

{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>
{:else if m}
	<Panel title="New checks">
		<div class="grid gap-4 md:grid-cols-2">
			<div class="space-y-1.5">
				<Label for="mi">Check every (seconds)</Label><Input
					id="mi"
					type="number"
					min="10"
					bind:value={m.interval_seconds}
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="mt">Give up after (seconds)</Label><Input
					id="mt"
					type="number"
					min="1"
					max="60"
					bind:value={m.timeout_seconds}
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="mc">Healthy HTTP codes</Label>
				<div class="flex items-center gap-2">
					<Input
						id="mc"
						type="number"
						min="100"
						max="599"
						bind:value={m.code_min}
						aria-label="Healthy HTTP codes from"
					/><span class="text-muted-foreground">to</span><Input
						type="number"
						min="100"
						max="599"
						bind:value={m.code_max}
						aria-label="Healthy HTTP codes to"
					/>
				</div>
			</div>
			<div class="space-y-1.5">
				<Label for="md">Slow above (ms)</Label><Input
					id="md"
					type="number"
					min="0"
					placeholder="0 = never slow"
					bind:value={m.degraded_ms}
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="mr">Keep every result for (days)</Label><Input
					id="mr"
					type="number"
					min="1"
					max="400"
					bind:value={m.retention_days}
				/>
			</div>
		</div>
		<div class="mt-3"><SaveBar {saver} dirty={draft.dirty} onsave={save} /></div>
	</Panel>
{/if}
<Notifications />
