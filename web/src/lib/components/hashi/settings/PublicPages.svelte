<script lang="ts">
	// Settings → Public pages (10c §1.2, bug 65): the two public pages on or off, and how the dashboard starts. Which
	// service shows on them is an instant switch on App display.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';

	type D = Schemas['DashboardSettings'];
	let pages = $state<{ dashboard: boolean; status: boolean } | null>(null);
	let d = $state<D | null>(null);
	let error = $state('');
	const draft = new Draft(() => [pages, d]);
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);
	onMount(async () => {
		const [a, b] = await Promise.all([
			api.GET('/api/settings/appearance'),
			api.GET('/api/settings/dashboard')
		]);
		if (!a.data || !b.data) {
			error = problemText(a.error ?? b.error);
			return;
		}
		pages = { dashboard: a.data.public_dashboard_enabled, status: a.data.public_status_enabled };
		d = b.data;
		draft.mark();
	});
	const save = () =>
		saver.run(
			async () => {
				// The page switches share the appearance record with the theme: change only them.
				const cur = await api.GET('/api/settings/appearance');
				if (!cur.data) throw cur.error;
				await withReauth(() =>
					api.PUT('/api/settings/appearance', {
						body: {
							...cur.data,
							public_dashboard_enabled: pages!.dashboard,
							public_status_enabled: pages!.status
						}
					})
				);
				const r = await api.PUT('/api/settings/dashboard', { body: d! });
				if (!r.data) throw r.error;
				d = r.data;
			},
			'Saved.',
			draft
		);
</script>

{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>
{:else if pages && d}
	<Panel title="Pages">
		<div class="grid gap-3 md:grid-cols-2">
			<label class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-md"
				>Public app dashboard<Switch bind:checked={pages.dashboard} /></label
			>
			<label class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-md"
				>Public status page<Switch bind:checked={pages.status} /></label
			>
		</div>
	</Panel>
	<Panel title="App dashboard">
		<div class="space-y-4">
			<div class="space-y-1.5">
				<span class="block text-md font-medium">Visitors first see the apps sorted by</span>
				<ToggleGroup.Root
					type="single"
					variant="outline"
					value={d.default_sort}
					onValueChange={(v) => v && (d!.default_sort = v as D['default_sort'])}
					aria-label="Default order"
				>
					<ToggleGroup.Item value="group">Group</ToggleGroup.Item>
					<ToggleGroup.Item value="name">Name</ToggleGroup.Item>
					<ToggleGroup.Item value="status">Status</ToggleGroup.Item>
				</ToggleGroup.Root>
			</div>
			<label class="flex items-center justify-between gap-3 text-md"
				>New services show on the dashboard<Switch bind:checked={d.new_services_visible} /></label
			>
		</div>
	</Panel>
	<SaveBar {saver} dirty={draft.dirty} onsave={save} />
{/if}
