<script lang="ts">
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { hrefWithTab, tabFrom } from '#lib/url-tab.ts';
	import ServiceExtras from '#lib/components/hashi/services/ServiceExtras.svelte';
	import MoreOptions from '#lib/components/hashi/MoreOptions.svelte';
	import ServiceAccess from '#lib/components/hashi/rules/ServiceAccess.svelte';
	import ServiceWaf from '#lib/components/hashi/services/ServiceWaf.svelte';
	import ServiceTraffic from '#lib/components/hashi/services/ServiceTraffic.svelte';
	import MiddlewarePicker from '#lib/components/hashi/services/MiddlewarePicker.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import StatusDot from '#lib/components/hashi/StatusDot.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as Tabs from '#lib/components/ui/tabs/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.ts';
	import { ChevronLeft, CircleAlert, Lock, Plus, Trash2 } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { pendingPlans } from '#lib/sync-pending.svelte.ts';
	import {
		domainMode,
		isStream,
		kinds,
		kindText,
		newRoute,
		parseTargets,
		rewriteText,
		rootDomain,
		saveText,
		targetText,
		toInput,
		type Route,
		type Service,
		type ServiceInput,
		upstreamOf,
		upstreamTarget
	} from '#lib/services.ts';
	import UpstreamTarget from '#lib/components/hashi/services/UpstreamTarget.svelte';
	import { targetNames } from '#lib/targetNames.svelte.ts';

	// One service (04 §1): public name, upstream, advanced routes with rewrites, TLS options, the rendered excerpt.
	// The Hashi admin service is shown read-only: it follows Settings → General (invariant 10).
	const id = $derived(page.params.id ?? '');
	let svc = $state<Service | null>(null);
	// Where it is published and why a part is missing (bug 20).
	let pub = $state<Schemas['ServicePublication'] | null>(null);
	let edit = $state<ServiceInput | null>(null);
	let targetsText = $state<string[]>([]);
	let extra = $state({ upstream: upstreamOf(), publicPort: '', proxy: false, entryHost: '' });
	let root = $state('');
	let yaml = $state('');
	// The checks Status runs for this service (bug 61), shown in the summary with a link to each.
	let checks = $state<Schemas['MonitorEndpoint'][]>([]);
	// Names from the user middleware file and the ones on every service (04 §4, 09 §2).
	let mwNames = $state<string[]>([]);
	let mwEvery = $state<string[]>([]);
	let loadError = $state('');
	let saveError = $state('');
	let outcome = $state<{ text: string; review: boolean; bad: boolean } | null>(null);
	let busy = $state('');
	let delOpen = $state(false);
	const system = $derived(svc?.origin === 'system');
	// Unsaved edits: leaving the page asks first (bug 40). A save reloads, which marks the new state saved.
	const draft = new Draft(() => [edit, extra, targetsText]);
	// The WAF tab keeps its own draft and saves with this page's Save (bug 65).
	let waf = $state<ReturnType<typeof ServiceWaf>>();
	const wafDirty = $derived(!!waf?.draft.dirty);
	guardUnsaved(() => draft.dirty || wafDirty);

	async function load() {
		const [s, r] = await Promise.all([
			api.GET('/api/services/{id}', { params: { path: { id } } }),
			rootDomain(),
			targetNames.load()
		]);
		if (s.error || !s.data) throw s.error;
		svc = s.data;
		root = r;
		void api
			.GET('/api/services/{id}/publication', { params: { path: { id } } })
			.then((p) => (pub = p.data ?? null));
		void api
			.GET('/api/monitor/endpoints')
			.then((m) => (checks = (m.data?.endpoints ?? []).filter((e) => e.source_id === id)));
		edit = toInput(s.data);
		extra = {
			upstream: upstreamOf(s.data.target),
			publicPort: String(s.data.public_port ?? ''),
			proxy: !!s.data.proxy_protocol,
			entryHost: s.data.entry_host_id ?? ''
		};
		targetsText = s.data.routes.map((r) =>
			r.targets.map((t) => targetText(t, targetNames.sets)).join(', ')
		);
		const mw = await api.GET('/api/traefik/user-middlewares');
		if (mw.data) {
			mwNames = mw.data.middlewares.map((m) => m.name);
			mwEvery = mw.data.every_service;
		}
		yaml = '';
		if (!s.data.invalid_reason) {
			const e = await api.GET('/api/services/{id}/effective', { params: { path: { id } } });
			if (e.data) yaml = e.data.yaml;
		}
		draft.mark();
	}
	onMount(() => load().catch((e) => (loadError = problemText(e))));

	async function act(what: string, fn: () => Promise<void>) {
		busy = what;
		outcome = null;
		saveError = '';
		try {
			await fn();
			await load();
			await pendingPlans.refresh();
		} catch (e) {
			saveError = problemText(e);
		} finally {
			busy = '';
		}
	}

	const save = () =>
		act('save', async () => {
			if (!edit) return;
			const routes: Route[] = [];
			for (const [i, r] of (edit.routes ?? []).entries()) {
				const p = parseTargets(targetsText[i] ?? '', targetNames.sets);
				if (p.bad) throw new Error(`Route ${r.path_value}: "${p.bad}" is not address:port.`);
				routes.push({ ...r, targets: p.targets });
			}
			// Header rows left completely empty are dropped; a name without a value removes that header.
			const rows = (hs?: { name: string; value: string }[]) =>
				(hs ?? [])
					.map((h) => ({ name: h.name.trim(), value: h.value }))
					.filter((h) => h.name || h.value);
			const traffic = isStream(edit.kind)
				? {}
				: {
						...edit.traffic,
						host_value:
							edit.traffic?.host === 'custom' ? edit.traffic.host_value?.trim() : undefined,
						request_headers: rows(edit.traffic?.request_headers),
						response_headers: rows(edit.traffic?.response_headers)
					};
			const body: ServiceInput = {
				...edit,
				host_name: edit.host_name.trim().toLowerCase(),
				domain_mode: domainMode(edit.host_name.trim().toLowerCase(), root),
				target: upstreamTarget(extra.upstream, Number(edit.target.port)),
				routes: isStream(edit.kind) ? [] : routes,
				traffic,
				middlewares: isStream(edit.kind) ? [] : (edit.middlewares ?? []),
				public_port: isStream(edit.kind) ? Number(extra.publicPort) : undefined,
				proxy_protocol: edit.kind === 'tcp' ? extra.proxy : undefined,
				entry_host_id: extra.entryHost || undefined
			};
			if (isStream(body.kind)) {
				body.advanced_routes = false;
				body.tls_min_version = '';
			}
			// Upstream TLS options belong to https only; their fields are hidden for the other kinds.
			if (body.kind !== 'https') {
				body.tls_skip_verify = false;
				body.tls_server_name = '';
			}
			const res = await api.PUT('/api/services/{id}', { params: { path: { id } }, body });
			if (res.error || !res.data) throw res.error;
			const wafPlans = wafDirty && waf ? await waf.save() : [];
			outcome = saveText({ ...res.data, plans: [...res.data.plans, ...wafPlans] });
		});

	const toggle = () =>
		act('toggle', async () => {
			const path = { params: { path: { id } } };
			const res = svc?.enabled
				? await api.POST('/api/services/{id}/disable', path)
				: await api.POST('/api/services/{id}/enable', path);
			if (res.error || !res.data) throw res.error;
			outcome = saveText(res.data);
		});

	async function remove() {
		busy = 'delete';
		try {
			await withReauth(() => api.DELETE('/api/services/{id}', { params: { path: { id } } }));
			await pendingPlans.refresh();
			goto('/services');
		} catch (e) {
			saveError = problemText(e);
			delOpen = false;
		} finally {
			busy = '';
		}
	}

	function addRoute() {
		if (!edit) return;
		edit.routes = [...(edit.routes ?? []), newRoute(edit.routes ?? [], edit.target)];
		const first = upstreamTarget(extra.upstream, Number(edit.target.port));
		targetsText = [...targetsText, first.agent_id ? '' : targetText(first, targetNames.sets)];
	}
	function dropRoute(i: number) {
		if (!edit?.routes) return;
		edit.routes = edit.routes.filter((_, j) => j !== i);
		targetsText = targetsText.filter((_, j) => j !== i);
	}
	const matchText = { prefix: 'starts with', exact: 'is exactly', regex: 'matches regex' } as const;
	// The open tab lives in the address (bug 41).
	const tabs = ['routing', 'traffic', 'access', 'waf', 'config'];
	let tab = $state(tabFrom(page.url, tabs));
	const setTab = (t: string) =>
		goto(hrefWithTab(page.url, tabs, t), { replace: true, reset: false });
