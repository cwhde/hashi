<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { FirewallHost, PulseAgent, Resource } from '$lib/api/types';
	import ResourceRoutesEditor from '$lib/components/resources/ResourceRoutesEditor.svelte';
	import {
		normalizeRoutes,
		type ResourceRouteRequest
	} from '$lib/components/resources/resource-routes';
	import ResourceRulesEditor from '$lib/components/resources/ResourceRulesEditor.svelte';
	import { normalizeRules, type ResourceRuleRequest } from '$lib/components/resources/resource-rules';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { Server, Trash2 } from 'lucide-svelte';

	let resources = $state<Resource[]>([]);
	let firewallHosts = $state<FirewallHost[]>([]);
	let pulseAgents = $state<PulseAgent[]>([]);
	let availableMiddlewares = $state<string[]>([]);
	let routeDrafts = $state<Record<string, ResourceRouteRequest[]>>({});
	let ruleDrafts = $state<Record<string, ResourceRuleRequest[]>>({});
	let wafExclusionDrafts = $state<Record<string, string>>({});
	let routeSaving = $state<Record<string, boolean>>({});
	let ruleSaving = $state<Record<string, boolean>>({});
	let wafExclusionSaving = $state<Record<string, boolean>>({});
	let loading = $state(true);
	let error = $state<string | null>(null);
	let saving = $state(false);
	let form = $state({
		name: '',
		kind: 'http',
		domainMode: 'subdomain',
		domain: '',
		targetScheme: 'https',
		targetHost: '',
		targetPort: 443,
		publicPort: 443,
		pathPrefix: '',
		pathRewriteMode: '',
		pathRewrite: '',
		forwardAuthPolicy: 'adaptive',
		wafMode: 'detect_only',
		wafExclusions: '',
		firewallHostId: '',
		dashboardEnabled: false,
		statusEnabled: true,
		routes: [] as ResourceRouteRequest[],
		rules: [] as ResourceRuleRequest[]
	});

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			const [resourceList, hostList, middlewares, agents] = await Promise.all([
				api.listResources(),
				api.listFirewallHosts(),
				api.getTraefikUserMiddlewares().catch(() => ({ middlewareNames: [] })),
				api.listPulseAgents().catch(() => [])
			]);
			resources = resourceList;
			routeDrafts = Object.fromEntries(
				resourceList.map((resource) => [resource.id, cloneResourceRoutes(resource)])
			);
			ruleDrafts = Object.fromEntries(
				resourceList.map((resource) => [resource.id, cloneResourceRules(resource)])
			);
			wafExclusionDrafts = Object.fromEntries(
				resourceList.map((resource) => [resource.id, (resource.wafExclusions ?? []).join('\n')])
			);
			firewallHosts = hostList;
			pulseAgents = agents;
			availableMiddlewares = middlewares.middlewareNames ?? [];
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load resources';
		} finally {
			loading = false;
		}
	}

	function hostLabel(host: FirewallHost): string {
		const fqdn = host.domain.includes('.') ? host.domain : `${host.name}.${host.domain}`;
		return host.publicIp ? `${host.name} (${host.publicIp}) — ${fqdn}` : `${host.name} — ${fqdn}`;
	}

	async function create() {
		saving = true;
		error = null;
		try {
			await api.createResource({
				name: form.name,
				kind: form.kind,
				domainMode: form.domainMode,
				domain: form.domainMode === 'root' ? null : form.domain || null,
				targetScheme: form.targetScheme,
				targetHost: form.targetHost,
				targetPort: Number(form.targetPort),
				publicPort: form.kind === 'tcp' || form.kind === 'udp' ? Number(form.publicPort) : null,
				forwardAuthPolicy: form.forwardAuthPolicy,
				wafMode: form.wafMode,
				wafExclusions: parseLineList(form.wafExclusions),
				dashboardEnabled: form.dashboardEnabled,
				statusEnabled: form.statusEnabled,
				firewallHostId: form.firewallHostId || null,
				pathPrefix: form.pathPrefix || null,
				pathRewriteMode: form.pathRewriteMode || null,
				pathRewrite: form.pathRewriteMode ? form.pathRewrite || null : null,
				routes: form.routes.length > 0 ? normalizeRoutes(form.routes) : null,
				rules: form.rules.length > 0 ? normalizeRules(form.rules) : null
			});
			form.name = '';
			form.domainMode = 'subdomain';
			form.domain = '';
			form.targetHost = '';
			form.pathPrefix = '';
			form.pathRewriteMode = '';
			form.pathRewrite = '';
			form.wafExclusions = '';
			form.firewallHostId = '';
			form.routes = [];
			form.rules = [];
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create resource';
		} finally {
			saving = false;
		}
	}

	const resourcePatchFlags = {
		clearDomain: false,
		clearPublicPort: false,
		clearFirewallHostId: false,
		clearPulseAgentId: false,
		clearPathPrefix: false,
		clearPathRewriteMode: false,
		clearPathRewrite: false,
		clearExtraMiddlewares: false,
		clearWafExclusions: false
	} as const;

	async function toggleEnabled(resource: Resource) {
		try {
			await api.updateResource(resource.id, {
				name: null,
				enabled: !resource.enabled,
				domain: null,
				targetScheme: null,
				targetHost: null,
				targetPort: null,
				dashboardEnabled: null,
				statusEnabled: null,
				...resourcePatchFlags
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update resource';
		}
	}

	async function updateFirewallHost(resource: Resource, firewallHostId: string) {
		try {
			await api.updateResource(resource.id, {
				name: null,
				enabled: null,
				domain: null,
				targetScheme: null,
				targetHost: null,
				targetPort: null,
				dashboardEnabled: null,
				statusEnabled: null,
				firewallHostId: firewallHostId || null,
				...resourcePatchFlags,
				clearFirewallHostId: !firewallHostId
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update firewall host';
		}
	}

	async function updatePulseAgent(resource: Resource, pulseAgentId: string) {
		try {
			await api.updateResource(resource.id, {
				name: null,
				enabled: null,
				domain: null,
				targetScheme: null,
				targetHost: null,
				targetPort: null,
				dashboardEnabled: null,
				statusEnabled: null,
				pulseAgentId: pulseAgentId || null,
				...resourcePatchFlags,
				clearPulseAgentId: !pulseAgentId
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update Pulse agent';
		}
	}

	async function updateExtraMiddlewares(resource: Resource, middleware: string, enabled: boolean) {
		const current = resource.extraMiddlewares ?? [];
		const next = enabled
			? [...new Set([...current, middleware])]
			: current.filter((name) => name !== middleware);
		try {
			await api.updateResource(resource.id, {
				name: null,
				enabled: null,
				domain: null,
				targetScheme: null,
				targetHost: null,
				targetPort: null,
				dashboardEnabled: null,
				statusEnabled: null,
				...resourcePatchFlags,
				extraMiddlewares: next
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update middlewares';
		}
	}

	function parseLineList(value: string): string[] {
		return value
			.split('\n')
			.map((line) => line.trim())
			.filter(Boolean);
	}

	async function saveWafExclusions(resource: Resource) {
		wafExclusionSaving = { ...wafExclusionSaving, [resource.id]: true };
		try {
			await api.updateResource(resource.id, {
				name: null,
				enabled: null,
				domain: null,
				targetScheme: null,
				targetHost: null,
				targetPort: null,
				dashboardEnabled: null,
				statusEnabled: null,
				...resourcePatchFlags,
				wafExclusions: parseLineList(wafExclusionDrafts[resource.id] ?? '')
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update WAF exclusions';
		} finally {
			wafExclusionSaving = { ...wafExclusionSaving, [resource.id]: false };
		}
	}

	function cloneResourceRoutes(resource: Resource): ResourceRouteRequest[] {
		return (resource.routes ?? []).map((route) => ({
			enabled: route.enabled,
			priority: Number(route.priority),
			pathMatchType: route.pathMatchType,
			pathValue: route.pathValue,
			targetScheme: route.targetScheme,
			targetHost: route.targetHost,
			targetPort: Number(route.targetPort),
			rewriteMode: route.rewriteMode,
			rewriteValue: route.rewriteValue,
			extraMiddlewares: [...(route.extraMiddlewares ?? [])]
		}));
	}

	function cloneResourceRules(resource: Resource): ResourceRuleRequest[] {
		return (resource.rules ?? []).map((rule) => ({
			enabled: rule.enabled,
			priority: Number(rule.priority),
			action: rule.action,
			matchType: rule.matchType,
			matchValue: rule.matchValue
		}));
	}

	async function saveResourceRoutes(resource: Resource) {
		const draft = normalizeRoutes(routeDrafts[resource.id] ?? []);
		routeSaving = { ...routeSaving, [resource.id]: true };
		try {
			await api.updateResource(resource.id, {
				name: null,
				enabled: null,
				domain: null,
				targetScheme: null,
				targetHost: null,
				targetPort: null,
				dashboardEnabled: null,
				statusEnabled: null,
				...resourcePatchFlags,
				routes: draft
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update resource routes';
		} finally {
			routeSaving = { ...routeSaving, [resource.id]: false };
		}
	}

	async function saveResourceRules(resource: Resource) {
		const draft = normalizeRules(ruleDrafts[resource.id] ?? []);
		ruleSaving = { ...ruleSaving, [resource.id]: true };
		try {
			await api.updateResource(resource.id, {
				name: null,
				enabled: null,
				domain: null,
				targetScheme: null,
				targetHost: null,
				targetPort: null,
				dashboardEnabled: null,
				statusEnabled: null,
				...resourcePatchFlags,
				rules: draft
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update resource rules';
		} finally {
			ruleSaving = { ...ruleSaving, [resource.id]: false };
		}
	}

	async function remove(id: string) {
		if (!confirm('Delete this resource?')) return;
		try {
			await api.deleteResource(id);
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to delete resource';
		}
	}
</script>

<AdminSectionPage
	title="Resources"
	description="Managed homelab services, routing targets, and health probes."
	icon={Server}
>
	<PanelSection title="Create resource" description="Add a new managed service target.">
		<div class="grid max-w-2xl gap-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="res-name">Name</Label>
					<Input id="res-name" bind:value={form.name} />
				</div>
				<div class="grid gap-1.5">
					<Label for="res-kind">Kind</Label>
					<select
						id="res-kind"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={form.kind}
					>
						<option value="http">HTTP</option>
						<option value="https">HTTPS</option>
						<option value="h2c">H2C</option>
						<option value="tcp">TCP</option>
						<option value="udp">UDP</option>
					</select>
				</div>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="res-path-prefix">Path prefix (optional)</Label>
					<Input id="res-path-prefix" bind:value={form.pathPrefix} placeholder="/api" />
				</div>
				<div class="grid gap-1.5">
					<Label for="res-path-rewrite-mode">Rewrite mode</Label>
					<select
						id="res-path-rewrite-mode"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={form.pathRewriteMode}
					>
						<option value="">None</option>
						<option value="replace_path">Replace path</option>
						<option value="replace_prefix">Replace prefix</option>
						<option value="strip_prefix">Strip prefix</option>
						<option value="regex">Regex replace</option>
					</select>
				</div>
			</div>
			{#if form.pathRewriteMode}
				<div class="grid gap-1.5">
					<Label for="res-path-rewrite">Path rewrite target</Label>
					<Input
						id="res-path-rewrite"
						bind:value={form.pathRewrite}
						placeholder={form.pathRewriteMode === 'regex' ? '^/api/(.*) => /v1/$1' : '/'}
					/>
				</div>
			{/if}
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="res-domain-mode">Domain mode</Label>
					<select
						id="res-domain-mode"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={form.domainMode}
					>
						<option value="subdomain">Subdomain</option>
						<option value="root">Root domain</option>
						<option value="custom">Custom domain</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="res-domain">Domain</Label>
					<Input
						id="res-domain"
						bind:value={form.domain}
						placeholder={form.domainMode === 'custom' ? 'app.example.com' : 'app'}
						disabled={form.domainMode === 'root'}
					/>
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="res-firewall-host">Linux firewall host (optional)</Label>
				<select
					id="res-firewall-host"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={form.firewallHostId}
				>
					<option value="">None — manual / Pulse target</option>
					{#each firewallHosts as host (host.id)}
						<option value={host.id}>{hostLabel(host)}</option>
					{/each}
				</select>
			</div>
			<div class="grid grid-cols-3 gap-3">
				<div class="grid gap-1.5">
					<Label for="res-scheme">Scheme</Label>
					<Input id="res-scheme" bind:value={form.targetScheme} />
				</div>
				<div class="col-span-2 grid gap-1.5">
					<Label for="res-host">Target host</Label>
					<Input id="res-host" bind:value={form.targetHost} />
				</div>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="res-port">Target port</Label>
					<Input id="res-port" type="number" bind:value={form.targetPort} inputmode="numeric" />
				</div>
				{#if form.kind === 'tcp' || form.kind === 'udp'}
					<div class="grid gap-1.5">
						<Label for="res-public-port">Public port</Label>
						<Input id="res-public-port" type="number" bind:value={form.publicPort} />
					</div>
				{/if}
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="res-forward-auth">Forward auth</Label>
					<select
						id="res-forward-auth"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={form.forwardAuthPolicy}
					>
						<option value="off">Off</option>
						<option value="adaptive">Adaptive</option>
						<option value="sso_required">SSO required</option>
						<option value="observe">Observe</option>
					</select>
				</div>
				<div class="grid gap-1.5">
					<Label for="res-waf">WAF mode</Label>
					<select
						id="res-waf"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={form.wafMode}
					>
						<option value="off">Off</option>
						<option value="detect_only">Detect only</option>
						<option value="on">Block</option>
					</select>
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="res-waf-exclusions">WAF exclusions</Label>
				<textarea
					id="res-waf-exclusions"
					class="min-h-20 rounded-md border border-border bg-background px-3 py-2 font-mono text-xs text-white"
					bind:value={form.wafExclusions}
					placeholder="SecRuleRemoveById 941100"
				></textarea>
			</div>
			<ResourceRoutesEditor
				title="Advanced routes (optional)"
				bind:routes={form.routes}
				availableMiddlewares={availableMiddlewares}
				baseTarget={{
					targetScheme: form.targetScheme,
					targetHost: form.targetHost,
					targetPort: Number(form.targetPort)
				}}
				disabled={saving}
			/>
			<ResourceRulesEditor
				title="Resource rules (optional)"
				bind:rules={form.rules}
				disabled={saving}
			/>
			<div class="flex flex-wrap gap-4">
				<div class="flex items-center gap-2">
					<Switch bind:checked={form.dashboardEnabled} id="res-dash" />
					<Label for="res-dash">Dashboard tile</Label>
				</div>
				<div class="flex items-center gap-2">
					<Switch bind:checked={form.statusEnabled} id="res-status" />
					<Label for="res-status">Status monitor</Label>
				</div>
			</div>
			<Button onclick={() => create()} disabled={saving || !form.name || !form.targetHost}>
				{saving ? 'Creating…' : 'Create resource'}
			</Button>
		</div>
	</PanelSection>

	<PanelSection title="Managed resources" description="System resources cannot be deleted.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if resources.length === 0}
			<p class="text-sm text-muted-foreground">No resources yet.</p>
		{:else}
			<div class="overflow-hidden rounded-md border border-border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Domain</TableHead>
							<TableHead>Firewall host</TableHead>
							<TableHead>Pulse agent</TableHead>
							<TableHead>Target</TableHead>
							<TableHead>Extra middlewares</TableHead>
							<TableHead>WAF exclusions</TableHead>
							<TableHead>Advanced routes</TableHead>
							<TableHead>Resource rules</TableHead>
							<TableHead>Enabled</TableHead>
							<TableHead class="w-12"></TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each resources as resource (resource.id)}
							<TableRow>
								<TableCell>
									<span class="font-medium text-white">{resource.name}</span>
									{#if resource.isSystem}
										<span class="ml-1 text-[10px] text-hashi-contrast">system</span>
									{/if}
								</TableCell>
								<TableCell class="font-mono text-xs">
									{resource.resolvedDomain ?? resource.domain ?? '—'}
								</TableCell>
								<TableCell>
									<select
										class="h-8 max-w-[12rem] rounded-md border border-border bg-background px-2 text-xs text-white"
										value={resource.firewallHostId ?? ''}
										onchange={(e) =>
											updateFirewallHost(
												resource,
												(e.currentTarget as HTMLSelectElement).value
											)}
									>
										<option value="">None</option>
										{#each firewallHosts as host (host.id)}
											<option value={host.id}>{host.name}</option>
										{/each}
									</select>
								</TableCell>
								<TableCell>
									<select
										class="h-8 max-w-[12rem] rounded-md border border-border bg-background px-2 text-xs text-white"
										value={resource.pulseAgentId ?? ''}
										onchange={(e) =>
											updatePulseAgent(
												resource,
												(e.currentTarget as HTMLSelectElement).value
											)}
									>
										<option value="">None</option>
										{#each pulseAgents as agent (agent.id)}
											<option value={agent.id}>{agent.name}</option>
										{/each}
									</select>
								</TableCell>
								<TableCell class="font-mono text-xs">
									{resource.targetScheme}://{resource.targetHost}:{resource.targetPort}
								</TableCell>
								<TableCell>
									{#if availableMiddlewares.length === 0}
										<span class="text-xs text-muted-foreground">—</span>
									{:else if ['http', 'https', 'h2c'].includes(resource.kind.toLowerCase())}
										<div class="flex flex-col gap-1">
											{#each availableMiddlewares as middleware (middleware)}
												<label class="flex items-center gap-2 text-[11px] text-muted-foreground">
													<input
														type="checkbox"
														checked={(resource.extraMiddlewares ?? []).includes(middleware)}
														onchange={(e) =>
															updateExtraMiddlewares(
																resource,
																middleware,
																(e.currentTarget as HTMLInputElement).checked
															)}
													/>
													{middleware}
												</label>
											{/each}
										</div>
									{:else}
										<span class="text-xs text-muted-foreground">n/a</span>
									{/if}
								</TableCell>
								<TableCell class="min-w-[16rem]">
									{#if ['http', 'https', 'h2c'].includes(resource.kind.toLowerCase())}
										<div class="grid gap-2">
											<textarea
												class="min-h-20 rounded-md border border-border bg-background px-2 py-1 font-mono text-[11px] text-white"
												bind:value={wafExclusionDrafts[resource.id]}
											></textarea>
											<Button
												size="sm"
												variant="outline"
												onclick={() => saveWafExclusions(resource)}
												disabled={wafExclusionSaving[resource.id] === true}
											>
												{wafExclusionSaving[resource.id] ? 'Saving...' : 'Save WAF'}
											</Button>
										</div>
									{:else}
										<span class="text-xs text-muted-foreground">n/a</span>
									{/if}
								</TableCell>
								<TableCell class="min-w-[28rem]">
									<div class="grid gap-2">
										<ResourceRoutesEditor
											title="Routes"
											bind:routes={routeDrafts[resource.id]}
											availableMiddlewares={availableMiddlewares}
											baseTarget={{
												targetScheme: resource.targetScheme,
												targetHost: resource.targetHost,
												targetPort: Number(resource.targetPort)
											}}
											disabled={routeSaving[resource.id] === true}
										/>
										<Button
											size="sm"
											variant="outline"
											onclick={() => saveResourceRoutes(resource)}
											disabled={routeSaving[resource.id] === true}
										>
											{routeSaving[resource.id] ? 'Saving…' : 'Save routes'}
										</Button>
									</div>
								</TableCell>
								<TableCell class="min-w-[24rem]">
									<div class="grid gap-2">
										<ResourceRulesEditor
											title="Rules"
											bind:rules={ruleDrafts[resource.id]}
											disabled={ruleSaving[resource.id] === true}
										/>
										<Button
											size="sm"
											variant="outline"
											onclick={() => saveResourceRules(resource)}
											disabled={ruleSaving[resource.id] === true}
										>
											{ruleSaving[resource.id] ? 'Saving...' : 'Save rules'}
										</Button>
									</div>
								</TableCell>
								<TableCell>
									<Switch
										checked={resource.enabled}
										onCheckedChange={() => toggleEnabled(resource)}
									/>
								</TableCell>
								<TableCell>
									{#if !resource.isSystem}
										<Button variant="ghost" size="icon-sm" onclick={() => remove(resource.id)}>
											<Trash2 class="size-4 text-destructive" />
										</Button>
									{/if}
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
		{/if}
	</PanelSection>
</AdminSectionPage>
