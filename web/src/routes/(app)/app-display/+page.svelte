<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import type { Endpoint } from '#lib/monitor.ts';
	import AppIconPicker from '#lib/components/hashi/AppIconPicker.svelte';

	// 10b §3: what the public dashboard and status page show. Order follows group and name for now.
	let services = $state<Schemas['Service'][]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let look = $state<Schemas['AppearanceSettings'] | null>(null);
	// Each app's icon on the dashboard (bug 149), by service.
	let icons = $state<Record<string, Schemas['AppIcon']>>({});
	// Where each page is published (bug 64): the services that point at it.
	let pages = $state<Schemas['PublicPages'] | null>(null);
	let error = $state('');
	// Until the lists arrive, counts and empty texts would claim "0 apps" (bug 71).
	let loaded = $state(false);

	async function load() {
		const [s, e, a] = await Promise.all([
			api.GET('/api/services'),
			api.GET('/api/monitor/endpoints'),
			api.GET('/api/settings/appearance')
		]);
		if (!s.data || !e.data || !a.data) throw s.error ?? e.error ?? a.error;
		// Web services only, as on the apps page (bug 139).
		services = s.data.services.filter(
			(x) => x.origin !== 'system' && x.host_name && x.kind !== 'tcp' && x.kind !== 'udp'
		);
		endpoints = e.data.endpoints;
		look = a.data;
		loaded = true;
	}
	onMount(() => {
		load().catch((e) => (error = problemText(e)));
		void api.GET('/api/app-icons').then((r) => {
			for (const a of r.data?.apps ?? []) if (a.service_id) icons[a.service_id] = a;
		});
		void api.GET('/api/app-display/pages').then((r) => (pages = r.data ?? null));
	});

	async function run(p: Promise<{ error?: unknown }>) {
		error = '';
		const r = await p;
		if (r.error) error = problemText(r.error);
		await load().catch(() => {});
	}
	// "All" switches (bug 123): change only the rows that differ, then reload once.
	async function setAll(changes: Promise<{ error?: unknown }>[]) {
		error = '';
		const res = await Promise.all(changes);
		const bad = res.find((r) => r.error);
		if (bad) error = problemText(bad.error);
		await load().catch(() => {});
	}
	const allApps = (visible: boolean) =>
		setAll(
			services
				.filter((r) => r.dashboard_visible !== visible)
				.map((r) =>
					api.PUT('/api/services/{id}/dashboard', {
						params: { path: { id: r.id } },
						body: { visible }
					})
				)
		);
	const allPublic = (pub: boolean) =>
		setAll(endpoints.filter((e) => e.public !== pub).map((e) => putPublic(e, pub)));
	const setApp = (id: string, visible: boolean) =>
		run(api.PUT('/api/services/{id}/dashboard', { params: { path: { id } }, body: { visible } }));
	const putPublic = (e: Endpoint, pub: boolean) =>
		api.PUT('/api/monitor/endpoints/{id}', {
			params: { path: { id: e.id } },
			body: {
				group: e.group,
				paused: e.paused,
				public: pub,
				interval_seconds: e.interval_seconds,
				timeout_seconds: e.timeout_seconds,
				degraded_ms: e.degraded_ms,
				route_provider_id: e.route_provider_id
			}
		});
	const setPublic = (e: Endpoint, pub: boolean) => run(putPublic(e, pub));
</script>

<svelte:head><title>App display · Hashi</title></svelte:head>

<!-- A whole page on or off is a setting (10c §1.2, bug 65); here it's shown with a link there. -->
{#snippet pageState(on: boolean)}
	<a href="/settings?category=public" class="inline-flex"
		><Tag tone={on ? 'ok' : 'neutral'}>{on ? 'on' : 'off'}</Tag></a
	>
{/snippet}

<PageHeader title="App display" />
{#if error}<p role="alert" class="mb-3 text-md text-bad-text">{error}</p>{/if}
{#snippet reached(links: Schemas['PublicPageLink'][] | undefined)}
	{#if links}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-2 text-md">
			{#each links as l (l.service_id)}
				<a href={l.url} target="_blank" rel="noreferrer" class="font-medium hover:underline"
					>{l.url.replace(/^https:\/\/|\/$/g, '')}</a
				>
				<a href="/services/{l.service_id}" class="text-muted-foreground hover:underline"
					><Mono>{l.upstream}</Mono></a
				>
			{:else}
				<span class="text-muted-foreground"
					>No service publishes it yet. <a href="/services" class="underline">Add one</a></span
				>
			{/each}
		</div>
	{/if}
{/snippet}

<div class="grid gap-4 lg:grid-cols-2">
	<Panel
		title="App dashboard"
		description={loaded ? `${services.filter((r) => r.dashboard_visible).length} apps` : 'Loading…'}
		flush
	>
		{#snippet actions()}{#if look}{@render pageState(look.public_dashboard_enabled)}{/if}{/snippet}
		{@render reached(pages?.apps)}
		{#if !loaded}<div class="space-y-2 p-4"><Skeleton class="h-6" /><Skeleton class="h-6" /></div>
		{:else if services.length === 0}<p class="px-4 py-3 text-md text-muted-foreground">
				No services with a public name yet.
			</p>{/if}
		<ul class="divide-y">
			{#if services.length > 1}<li
					class="flex items-center gap-3 bg-surface-2/40 px-4 py-2 text-md font-medium"
				>
					<span class="flex-1">All</span><Switch
						checked={services.every((r) => r.dashboard_visible)}
						onCheckedChange={allApps}
						aria-label="Show all apps"
					/>
				</li>{/if}
			{#each services as r (r.id)}
				{@const icon = icons[r.id]}
				<li class="flex items-center gap-3 px-4 py-2 text-md">
					{#if icon}<AppIconPicker app={icon} onchange={(a) => (icons[r.id] = a)} />{:else}<span
							class="grid size-7 place-items-center rounded-lg border bg-surface-2 text-xs font-semibold"
							>{r.name[0]}</span
						>{/if}
					<span class="min-w-0 flex-1 truncate font-medium">{r.name}</span>
					<Switch
						checked={r.dashboard_visible}
						onCheckedChange={(v) => setApp(r.id, v)}
						aria-label="Show {r.name}"
					/>
				</li>
			{/each}
		</ul>
	</Panel>
	<Panel title="Status page" flush>
		{#snippet actions()}{#if look}{@render pageState(look.public_status_enabled)}{/if}{/snippet}
		{@render reached(pages?.status)}
		{#if !loaded}<div class="space-y-2 p-4">
				<Skeleton class="h-6" /><Skeleton class="h-6" />
			</div>{/if}
		<ul class="divide-y">
			{#if endpoints.length > 1}<li
					class="flex items-center gap-3 bg-surface-2/40 px-4 py-2 text-md font-medium"
				>
					<span class="flex-1">All</span><Switch
						checked={endpoints.every((e) => e.public)}
						onCheckedChange={allPublic}
						aria-label="Show all checks publicly"
					/>
				</li>{/if}
			{#each endpoints as m (m.id)}
				<li class="flex items-center gap-3 px-4 py-2 text-md">
					<span class="min-w-0 flex-1 truncate"
						>{m.name} <span class="text-muted-foreground">· {m.group}</span></span
					>
					<Switch
						checked={m.public}
						onCheckedChange={(v) => setPublic(m, v)}
						aria-label="Show {m.name} publicly"
					/>
				</li>
			{/each}
		</ul>
	</Panel>
</div>
