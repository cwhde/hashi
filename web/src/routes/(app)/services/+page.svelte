<script lang="ts">
	import ServiceExtras from '$lib/components/hashi/services/ServiceExtras.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Select from '$lib/components/ui/select';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { CircleAlert, Lock, MoreHorizontal, Plus, Search } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { pendingPlans } from '$lib/sync-pending.svelte';
	import {
		domainMode,
		isStream,
		kinds,
		kindText,
		rootDomain,
		saveText,
		slugify,
		targetText,
		type Service
	} from '$lib/services';

	// 04-traefik-services.md §1: what Hashi publishes. Every enabled service goes to every Traefik host. Access rules,
	// WAF, DNS records and monitoring join the same record in later slices.
	let list = $state<Service[]>([]);
	let root = $state('');
	let loaded = $state(false);
	let loadError = $state('');
	let outcome = $state<{ text: string; review: boolean; bad: boolean } | null>(null);
	let busy = $state('');

	let q = $state('');
	let filter = $state('all');
	const rows = $derived(
		list.filter(
			(s) =>
				(filter === 'all' ||
					(filter === 'off' ? !s.enabled || !!s.invalid_reason : s.enabled && !s.invalid_reason)) &&
				(s.name + ' ' + s.host_name).toLowerCase().includes(q.toLowerCase())
		)
	);

	async function load() {
		const [s, r] = await Promise.all([api.GET('/api/services'), rootDomain()]);
		if (s.error || !s.data) throw s.error;
		list = s.data.services;
		root = r;
		loaded = true;
	}
	onMount(() => load().catch((e) => (loadError = problemText(e))));

	async function act(what: string, fn: () => Promise<void>) {
		busy = what;
		outcome = null;
		try {
			await fn();
			await load();
			await pendingPlans.refresh();
		} catch (e) {
			outcome = { text: problemText(e), review: false, bad: true };
		} finally {
			busy = '';
		}
	}

	const toggle = (s: Service) =>
		act('toggle:' + s.id, async () => {
			const path = { params: { path: { id: s.id } } };
			const res = s.enabled
				? await api.POST('/api/services/{id}/disable', path)
				: await api.POST('/api/services/{id}/enable', path);
			if (res.error || !res.data) throw res.error;
			outcome = saveText(res.data);
		});

	// ---- add ----
	let addOpen = $state(false);
	let form = $state({
		name: '',
		slug: '',
		slugTouched: false,
		kind: 'http' as Service['kind'],
		sub: '',
		full: '',
		custom: false,
		address: '',
		setId: '',
		publicPort: '',
		proxy: false,
		entryHost: '',
		port: '',
		dashboard: true,
		monitoring: true,
		adguard: true
	});
	let formError = $state('');
	const host = $derived(
		isStream(form.kind) && !(form.custom ? form.full : form.sub).trim()
			? ''
			: form.custom
				? form.full.trim().toLowerCase()
				: `${form.sub.trim().toLowerCase()}.${root}`
	);

	function openAdd() {
		form = {
			name: '',
			slug: '',
			slugTouched: false,
			kind: 'http',
			sub: '',
			full: '',
			custom: false,
			address: '',
			setId: '',
			publicPort: '',
			proxy: false,
			entryHost: '',
			port: '',
			dashboard: true,
			monitoring: true,
			adguard: true
		};
		formError = '';
		addOpen = true;
	}

	async function add() {
		formError = '';
		busy = 'add';
		try {
			const res = await api.POST('/api/services', {
				body: {
					name: form.name.trim(),
					slug: form.slug || slugify(form.name),
					kind: form.kind,
					domain_mode: domainMode(host, root),
					host_name: host,
					target: form.setId
						? { address: '', set_id: form.setId, port: Number(form.port) }
						: { address: form.address.trim(), port: Number(form.port) },
					public_port: isStream(form.kind) ? Number(form.publicPort) : undefined,
					proxy_protocol: form.kind === 'tcp' ? form.proxy : undefined,
					entry_host_id: form.entryHost || undefined,
					dashboard_visible: form.dashboard,
					monitoring_visible: form.monitoring,
					adguard_rewrite: form.adguard
				}
			});
			if (res.error || !res.data) throw res.error;
			addOpen = false;
			outcome = saveText(res.data);
			await load();
			await pendingPlans.refresh();
		} catch (e) {
			formError = problemText(e);
		} finally {
			busy = '';
		}
	}
