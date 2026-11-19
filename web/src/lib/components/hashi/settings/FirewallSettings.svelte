<script lang="ts">
	import Choice from '#lib/components/hashi/Choice.svelte';
	import HelpTip from '#lib/components/hashi/HelpTip.svelte';
	import { Label } from '#lib/components/ui/label/index.ts';
	// Settings → Firewall (09 §2): what a new host starts with, and MSS clamping on every host.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
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
			<span class="flex items-center gap-1"
				><Label for="fw-nb-default">When Hashi finds NetBird on a new host</Label><HelpTip
					topic="the NetBird default"
					><p>Only where a new host starts. Each host's own page can change it.</p></HelpTip
				></span
			>
			<Choice
				id="fw-nb-default"
				value={f.netbird_mode}
				onchange={(v) => (f!.netbird_mode = v as F['netbird_mode'])}
				options={[
					{ value: 'peer', label: 'The host is a NetBird peer' },
					{ value: 'routing', label: 'The host also routes NetBird traffic to its networks' },
					{ value: 'disabled', label: "Don't use NetBird on it" }
				]}
			/>
		</div>
	</Panel>
	<Panel title="Packet size">
		<div class="flex items-center justify-between gap-3">
			<Label for="fw-mss">Fit TCP packets to narrow networks</Label><Switch
				id="fw-mss"
				bind:checked={f.mss_clamp}
			/>
		</div>
	</Panel>
	<SaveBar {saver} dirty={draft.dirty} onsave={save} />
{/if}
