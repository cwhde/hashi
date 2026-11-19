<script lang="ts">
	// Settings → Firewall (09 §2): what a new host starts with, and MSS clamping on every host.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';

	type F = Schemas['FirewallDefaultsSettings'];
	let f = $state<F | null>(null);
	let error = $state('');
	const draft = new Draft(() => f);
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);
	onMount(async () => {
		const r = await api.GET('/api/settings/firewall-defaults');
		if (!r.data) error = problemText(r.error);
		else {
			f = r.data;
			draft.mark();
		}
	});
	const save = () =>
		saver.run(
			async () => {
				const r = await api.PUT('/api/settings/firewall-defaults', { body: f! });
				if (!r.data) throw r.error;
				f = r.data;
			},
			'Saved. A changed MSS setting reaches each host as a plan on Pending changes.',
			draft
		);
</script>

{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>
{:else if f}
	<Panel title="New firewall hosts">
		<div class="space-y-1.5">
			<span class="block text-md font-medium">NetBird mode a new host starts with</span>
			<ToggleGroup.Root
				type="single"
				variant="outline"
				value={f.netbird_mode}
				onValueChange={(v) => v && (f!.netbird_mode = v as F['netbird_mode'])}
				aria-label="Default NetBird mode"
			>
				<ToggleGroup.Item value="peer">Peer only</ToggleGroup.Item>
				<ToggleGroup.Item value="routing">Routing peer</ToggleGroup.Item>
				<ToggleGroup.Item value="disabled">Leave NetBird alone</ToggleGroup.Item>
			</ToggleGroup.Root>
		</div>
	</Panel>
	<Panel title="Packet size">
		<label class="flex items-center justify-between gap-3 text-md"
			>Fit TCP packets to narrow networks<Switch bind:checked={f.mss_clamp} /></label
		>
	</Panel>
	<SaveBar {saver} dirty={draft.dirty} onsave={save} />
{/if}
