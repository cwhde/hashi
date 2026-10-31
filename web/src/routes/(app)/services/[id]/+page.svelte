<script lang="ts">
	import ServiceExtras from '$lib/components/hashi/services/ServiceExtras.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Select from '$lib/components/ui/select';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { ChevronLeft, CircleAlert, Lock, Plus, Trash2 } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { pendingPlans } from '$lib/sync-pending.svelte';
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
		type ServiceInput
	} from '$lib/services';

	// One service (04 §1): public name, upstream, advanced routes with rewrites, TLS options, the rendered excerpt.
	// The Hashi admin service is shown read-only: it follows Settings → General (invariant 10).
	const id = $derived(page.params.id ?? '');
	let svc = $state<Service | null>(null);
	let edit = $state<ServiceInput | null>(null);
	let targetsText = $state<string[]>([]);
	let extra = $state({ setId: '', publicPort: '', proxy: false, entryHost: '' });
	let root = $state('');
	let yaml = $state('');
	let loadError = $state('');
	let saveError = $state('');
	let outcome = $state<{ text: string; review: boolean; bad: boolean } | null>(null);
	let busy = $state('');
	let delOpen = $state(false);
	const system = $derived(svc?.origin === 'system');

	async function load() {
		const [s, r] = await Promise.all([
			api.GET('/api/services/{id}', { params: { path: { id } } }),
			rootDomain()
		]);
		if (s.error || !s.data) throw s.error;
		svc = s.data;
		root = r;
		edit = toInput(s.data);
		extra = {
			setId: s.data.target.set_id ?? '',
			publicPort: String(s.data.public_port ?? ''),
			proxy: !!s.data.proxy_protocol,
			entryHost: s.data.entry_host_id ?? ''
		};
		targetsText = s.data.routes.map((r) =>
			r.targets
				.map(targetText)
				.join(', ')
				.replace(/ ×(\d+)/g, '*$1')
		);
		yaml = '';
		if (!s.data.invalid_reason) {
			const e = await api.GET('/api/services/{id}/effective', { params: { path: { id } } });
			if (e.data) yaml = e.data.yaml;
		}
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
				const p = parseTargets(targetsText[i] ?? '');
				if (p.bad) throw new Error(`Route ${r.path_value}: "${p.bad}" is not address:port.`);
				routes.push({ ...r, targets: p.targets });
			}
			const body: ServiceInput = {
				...edit,
				host_name: edit.host_name.trim().toLowerCase(),
				domain_mode: domainMode(edit.host_name.trim().toLowerCase(), root),
				target: extra.setId
					? { address: '', set_id: extra.setId, port: Number(edit.target.port) }
					: { address: edit.target.address.trim(), port: Number(edit.target.port) },
				routes: isStream(edit.kind) ? [] : routes,
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
			outcome = saveText(res.data);
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
		targetsText = [...targetsText, targetText(edit.target)];
	}
	function dropRoute(i: number) {
		if (!edit?.routes) return;
		edit.routes = edit.routes.filter((_, j) => j !== i);
		targetsText = targetsText.filter((_, j) => j !== i);
	}
	const matchText = { prefix: 'starts with', exact: 'is exactly', regex: 'matches regex' } as const;
</script>

<a
	href="/services"
	class="mb-3 inline-flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
	><ChevronLeft class="size-4" />Services</a
