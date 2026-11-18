<script lang="ts">
	// How long a sign-in at the edge lasts (06 §7). Providers themselves are connections (10c §1.2).
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';

	let l = $state<Schemas['EdgeSSOSettings'] | null>(null);
	let error = $state('');
	const draft = new Draft(() => l);
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);
	onMount(async () => {
		const r = await api.GET('/api/settings/edge-sso');
		if (!r.data) error = problemText(r.error);
		else {
			l = r.data;
			draft.mark();
		}
	});
	const save = () =>
		saver.run(
			async () => {
				const r = await api.PUT('/api/settings/edge-sso', { body: l! });
				if (!r.data) throw r.error;
				l = r.data;
			},
			'Saved.',
			draft
		);
</script>

<Panel title="Edge sign-in">
	{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>
	{:else if l}
		<div class="grid max-w-lg gap-3 sm:grid-cols-2">
			<div class="space-y-1.5">
				<Label for="lm">Longest session (hours)</Label>
				<Input id="lm" type="number" bind:value={l.max_session_hours} />
			</div>
			<div class="space-y-1.5">
				<Label for="li">Ends when unused for (hours)</Label>
				<Input id="li" type="number" bind:value={l.idle_timeout_hours} />
			</div>
		</div>
		<label class="mt-3 flex items-center gap-2 text-md"
			><Switch bind:checked={l.remember_device} />Stay signed in after closing the browser</label
		>
		<div class="mt-3"><SaveBar {saver} dirty={draft.dirty} onsave={save} /></div>
	{/if}
</Panel>
