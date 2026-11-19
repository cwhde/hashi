<script lang="ts">
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	// Settings → Public pages (10c §1.2, bug 65): the two public pages on or off, how the dashboard starts, and the status
	// page's categories (07 §7, bug 138). Which service shows on them is an instant switch on App display.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { ArrowDown, ArrowUp } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';

	type D = Schemas['DashboardSettings'];
	let pages = $state<{ dashboard: boolean; status: boolean } | null>(null);
	let d = $state<D | null>(null);
	let sp = $state<Schemas['StatusPageSettings'] | null>(null);
	let cats = $state<string[]>([]); // categories of the public checks, in the order set
	let error = $state('');
	const draft = new Draft(() => [pages, d, sp, cats]);
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);
	onMount(async () => {
		const [a, b, c, e] = await Promise.all([
			api.GET('/api/settings/appearance'),
			api.GET('/api/settings/dashboard'),
			api.GET('/api/settings/status-page'),
			api.GET('/api/monitor/endpoints')
		]);
		if (!a.data || !b.data || !c.data || !e.data) {
			error = problemText(a.error ?? b.error ?? c.error ?? e.error);
			return;
		}
		pages = { dashboard: a.data.public_dashboard_enabled, status: a.data.public_status_enabled };
		d = b.data;
		sp = c.data;
		const found = new Set(e.data.endpoints.filter((x) => x.public).map((x) => x.group));
		cats = [
			...c.data.order.filter((g) => found.has(g)),
			...[...found].filter((g) => !c.data.order.includes(g)).sort((x, y) => x.localeCompare(y))
		];
		draft.mark();
	});
	function move(i: number, by: number) {
		const next = [...cats];
		next.splice(i + by, 0, ...next.splice(i, 1));
		cats = next;
	}
	function show(g: string, on: boolean) {
		sp!.hidden = on ? sp!.hidden.filter((x) => x !== g) : [...sp!.hidden, g];
	}
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
				const keep = sp!.order.filter((g) => !cats.includes(g)); // categories with no public check now
				const s = await api.PUT('/api/settings/status-page', {
					body: { ...sp!, order: [...cats, ...keep] }
				});
				if (!s.data) throw s.error;
				sp = s.data;
			},
			'Saved.',
			draft
		);
</script>

{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>
{:else if pages && d && sp}
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
	<Panel title="Status page">
		<div class="space-y-3 text-md">
			<label class="flex items-center justify-between gap-3"
				>Show categories<Switch bind:checked={sp.grouped} /></label
			>
			{#if cats.length === 0}
				<p class="text-muted-foreground">No checks are on the status page yet.</p>
			{:else}
				<ul class="divide-y rounded-xl border">
					{#each cats as g, i (g)}
						<li class="flex items-center gap-2 px-3 py-1.5">
							<span class="min-w-0 flex-1 truncate">{g}</span>
							{#if sp.grouped}
								<Button
									variant="ghost"
									size="icon-sm"
									disabled={i === 0}
									onclick={() => move(i, -1)}
									aria-label="Move {g} up"><ArrowUp /></Button
								>
								<Button
									variant="ghost"
									size="icon-sm"
									disabled={i === cats.length - 1}
									onclick={() => move(i, 1)}
									aria-label="Move {g} down"><ArrowDown /></Button
								>
							{/if}
							<Switch
								checked={!sp.hidden.includes(g)}
								onCheckedChange={(on) => show(g, on)}
								aria-label="Show {g}"
							/>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</Panel>
	<SaveBar {saver} dirty={draft.dirty} onsave={save} />
{:else}<Skeleton class="h-40 rounded-2xl" />
{/if}