</script>

<svelte:head><title>{svc?.name ?? 'Service'} · Hashi</title></svelte:head>

<a
	href="/services"
	class="mb-3 inline-flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Services</a
>

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-md text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !svc || !edit}
	<Skeleton class="h-28 rounded-2xl" /><Skeleton class="mt-4 h-72 rounded-2xl" />
{:else}
	<div class="mb-5 rounded-2xl border bg-card p-4">
		<!-- On a phone the buttons go under the title instead of squeezing it (bug 15). -->
		<div class="flex flex-col gap-x-6 gap-y-3 sm:flex-row sm:items-start">
			<div class="min-w-0 flex-1">
				<h1 class="flex flex-wrap items-center gap-2 text-title leading-tight font-semibold">
					{svc.name}
					{#if system}<Tag><Lock class="size-3" />system · change it in Settings → General</Tag
						>{/if}
				</h1>
				{#if isStream(svc.kind)}
					<p class="text-md text-muted-foreground">
						Port {svc.public_port}/{svc.kind}{svc.host_name ? ` · ${svc.host_name}` : ''}
					</p>
				{:else}
					<a class="text-md text-muted-foreground hover:underline" href="https://{svc.host_name}"
						>https://{svc.host_name}</a
					>
				{/if}
			</div>
			{#if !system}
				<div class="flex flex-wrap gap-2">
					<Button variant="outline" size="sm" onclick={toggle} disabled={!!busy}
						>{svc.enabled ? 'Turn off' : 'Turn on'}</Button
					>
					<Button
						variant="ghost"
						size="sm"
						class="text-bad-text"
						onclick={() => (delOpen = true)}
						disabled={!!busy}><Trash2 />Delete</Button
					>
					<Button size="sm" onclick={save} disabled={!!busy || !(draft.dirty || wafDirty)}
						>{busy === 'save' ? 'Publishing…' : 'Save and publish'}</Button
					>
				</div>
			{/if}
		</div>
		<dl class="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 text-md sm:grid-cols-3 lg:grid-cols-5">
			<div>
				<dt class="text-xs text-muted-foreground">State</dt>
				<dd>
					{#if svc.invalid_reason}<span class="text-bad-text">Not published</span
						>{:else if svc.enabled}Published on every Traefik host{:else}Off{/if}
				</dd>
			</div>
			<div class="col-span-2 min-w-0 sm:col-span-1">
				<dt class="text-xs text-muted-foreground">Upstream</dt>
				<dd class="overflow-x-auto whitespace-nowrap">
					<Mono>{svc.kind}://{targetNames.label(svc.target)}</Mono>
				</dd>
			</div>
			<div>
				<dt class="text-xs text-muted-foreground">Routes</dt>
				<dd>{svc.advanced_routes ? svc.routes.length : 'whole service to one upstream'}</dd>
			</div>
			<div class="min-w-0">
				<dt class="text-xs text-muted-foreground">Checks</dt>
				<dd class="truncate">
					{#each checks as c, i (c.id)}{i ? ', ' : ''}<a
							href="/status/{c.id}"
							class="inline-flex items-center gap-1.5 hover:underline"
							><StatusDot state={c.state} />{c.name}</a
						>{:else}<span class="text-muted-foreground">none</span>{/each}
				</dd>
			</div>
			<div class="min-w-0">
				<dt class="text-xs text-muted-foreground">Behind</dt>
				<dd class="truncate">
					{#if pub?.host_id}<a href="/firewall/{pub.host_id}" class="hover:underline"
							>{pub.host_name}</a
						>{#if pub.rewrite}<span class="text-muted-foreground">
								· home <Mono>{pub.rewrite}</Mono></span
							>{/if}{:else}<span class="text-muted-foreground">no firewall host</span>{/if}
				</dd>
			</div>
		</dl>
		{#if svc.invalid_reason}
			<p class="mt-3 rounded-xl border border-bad/25 bg-bad/10 p-3 text-sm text-bad-text">
				Left out of the Traefik files: {svc.invalid_reason}
			</p>
		{:else if pub?.reason && svc.enabled}
			<p class="mt-3 rounded-xl border border-warn/25 bg-warn/10 p-3 text-sm text-warn-text">
				{pub.reason}
			</p>
		{/if}
	</div>

	{#if outcome}
		<p
			role={outcome.bad ? 'alert' : 'status'}
			class="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-3 text-md {outcome.bad
				? 'border-bad/30 bg-bad/10 text-bad-text'
				: 'bg-card'}"
		>
			{outcome.text}
			{#if outcome.review}<a href="/changes" class="font-medium underline underline-offset-2"
					>Review</a
				>{/if}
		</p>
	{/if}
	{#if saveError}
		<p
			role="alert"
			class="mb-4 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-3 text-md text-bad-text"
		>
			<CircleAlert class="mt-0.5 size-4 shrink-0" />{saveError}
		</p>
	{/if}

	<Tabs.Root bind:value={tab} onValueChange={setTab}>
		<!-- Six tabs are wider than a phone: the row scrolls on its own instead of the page. -->
		<Tabs.List class="mb-4 max-w-full justify-start overflow-x-auto">
			<Tabs.Trigger value="routing">Routing</Tabs.Trigger>
			{#if !isStream(svc.kind) && !system}<Tabs.Trigger value="traffic">Traffic</Tabs.Trigger>{/if}
			{#if !isStream(svc.kind)}<Tabs.Trigger value="access">Access</Tabs.Trigger>
				<Tabs.Trigger value="waf">WAF</Tabs.Trigger>{/if}
			<Tabs.Trigger value="config">Rendered config</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="routing" class="grid gap-4 lg:grid-cols-2">
			<Panel title="Public address">
				<fieldset class="space-y-4" disabled={system}>
					<div class="space-y-1.5">
						<Label for="n">Name</Label><Input id="n" bind:value={edit.name} />
					</div>
					<div class="space-y-1.5">
						<Label for="h">Domain</Label><Input id="h" bind:value={edit.host_name} />
					</div>
					<MoreOptions changed={edit.tls_min_version ? 1 : 0}>
						<div class="space-y-1.5">
							<Label for="sl">Short name</Label><Input
								id="sl"
								bind:value={edit.slug}
								class="font-mono"
							/>
						</div>
						<div class="space-y-1.5">
							<Label>Minimum TLS version for visitors</Label>
							<Select.Root type="single" bind:value={edit.tls_min_version}>
								<Select.Trigger class="w-full"
									>{edit.tls_min_version === 'VersionTLS13'
										? 'TLS 1.3'
										: edit.tls_min_version === 'VersionTLS12'
											? 'TLS 1.2'
											: 'Traefik default'}</Select.Trigger
								>
								<Select.Content>
									<Select.Item value="">Traefik default</Select.Item>
									<Select.Item value="VersionTLS12">TLS 1.2</Select.Item>
									<Select.Item value="VersionTLS13">TLS 1.3</Select.Item>
								</Select.Content>
							</Select.Root>
						</div>
					</MoreOptions>
					<div class="space-y-3 rounded-xl border p-3">
						<div class="flex items-center justify-between gap-3">
							<Label for="d1">Show on the app dashboard</Label><Switch
								id="d1"
								bind:checked={edit.dashboard_visible}
							/>
						</div>
						<div class="flex items-center justify-between gap-3">
							<Label for="d2">Monitor</Label><Switch
								id="d2"
								bind:checked={edit.monitoring_visible}
							/>
						</div>
						<div class="flex items-center justify-between gap-3">
							<Label for="d3">Internal DNS rewrite (AdGuard)</Label><Switch
								id="d3"
								bind:checked={edit.adguard_rewrite}
							/>
						</div>
					</div>
				</fieldset>
			</Panel>

			<Panel title="Upstream">
				{#snippet actions()}{#if !system}<div class="flex items-center gap-2 text-md">
							<Label for="adv">Advanced routes</Label><Switch
								id="adv"
								bind:checked={edit!.advanced_routes}
							/>
						</div>{/if}{/snippet}
				<fieldset class="space-y-3" disabled={system}>
					<div class="grid grid-cols-[minmax(0,1fr)_6rem] gap-2">
						<Select.Root type="single" bind:value={edit.kind}>
							<Select.Trigger aria-label="Protocol">{kindText[edit.kind]}</Select.Trigger>
							<Select.Content>
								{#each kinds as k (k)}<Select.Item value={k}>{kindText[k]}</Select.Item>{/each}
							</Select.Content>
						</Select.Root>
						<Input bind:value={edit.target.port} aria-label="Port" inputmode="numeric" />
					</div>
					<UpstreamTarget bind:value={extra.upstream} id="svc-upstream" />
					<ServiceExtras
						kind={edit.kind}
						bind:publicPort={extra.publicPort}
						bind:proxy={extra.proxy}
						bind:entryHost={extra.entryHost}
					/>
					{#if edit.kind === 'https'}
						<MoreOptions
							label="Upstream certificate"
							changed={(edit.tls_server_name ? 1 : 0) + (edit.tls_skip_verify ? 1 : 0)}
						>
							<div class="space-y-1.5">
								<Label for="sni">Upstream certificate name</Label>
								<Input id="sni" bind:value={edit.tls_server_name} placeholder="optional" />
							</div>
							<div class="flex items-center justify-between gap-3">
								<Label for="skip">Accept any upstream certificate</Label><Switch
									id="skip"
									bind:checked={edit.tls_skip_verify}
								/>
							</div>
						</MoreOptions>
					{/if}
					{#if edit.advanced_routes && !isStream(edit.kind)}
						<ul class="space-y-3" aria-label="Routes">
							{#each edit.routes ?? [] as r, i (r.id ?? i)}
								<li class="space-y-2 rounded-xl border p-3">
									<div class="flex flex-wrap items-center gap-2">
										<Switch bind:checked={r.enabled} aria-label="Route on" />
										<Select.Root
											type="single"
											value={r.kind || 'same'}
											onValueChange={(v) => (r.kind = v === 'same' ? '' : (v as Route['kind']))}
										>
											<Select.Trigger class="w-36" aria-label="Upstream protocol"
												>{r.kind || 'Same as service'}</Select.Trigger
											>
											<Select.Content>
												<Select.Item value="same">Same as service</Select.Item>
												<Select.Item value="http">http</Select.Item>
												<Select.Item value="https">https</Select.Item>
												<Select.Item value="h2c">h2c (gRPC)</Select.Item>
											</Select.Content>
										</Select.Root>
										<Select.Root type="single" bind:value={r.path_match}>
											<Select.Trigger class="w-36" aria-label="Path match"
												>{matchText[r.path_match]}</Select.Trigger
											>
											<Select.Content>
												<Select.Item value="prefix">starts with</Select.Item>
												<Select.Item value="exact">is exactly</Select.Item>
												<Select.Item value="regex">matches regex</Select.Item>
											</Select.Content>
										</Select.Root>
										<Input
											bind:value={r.path_value}
											class="w-40 flex-1 font-mono"
											aria-label="Path"
										/>
										<Input
											bind:value={r.priority}
											type="number"
											min="1"
											max="9999"
											class="w-20"
											aria-label="Priority"
										/>
										<Button
											variant="ghost"
											size="icon-sm"
											aria-label="Remove route"
											onclick={() => dropRoute(i)}><Trash2 /></Button
										>
									</div>
									<Input
										bind:value={targetsText[i]}
										class="font-mono"
										aria-label="Route targets"
										placeholder="192.0.2.21:9000, 192.0.2.22:9000*2"
									/>
									{#if mwNames.length || r.middlewares?.length}
										<div class="flex flex-wrap items-center gap-2">
											<span class="text-sm text-muted-foreground">Extra middlewares</span>
											<MiddlewarePicker
												names={mwNames}
												bind:selected={r.middlewares}
												label="Route middlewares"
											/>
										</div>
									{/if}
									<div class="flex flex-wrap items-center gap-2">
										<Select.Root type="single" bind:value={r.rewrite_mode}>
											<Select.Trigger class="w-40" aria-label="Rewrite"
												>{rewriteText[r.rewrite_mode]}</Select.Trigger
											>
											<Select.Content>
												{#each Object.entries(rewriteText) as [k, v] (k)}<Select.Item value={k}
														>{v}</Select.Item
													>{/each}
											</Select.Content>
										</Select.Root>
										{#if r.rewrite_mode === 'regex'}
											<Input
												bind:value={r.rewrite_pattern}
												class="w-40 flex-1 font-mono"
												aria-label="Rewrite pattern"
												placeholder="^/u/([0-9]+)"
											/>
										{/if}
										{#if r.rewrite_mode === 'replace_prefix' || r.rewrite_mode === 'exact' || r.rewrite_mode === 'regex'}
											<Input
												bind:value={r.rewrite_value}
												class="w-40 flex-1 font-mono"
												aria-label="Rewrite to"
												placeholder={r.rewrite_mode === 'regex' ? '/user/$1' : '/new'}
											/>
										{/if}
									</div>
								</li>
							{/each}
						</ul>
						<Button variant="ghost" size="sm" onclick={addRoute}><Plus />Add route</Button>
					{/if}
				</fieldset>
			</Panel>
		</Tabs.Content>

		{#if !isStream(svc.kind) && !system && edit.traffic}
			<Tabs.Content value="traffic">
				<ServiceTraffic
					bind:traffic={edit.traffic}
					bind:middlewares={edit.middlewares}
					names={mwNames}
					every={mwEvery}
					publicName={edit.host_name}
					upstream={targetNames.label(upstreamTarget(extra.upstream, Number(edit.target.port)))}
				/>
			</Tabs.Content>
		{/if}

		<Tabs.Content value="access">
			{#if !isStream(svc.kind)}<ServiceAccess serviceId={id} />{/if}
		</Tabs.Content>

		<Tabs.Content value="waf"><ServiceWaf serviceId={id} bind:this={waf} /></Tabs.Content>

		<Tabs.Content value="config">
			<Panel
				title={isStream(svc.kind) ? '20-hashi-stream-services.yml' : '10-hashi-http-services.yml'}
				description={svc.enabled ? 'On every Traefik host' : 'Off: not on any Traefik host'}
				flush
			>
				{#if yaml}
					<pre
						class="overflow-x-auto rounded-b-2xl bg-surface-2/40 p-4 font-mono text-sm leading-relaxed"><code
							>{yaml}</code
						></pre>
				{:else}
					<p class="px-4 py-3 text-md text-muted-foreground">
						Nothing is rendered while the service is invalid.
					</p>
				{/if}
			</Panel>
		</Tabs.Content>
	</Tabs.Root>

	<AlertDialog.Root bind:open={delOpen}>
		<AlertDialog.Content>
			<AlertDialog.Header>
				<AlertDialog.Title>Delete {svc.name}?</AlertDialog.Title>
				<AlertDialog.Description
					>It disappears from every Traefik host right away. Hashi asks for your passkey to confirm.</AlertDialog.Description
				>
			</AlertDialog.Header>
			<AlertDialog.Footer>
				<AlertDialog.Cancel>Keep it</AlertDialog.Cancel>
				<AlertDialog.Action
					class="border border-bad/30 bg-bad/15 text-bad-text hover:bg-bad/25"
					onclick={remove}>Delete service</AlertDialog.Action
				>
			</AlertDialog.Footer>
		</AlertDialog.Content>
	</AlertDialog.Root>
{/if}
