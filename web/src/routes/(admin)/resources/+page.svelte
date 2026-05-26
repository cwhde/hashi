<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { FirewallHost, Resource } from '$lib/api/types';
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
	let loading = $state(true);
	let error = $state<string | null>(null);
	let saving = $state(false);
	let form = $state({
		name: '',
		kind: 'http',
		domain: '',
		targetScheme: 'https',
		targetHost: '',
		targetPort: 443,
		firewallHostId: '',
		dashboardEnabled: false,
		statusEnabled: true
	});

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			const [resourceList, hostList] = await Promise.all([
				api.listResources(),
				api.listFirewallHosts()
			]);
			resources = resourceList;
			firewallHosts = hostList;
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
				domain: form.domain || null,
				targetScheme: form.targetScheme,
				targetHost: form.targetHost,
				targetPort: form.targetPort,
				dashboardEnabled: form.dashboardEnabled,
				statusEnabled: form.statusEnabled,
				firewallHostId: form.firewallHostId || null
			});
			form.name = '';
			form.domain = '';
			form.targetHost = '';
			form.firewallHostId = '';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create resource';
		} finally {
			saving = false;
		}
	}

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
				clearFirewallHostId: false
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
				clearFirewallHostId: !firewallHostId
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update firewall host';
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
					<Input id="res-kind" bind:value={form.kind} placeholder="http" />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="res-domain">Domain</Label>
				<Input id="res-domain" bind:value={form.domain} placeholder="app.example.com" />
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
			<div class="grid gap-1.5">
				<Label for="res-port">Port</Label>
				<Input id="res-port" type="number" bind:value={form.targetPort} />
			</div>
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
							<TableHead>Target</TableHead>
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
								<TableCell class="font-mono text-xs">{resource.domain ?? '—'}</TableCell>
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
								<TableCell class="font-mono text-xs">
									{resource.targetScheme}://{resource.targetHost}:{resource.targetPort}
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