>

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !svc || !edit}
	<Skeleton class="h-28 rounded-2xl" /><Skeleton class="mt-4 h-72 rounded-2xl" />
{:else}
	<div class="mb-5 rounded-2xl border bg-card p-4">
		<div class="flex flex-wrap items-start gap-x-6 gap-y-3">
			<div class="min-w-0 flex-1">
				<h1 class="flex flex-wrap items-center gap-2 text-[22px] leading-tight font-semibold">
					{svc.name}
					{#if system}<Tag><Lock class="size-3" />system · change it in Settings → General</Tag
						>{/if}
				</h1>
				<a
					class="text-[13.5px] text-muted-foreground hover:underline"
					href="https://{svc.host_name}">https://{svc.host_name}</a
				>
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
					<Button size="sm" onclick={save} disabled={!!busy}
						>{busy === 'save' ? 'Publishing…' : 'Save and publish'}</Button
					>
				</div>
			{/if}
		</div>
		<dl class="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 text-[13px] sm:grid-cols-4">
			<div>
				<dt class="text-[12px] text-muted-foreground">State</dt>
				<dd>
					{#if svc.invalid_reason}<span class="text-bad-text">Not published</span
						>{:else if svc.enabled}Published on every Traefik host{:else}Off{/if}
				</dd>
			</div>
			<div>
				<dt class="text-[12px] text-muted-foreground">Upstream</dt>
				<dd><Mono>{svc.kind}://{targetText(svc.target)}</Mono></dd>
			</div>
			<div>
				<dt class="text-[12px] text-muted-foreground">Routes</dt>
				<dd>{svc.advanced_routes ? svc.routes.length : 'whole service to one upstream'}</dd>
			</div>
			<div>
				<dt class="text-[12px] text-muted-foreground">Origin</dt>
				<dd>{svc.origin}</dd>
			</div>
		</dl>
		{#if svc.invalid_reason}
			<p class="mt-3 rounded-xl border border-bad/25 bg-bad/10 p-3 text-[12.5px] text-bad-text">
				Left out of the Traefik files: {svc.invalid_reason}
			</p>
		{/if}
	</div>

	{#if outcome}
		<p
			role={outcome.bad ? 'alert' : 'status'}
			class="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-3 text-[13px] {outcome.bad
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
			class="mb-4 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
		>
			<CircleAlert class="mt-0.5 size-4 shrink-0" />{saveError}
		</p>
	{/if}

	<Tabs.Root value="routing">
		<Tabs.List class="mb-4">
			<Tabs.Trigger value="routing">Routing</Tabs.Trigger>
			<Tabs.Trigger value="access">Access</Tabs.Trigger>
			<Tabs.Trigger value="waf">WAF</Tabs.Trigger>
			<Tabs.Trigger value="monitoring">Monitoring</Tabs.Trigger>
			<Tabs.Trigger value="config">Rendered config</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="routing" class="grid gap-4 lg:grid-cols-2">
			<Panel title="Public address">
				<fieldset class="space-y-4" disabled={system}>
					<div class="grid gap-4 sm:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="n">Name</Label><Input id="n" bind:value={edit.name} />
						</div>
						<div class="space-y-1.5">
							<Label for="sl">Short name</Label><Input
								id="sl"
								bind:value={edit.slug}
								class="font-mono"
							/>
						</div>
					</div>
					<div class="space-y-1.5">
						<Label for="h">Domain</Label><Input id="h" bind:value={edit.host_name} />
						<p class="text-[12.5px] text-muted-foreground">
							Inside the managed zone. DNS records for services arrive with firewall hosts; until
							then the name points where you set it on the DNS page.
						</p>
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
				{#snippet actions()}{#if !system}<div class="flex items-center gap-2 text-[13px]">
							<Label for="adv">Advanced routes</Label><Switch
								id="adv"
								bind:checked={edit!.advanced_routes}
							/>
						</div>{/if}{/snippet}
				<fieldset class="space-y-3" disabled={system}>
					<div class="grid grid-cols-[minmax(0,9rem)_1fr_6rem] gap-2">
						<Select.Root type="single" bind:value={edit.kind}>
							<Select.Trigger aria-label="Protocol">{edit.kind.toUpperCase()}</Select.Trigger>
							<Select.Content>
								{#each kinds as k (k)}<Select.Item value={k}>{kindText[k]}</Select.Item>{/each}
							</Select.Content>
						</Select.Root>
						<Input bind:value={edit.target.address} aria-label="Address" disabled={!!extra.setId} />
						<Input bind:value={edit.target.port} aria-label="Port" inputmode="numeric" />
					</div>
					<ServiceExtras
						kind={edit.kind}
						bind:setId={extra.setId}
						bind:publicPort={extra.publicPort}
						bind:proxy={extra.proxy}
						bind:entryHost={extra.entryHost}
					/>
					{#if edit.kind === 'https'}
						<div class="space-y-3 rounded-xl border p-3">
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
						</div>
					{/if}
					{#if edit.advanced_routes && !isStream(edit.kind)}
						<p class="text-[12.5px] text-muted-foreground">
							Requests matching a route go to its targets; everything else goes to the upstream
							above. Higher priority is checked first.
						</p>
						<ul class="space-y-3" aria-label="Routes">
							{#each edit.routes ?? [] as r, i (r.id ?? i)}
								<li class="space-y-2 rounded-xl border p-3">
									<div class="flex flex-wrap items-center gap-2">
										<Switch bind:checked={r.enabled} aria-label="Route on" />
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

		<Tabs.Content value="access">
			<Panel title="Access">
				<p class="text-[13px] text-muted-foreground">
					Rules and sign-in arrive in a later slice. Until then Hashi's forward auth sits in front
					of this service and lets every request through; if Hashi is unreachable the service stays
					open.
				</p>
			</Panel>
		</Tabs.Content>

		<Tabs.Content value="waf">
			<Panel title="WAF">
				<p class="text-[13px] text-muted-foreground">
					The web application firewall per path arrives in a later slice. Nothing filters request
					bodies yet.
				</p>
			</Panel>
		</Tabs.Content>

		<Tabs.Content value="monitoring">
			<Panel title="Monitoring">
				<p class="text-[13px] text-muted-foreground">
					Checks of the public address and the upstream arrive with monitoring (S7). "Monitor" above
					already decides whether this service gets them.
				</p>
			</Panel>
		</Tabs.Content>

		<Tabs.Content value="config">
			<Panel
				title="10-hashi-http-services.yml"
				description="What Hashi renders for this service on every Traefik host. Read-only."
				flush
			>
				{#if yaml}
					<pre
						class="overflow-x-auto rounded-b-2xl bg-surface-2/40 p-4 font-mono text-[12.5px] leading-relaxed"><code
							>{yaml}</code
						></pre>
				{:else}
					<p class="px-4 py-3 text-[13px] text-muted-foreground">
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
