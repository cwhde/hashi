<script lang="ts">
	import { theme } from '#lib/theme.svelte.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	// Settings → Public pages (07 §8.1, 10c §1.2, bug 65): the page-wide options. Each page on or off and where it is
	// published (bug 64), how the apps page starts, and the look (§9). The order, the groups and what shows on each page
	// are the Public pages editor's.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';
	import { SvelteMap } from 'svelte/reactivity';
	import AssetSlots, { slotNames, type Pending, type SlotName } from './AssetSlots.svelte';

	type D = Schemas['DashboardSettings'];
	let pages = $state<{ dashboard: boolean; status: boolean } | null>(null);
	let d = $state<D | null>(null);
	// Where each page is published (bug 64): the services that point at it.
	let links = $state<Schemas['PublicPages'] | null>(null);
	// The public pages' icon, logos and background (07 §9, bug 30).
	let assets = $state<Schemas['PublicAssets'] | null>(null);
	let look = $state<Schemas['PublicAssetsSettings']>({ title: '', source: '', background: false });
	let domain = $state('');
	let pending = $state<Partial<Record<SlotName, Pending>>>({});
	const files = new SvelteMap<SlotName, File>();
	let error = $state('');
	const draft = new Draft(() => [pages, d, look, pending]);
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);
	onMount(async () => {
		void api.GET('/api/app-display/pages').then((r) => (links = r.data ?? null));
		void api.GET('/api/settings/general').then((r) => (domain = r.data?.root_domain ?? ''));
		const [a, b, f] = await Promise.all([
			theme.load(), // shared with the layout's read as the page loads
			api.GET('/api/settings/dashboard'),
			api.GET('/api/public-assets')
		]);
		if (!a.data || !b.data || !f.data) {
			error = problemText(a.error ?? b.error ?? f.error);
			return;
		}
		assets = f.data;
		look = { title: f.data.title, source: f.data.source, background: f.data.background };
		pages = { dashboard: a.data.public_dashboard_enabled, status: a.data.public_status_enabled };
		d = b.data;
		draft.mark();
	});
	// The source first: a file picked from it is read from the saved source.
	async function saveAssets() {
		// The source and background switch save with the first request: a file refused later says so.
		const lookChanged =
			!!assets &&
			((look.title ?? '').trim() !== assets.title ||
				look.source.trim() !== assets.source ||
				look.background !== assets.background);
		const r = await api.PUT('/api/public-assets', {
			body: { ...look, title: (look.title ?? '').trim(), source: look.source.trim() }
		});
		if (!r.data) throw r.error;
		assets = r.data;
		const done: string[] = lookChanged ? ['Look'] : [];
		for (const [slot, p] of Object.entries(pending) as [SlotName, Pending][]) {
			const path = { params: { path: { slot } } };
			const res =
				'remove' in p
					? await api.DELETE('/api/public-assets/slots/{slot}', path)
					: await api.PUT('/api/public-assets/slots/{slot}', {
							...path,
							body: 'path' in p ? { path: p.path } : { data: await base64(files.get(slot)!) }
						});
			if (!res.data) {
				// Say which file failed and what is already saved: the files go first, so nothing else is (bug 171).
				const label = slotNames.find((x) => x.id === slot)!.label;
				const earlier = done.length
					? ` ${done.join(', ')} saved; nothing else changed.`
					: ' Nothing was changed.';
				throw { detail: `${label} not saved: ${problemText(res.error)}.${earlier}` };
			}
			done.push(slotNames.find((x) => x.id === slot)!.label);
			assets = res.data;
			const next = { ...pending };
			delete next[slot];
			pending = next;
			files.delete(slot);
		}
		look = { title: assets.title, source: assets.source, background: assets.background };
	}
	const base64 = (f: File) =>
		new Promise<string>((done, fail) => {
			const r = new FileReader();
			r.onload = () => done(String(r.result).replace(/^data:[^,]*,/, ''));
			r.onerror = () => fail(r.error);
			r.readAsDataURL(f);
		});
	const save = () =>
		saver.run(
			async () => {
				// The files first: one the server refuses then leaves the other settings as they were (bug 171).
				await saveAssets();
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
			{#each [['dashboard', 'Public app dashboard', links?.apps], ['status', 'Public status page', links?.status]] as const as [k, label, at] (k)}
				<div class="rounded-xl border px-3 py-2.5 text-md">
					<label class="flex items-center justify-between gap-3"
						>{label}<Switch bind:checked={pages[k]} /></label
					>
					{#if at}
						<ul class="mt-1.5 space-y-0.5 text-sm">
							{#each at as l (l.service_id)}
								<li class="flex flex-wrap items-baseline gap-x-2">
									<a
										href={l.url}
										target="_blank"
										rel="noreferrer"
										class="font-medium hover:underline">{l.url.replace(/^https:\/\/|\/$/g, '')}</a
									>
									<a href="/services/{l.service_id}" class="text-muted-foreground hover:underline"
										><Mono>{l.upstream}</Mono></a
									>
								</li>
							{:else}
								<li class="text-muted-foreground">
									Not published yet. <a href="/services" class="underline">Add a service</a> that points
									at it.
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/each}
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
	{#if assets}
		<Panel title="Look">
			<AssetSlots bind:look bind:pending saved={assets} {files} {domain} />
		</Panel>
	{/if}
	<SaveBar {saver} dirty={draft.dirty} onsave={save} />
{:else}<Skeleton class="h-40 rounded-2xl" />
{/if}