</script>

<PageHeader
	title="Services"
	description="What Hashi publishes. Every enabled service is served by every Traefik host with Hashi's certificate."
>
	{#snippet actions()}
		<Button onclick={openAdd} disabled={!loaded}><Plus />Add service</Button>
	{/snippet}
</PageHeader>

{#if outcome}
	<p
		role={outcome.bad ? 'alert' : 'status'}
		class="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-3 text-[13px] {outcome.bad
			? 'border-bad/30 bg-bad/10 text-bad-text'
			: 'bg-card'}"
	>
		{#if outcome.bad}<CircleAlert class="size-4 shrink-0" />{/if}{outcome.text}
		{#if outcome.review}<a href="/changes" class="font-medium underline underline-offset-2"
				>Review</a
			>{/if}
	</p>
{/if}

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !loaded}
	<Skeleton class="h-80 rounded-2xl" />
{:else}
	<div class="rounded-2xl border bg-card">
		<div class="flex flex-wrap items-center gap-2 border-b px-3 py-2.5">
			<div class="relative w-full max-w-72">
				<Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					bind:value={q}
					placeholder="Filter by name or domain"
					class="pl-8"
					aria-label="Filter services"
				/>
			</div>
			<ToggleGroup.Root type="single" bind:value={filter} variant="outline" size="sm">
				<ToggleGroup.Item value="all">All</ToggleGroup.Item>
				<ToggleGroup.Item value="on">Published</ToggleGroup.Item>
				<ToggleGroup.Item value="off">Off or invalid</ToggleGroup.Item>
			</ToggleGroup.Root>
			<span class="ml-auto text-[12.5px] text-muted-foreground">{rows.length} of {list.length}</span
			>
		</div>
		{#if list.length === 0}
			<p class="px-4 py-10 text-center text-[13.5px] text-muted-foreground">
				No services yet. Add the first one; Hashi publishes it on every Traefik host.
			</p>
		{:else}
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head class="pl-4">Service</Table.Head>
						<Table.Head class="hidden md:table-cell">Upstream</Table.Head>
						<Table.Head>State</Table.Head>
						<Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each rows as s (s.id)}
						<Table.Row class={s.enabled ? '' : 'opacity-60'}>
							<Table.Cell class="pl-4">
								<a href="/services/{s.id}" class="group block">
									<span class="flex items-center gap-2 font-medium group-hover:underline">
										{s.name}
										{#if s.origin === 'system'}<Tag><Lock class="size-3" />system</Tag>{/if}
									</span>
									<span class="text-[12.5px] text-muted-foreground"
										>{s.host_name || `port ${s.public_port}/${s.kind}`}</span
									>
								</a>
							</Table.Cell>
							<Table.Cell class="hidden md:table-cell">
								<div class="flex items-center gap-2">
									<Tag>{s.kind.toUpperCase()}</Tag><Mono class="text-muted-foreground"
										>{targetText(s.target)}</Mono
									>
								</div>
								{#if s.advanced_routes && s.routes.length}<div
										class="mt-0.5 text-[12px] text-muted-foreground"
									>
										+ {s.routes.length} route{s.routes.length === 1 ? '' : 's'}
									</div>{/if}
							</Table.Cell>
							<Table.Cell class="text-[13px]">
								{#if s.invalid_reason}<span class="text-bad-text" title={s.invalid_reason}
										>Not published: invalid</span
									>{:else if s.enabled}Published{:else}<span class="text-muted-foreground">Off</span
									>{/if}
							</Table.Cell>
							<Table.Cell>
								<DropdownMenu.Root>
									<DropdownMenu.Trigger>
										{#snippet child({ props })}<Button
												{...props}
												variant="ghost"
												size="icon-sm"
												aria-label="Actions for {s.name}"><MoreHorizontal /></Button
											>{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end">
										<DropdownMenu.Item onclick={() => goto('/services/' + s.id)}
											>Open</DropdownMenu.Item
										>
										<DropdownMenu.Item
											disabled={s.origin === 'system' || !!busy}
											onclick={() => toggle(s)}
											>{s.enabled ? 'Turn off' : 'Turn on'}</DropdownMenu.Item
										>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		{/if}
	</div>
{/if}

<!-- Larger forms open in a drawer, the list stays visible (10-ui.md §1). -->
<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Add service</Sheet.Title>
			<Sheet.Description
				>Saving publishes it on every Traefik host. Routes, rewrites and TLS options are on the
				service page.</Sheet.Description
			>
		</Sheet.Header>
		<form
			id="add-service"
			class="space-y-5 overflow-y-auto px-4 pb-4"
			onsubmit={(e) => (e.preventDefault(), add())}
		>
			<div class="grid gap-4 sm:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="sn">Name</Label>
					<Input
						id="sn"
						bind:value={form.name}
						oninput={() => !form.slugTouched && (form.slug = slugify(form.name))}
						required
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="ss">Short name</Label>
					<Input
						id="ss"
						bind:value={form.slug}
						oninput={() => (form.slugTouched = true)}
						class="font-mono"
						required
					/>
				</div>
			</div>
			<div class="space-y-1.5">
				<div class="flex items-center justify-between gap-3">
					<Label for={form.custom ? 'sf' : 'sd'}
						>Public domain{isStream(form.kind) ? ' (optional)' : ''}</Label
					>
					<label class="flex items-center gap-2 text-[12.5px] text-muted-foreground"
						>Another name in the zone <Switch bind:checked={form.custom} /></label
					>
				</div>
				{#if form.custom}
					<Input
						id="sf"
						bind:value={form.full}
						placeholder="app.{root}"
						required={!isStream(form.kind)}
					/>
				{:else}
					<div class="flex items-center gap-2">
						<Input
							id="sd"
							bind:value={form.sub}
							class="max-w-56"
							placeholder="app"
							required={!isStream(form.kind)}
						/><span class="text-[13px] text-muted-foreground">.{root}</span>
					</div>
				{/if}
				<p class="text-[12.5px] text-muted-foreground">
					Names a level deeper (like <Mono>dev.hashi.{root}</Mono>) get their own wildcard
					certificate automatically.
				</p>
			</div>
			<fieldset class="space-y-2">
				<legend class="mb-1.5 text-[13px] font-medium">Upstream</legend>
				<div class="grid grid-cols-[minmax(0,10rem)_1fr_6rem] gap-2">
					<Select.Root type="single" bind:value={form.kind}>
						<Select.Trigger aria-label="Protocol">{form.kind.toUpperCase()}</Select.Trigger>
						<Select.Content>
							{#each kinds as k (k)}<Select.Item value={k}>{kindText[k]}</Select.Item>{/each}
						</Select.Content>
					</Select.Root>
					<Input
						bind:value={form.address}
						aria-label="Address"
						placeholder="192.0.2.20"
						required={!form.setId}
						disabled={!!form.setId}
					/>
					<Input
						bind:value={form.port}
						aria-label="Port"
						inputmode="numeric"
						placeholder="8080"
						required
					/>
				</div>
				<p class="text-[12.5px] text-muted-foreground">
					An IP address or host name the Traefik hosts can reach.
				</p>
			</fieldset>
			<ServiceExtras
				kind={form.kind}
				bind:setId={form.setId}
				bind:publicPort={form.publicPort}
				bind:proxy={form.proxy}
				bind:entryHost={form.entryHost}
			/>
			<div class="space-y-3 rounded-xl border p-3">
				<div class="flex items-center justify-between gap-3">
					<Label for="f1">Show on the app dashboard</Label><Switch
						id="f1"
						bind:checked={form.dashboard}
					/>
				</div>
				<div class="flex items-center justify-between gap-3">
					<Label for="f2">Monitor</Label><Switch id="f2" bind:checked={form.monitoring} />
				</div>
				<div class="flex items-center justify-between gap-3">
					<Label for="f3">Internal DNS rewrite (AdGuard)</Label><Switch
						id="f3"
						bind:checked={form.adguard}
					/>
				</div>
				<p class="text-[12.5px] text-muted-foreground">
					Access rules arrive later; until then Hashi's forward auth runs in front of every service
					and lets requests through.
				</p>
			</div>
			{#if formError}<p role="alert" class="text-[13px] text-bad-text">{formError}</p>{/if}
		</form>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button type="submit" form="add-service" disabled={busy === 'add'}
				>{busy === 'add' ? 'Publishing…' : 'Save and publish'}</Button
			>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
