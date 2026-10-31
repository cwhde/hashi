<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Switch } from '$lib/components/ui/switch';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import type { Endpoint } from '$lib/monitor';

	// 10b §3: what the public dashboard and status page show. Order follows group and name for now.
	let services = $state<Schemas['Service'][]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let look = $state<Schemas['AppearanceSettings'] | null>(null);
	let error = $state('');

	async function load() {
		const [s, e, a] = await Promise.all([
			api.GET('/api/services'),
			api.GET('/api/monitor/endpoints'),
			api.GET('/api/settings/appearance')
		]);
		if (!s.data || !e.data || !a.data) throw s.error ?? e.error ?? a.error;
		services = s.data.services.filter((x) => x.origin !== 'system' && x.host_name);
		endpoints = e.data.endpoints;
		look = a.data;
	}
	onMount(() => {
		load().catch((e) => (error = problemText(e)));
	});

	async function run(p: Promise<{ error?: unknown }>) {
		error = '';
		const r = await p;
		if (r.error) error = problemText(r.error);
		await load().catch(() => {});
	}
	const setApp = (id: string, visible: boolean) =>
		run(api.PUT('/api/services/{id}/dashboard', { params: { path: { id } }, body: { visible } }));
	const setPublic = (e: Endpoint, pub: boolean) =>
		run(
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
			})
		);
	const setPage = (key: 'public_dashboard_enabled' | 'public_status_enabled', v: boolean) =>
		look && run(api.PUT('/api/settings/appearance', { body: { ...look, [key]: v } }));
</script>

<svelte:head><title>App display · Hashi</title></svelte:head>

<PageHeader
	title="App display"
	description="What visitors see on the public app dashboard and status page. Both run on their own ports with no admin access."
/>
{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}
<div class="grid gap-4 lg:grid-cols-2">
	<Panel
		title="App dashboard"
		description="{services.filter((r) => r.dashboard_visible)
			.length} apps · cards link to the public address"
		flush
	>
		{#snippet actions()}{#if look}<Switch
					checked={look.public_dashboard_enabled}
					onCheckedChange={(v) => setPage('public_dashboard_enabled', v)}
					aria-label="Public dashboard enabled"
				/>{/if}{/snippet}
		{#if services.length === 0}<p class="px-4 py-3 text-[13px] text-muted-foreground">
				Services with a public name appear here.
			</p>{/if}
		<ul class="divide-y">
			{#each services as r (r.id)}
				<li class="flex items-center gap-3 px-4 py-2 text-[13px]">
					<span
						class="grid size-7 place-items-center rounded-lg border bg-surface-2 text-[12px] font-semibold"
						>{r.name[0]}</span
					>
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
	<Panel
		title="Status page"
		description="Names, groups, state and history only; never hosts, IPs or errors"
		flush
	>
		{#snippet actions()}{#if look}<Switch
					checked={look.public_status_enabled}
					onCheckedChange={(v) => setPage('public_status_enabled', v)}
					aria-label="Public status enabled"
				/>{/if}{/snippet}
		<ul class="divide-y">
			{#each endpoints as m (m.id)}
				<li class="flex items-center gap-3 px-4 py-2 text-[13px]">
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
