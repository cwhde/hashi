<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { ConnectionSummary, DnsImportDecision, DnsRecord, DnsSyncPlan, DnsZone } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { Globe } from 'lucide-svelte';

	let connections = $state<ConnectionSummary[]>([]);
	let zones = $state<DnsZone[]>([]);
	let records = $state<DnsRecord[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let creating = $state(false);
	let importConnectionId = $state<string | null>(null);
	let importDecisions = $state<DnsImportDecision[]>([]);
	let selectedImportIds = $state<string[]>([]);
	let importLoading = $state(false);
	let pruneConnectionId = $state<string | null>(null);
	let prunePlan = $state<DnsSyncPlan | null>(null);
	let pruneLoading = $state(false);
	let recordSaving = $state(false);
	let editingRecordId = $state<string | null>(null);
	let recordForm = $state({
		zoneId: '',
		name: '',
		type: 'A',
		value: '',
		ttl: 300,
		enabled: true
	});
	let form = $state({
		name: 'hetzner-primary',
		apiToken: '',
		zoneName: '',
		defaultTtl: 300
	});

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			[connections, zones, records] = await Promise.all([
				api.listDnsConnections(),
				api.listDnsZones(),
				api.listDnsRecords()
			]);
			if (!recordForm.zoneId && zones.length > 0) {
				recordForm.zoneId = zones[0].id;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load DNS data';
		} finally {
			loading = false;
		}
	}

	async function validateToken() {
		message = null;
		try {
			await api.validateHetznerDnsProvider({ apiToken: form.apiToken });
			message = 'Provider token validated.';
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Validation failed';
		}
	}

	async function createConnection() {
		creating = true;
		error = null;
		message = null;
		try {
			await api.createHetznerDnsConnection({
				name: form.name,
				apiToken: form.apiToken,
				zoneName: form.zoneName,
				defaultTtl: form.defaultTtl
			});
			message = 'DNS connection created.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create connection';
		} finally {
			creating = false;
		}
	}

	async function validateConnection(id: string) {
		try {
			await api.validateDnsConnection(id);
			message = 'Connection validated.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Validation failed';
		}
	}

	async function planSync(id: string) {
		try {
			const plan = await api.planDnsSync(id);
			message = `Sync plan ready (${JSON.stringify(plan).slice(0, 120)}…)`;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Sync plan failed';
		}
	}

	async function validateWrite(id: string) {
		try {
			const result = (await api.validateDnsWrite(id, true)) as { valid?: boolean; error?: string | null };
			message = result.valid
				? 'Write validation succeeded (_hashi-test record created and removed).'
				: (result.error ?? 'Write validation failed.');
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Write validation failed';
		}
	}

	async function loadImportPreview(id: string) {
		importLoading = true;
		importConnectionId = id;
		error = null;
		try {
			importDecisions = await api.previewDnsImport(id);
			selectedImportIds = importDecisions.map((d) => d.id);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Import preview failed';
		} finally {
			importLoading = false;
		}
	}

	function toggleImport(id: string, checked: boolean) {
		if (checked) {
			selectedImportIds = [...selectedImportIds, id];
		} else {
			selectedImportIds = selectedImportIds.filter((x) => x !== id);
		}
	}

	async function applyImport() {
		if (!importConnectionId) return;
		importLoading = true;
		error = null;
		try {
			await api.applyDnsImport(importConnectionId, {
				selectedDecisionIds: selectedImportIds
			});
			message = `Imported ${selectedImportIds.length} DNS records into Hashi.`;
			importDecisions = [];
			importConnectionId = null;
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Import apply failed';
		} finally {
			importLoading = false;
		}
	}

	async function previewPrune(id: string) {
		pruneLoading = true;
		pruneConnectionId = id;
		error = null;
		try {
			prunePlan = await api.previewDnsPrune(id);
			message = prunePlan.changes.length
				? `Prune preview: ${prunePlan.changes.length} record(s) to delete.`
				: 'No unimported records eligible for prune.';
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Prune preview failed';
		} finally {
			pruneLoading = false;
		}
	}

	async function applyPrune() {
		if (!pruneConnectionId) return;
		pruneLoading = true;
		error = null;
		try {
			await api.applyDnsPrune(pruneConnectionId);
			message = 'Prune applied to provider.';
			prunePlan = null;
			pruneConnectionId = null;
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Prune apply failed';
		} finally {
			pruneLoading = false;
		}
	}

	function editRecord(record: DnsRecord) {
		editingRecordId = record.id;
		recordForm = {
			zoneId: record.zoneId,
			name: record.name,
			type: record.type,
			value: record.value,
			ttl: record.ttl ?? 300,
			enabled: record.enabled
		};
	}

	function resetRecordForm() {
		editingRecordId = null;
		recordForm = {
			zoneId: zones[0]?.id ?? '',
			name: '',
			type: 'A',
			value: '',
			ttl: 300,
			enabled: true
		};
	}

	async function saveRecord() {
		recordSaving = true;
		error = null;
		message = null;
		try {
			const ttl = Number(recordForm.ttl);
			const payload = {
				...recordForm,
				ttl: Number.isFinite(ttl) && ttl > 0 ? ttl : null
			};
			if (editingRecordId) {
				await api.updateDnsRecord(editingRecordId, payload);
				message = 'Manual DNS record updated.';
			} else {
				await api.createDnsRecord(payload);
				message = 'Manual DNS record created.';
			}
			resetRecordForm();
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Manual DNS record save failed';
		} finally {
			recordSaving = false;
		}
	}

	async function toggleRecord(record: DnsRecord) {
		try {
			await api.updateDnsRecord(record.id, {
				zoneId: record.zoneId,
				name: record.name,
				type: record.type,
				value: record.value,
				ttl: record.ttl ?? null,
				enabled: !record.enabled
			});
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Manual DNS record update failed';
		}
	}

	async function deleteRecord(record: DnsRecord) {
		try {
			await api.deleteDnsRecord(record.id);
			message = 'Manual DNS record deleted.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Manual DNS record delete failed';
		}
	}
</script>

<AdminSectionPage
	title="DNS"
	description="Provider zones, managed records, sync plans, and import controls."
	icon={Globe}
>
	<PanelSection title="Hetzner connection" description="Create and validate a DNS provider connection.">
		<div class="grid max-w-xl gap-3">
			<div class="grid gap-1.5">
				<Label for="dns-name">Connection name</Label>
				<Input id="dns-name" bind:value={form.name} />
			</div>
			<div class="grid gap-1.5">
				<Label for="dns-token">API token</Label>
				<Input id="dns-token" type="password" bind:value={form.apiToken} />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="dns-zone">Zone</Label>
					<Input id="dns-zone" bind:value={form.zoneName} />
				</div>
				<div class="grid gap-1.5">
					<Label for="dns-ttl">Default TTL</Label>
					<Input id="dns-ttl" type="number" bind:value={form.defaultTtl} />
				</div>
			</div>
			<div class="flex flex-wrap gap-2">
				<Button variant="outline" onclick={() => validateToken()} disabled={!form.apiToken}>
					Validate token
				</Button>
				<Button onclick={() => createConnection()} disabled={creating || !form.apiToken || !form.zoneName}>
					{creating ? 'Creating…' : 'Create connection'}
				</Button>
			</div>
		</div>
	</PanelSection>

	{#if message}
		<p class="text-xs text-emerald-300">{message}</p>
	{/if}
	{#if error}
		<p class="text-xs text-destructive">{error}</p>
	{/if}

	<PanelSection title="Manual records" description="Create and maintain user-owned DNS records.">
		<div class="grid gap-3 md:grid-cols-[1.2fr_0.8fr_1.8fr_0.7fr_auto]">
			<div class="grid gap-1.5">
				<Label for="manual-zone">Zone</Label>
				<select
					id="manual-zone"
					class="h-10 rounded-md border border-input bg-background px-3 text-sm"
					bind:value={recordForm.zoneId}
				>
					{#each zones as zone (zone.id)}
						<option value={zone.id}>{zone.name}</option>
					{/each}
				</select>
			</div>
			<div class="grid gap-1.5">
				<Label for="manual-type">Type</Label>
				<select
					id="manual-type"
					class="h-10 rounded-md border border-input bg-background px-3 text-sm"
					bind:value={recordForm.type}
				>
					{#each ['A', 'AAAA', 'CNAME', 'MX', 'TXT'] as type (type)}
						<option value={type}>{type}</option>
					{/each}
				</select>
			</div>
			<div class="grid gap-1.5">
				<Label for="manual-name">Name</Label>
				<Input id="manual-name" bind:value={recordForm.name} placeholder="app.example.com" />
			</div>
			<div class="grid gap-1.5">
				<Label for="manual-ttl">TTL</Label>
				<Input id="manual-ttl" type="number" bind:value={recordForm.ttl} />
			</div>
			<div class="flex items-end gap-2">
				<Button
					onclick={() => saveRecord()}
					disabled={recordSaving || !recordForm.zoneId || !recordForm.name || !recordForm.value}
				>
					{recordSaving ? 'Saving...' : editingRecordId ? 'Update' : 'Create'}
				</Button>
				{#if editingRecordId}
					<Button variant="outline" onclick={() => resetRecordForm()}>Cancel</Button>
				{/if}
			</div>
		</div>
		<div class="mt-3 grid gap-1.5">
			<Label for="manual-value">Value</Label>
			<Input id="manual-value" bind:value={recordForm.value} placeholder="203.0.113.10" />
		</div>
		<label class="mt-3 flex items-center gap-2 text-sm">
			<input type="checkbox" bind:checked={recordForm.enabled} />
			Enabled
		</label>
	</PanelSection>

	<PanelSection title="Connections" description="Provider connection health and sync actions.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if connections.length === 0}
			<p class="text-sm text-muted-foreground">No DNS connections configured.</p>
		{:else}
			<div class="overflow-hidden rounded-md border border-border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Health</TableHead>
							<TableHead>Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each connections as conn (conn.id)}
							<TableRow>
								<TableCell>{conn.name}</TableCell>
								<TableCell>{conn.type}</TableCell>
								<TableCell>{conn.healthState}</TableCell>
								<TableCell class="space-x-2">
									<Button variant="outline" size="sm" onclick={() => validateConnection(conn.id)}>
										Validate
									</Button>
									<Button variant="outline" size="sm" onclick={() => planSync(conn.id)}>
										Plan sync
									</Button>
									<Button variant="outline" size="sm" onclick={() => validateWrite(conn.id)}>
										Test write
									</Button>
									<Button variant="outline" size="sm" onclick={() => loadImportPreview(conn.id)}>
										Import
									</Button>
									<Button variant="destructive" size="sm" onclick={() => previewPrune(conn.id)}>
										Prune preview
									</Button>
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
		{/if}
	</PanelSection>

	{#if importDecisions.length > 0}
		<PanelSection title="Import preview" description="Select provider records to manage in Hashi (spec §7.3).">
			<div class="overflow-hidden rounded-md border border-border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead></TableHead>
							<TableHead>Name</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Value</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each importDecisions as row (row.id)}
							<TableRow>
								<TableCell>
									<input
										type="checkbox"
										checked={selectedImportIds.includes(row.id)}
										onchange={(e) => toggleImport(row.id, (e.currentTarget as HTMLInputElement).checked)}
									/>
								</TableCell>
								<TableCell class="font-mono text-xs">{row.name}</TableCell>
								<TableCell>{row.type}</TableCell>
								<TableCell class="max-w-[14rem] truncate font-mono text-xs">{row.value}</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
			<div class="mt-3">
				<Button onclick={() => applyImport()} disabled={importLoading || selectedImportIds.length === 0}>
					{importLoading ? 'Importing…' : `Import ${selectedImportIds.length} records`}
				</Button>
			</div>
		</PanelSection>
	{/if}

	{#if prunePlan && prunePlan.changes.length > 0}
		<PanelSection
			title="Prune preview"
			description="Destructive: removes provider records that were not imported into Hashi."
		>
			<p class="text-sm text-muted-foreground">
				{prunePlan.changes.length} record(s) will be deleted from the provider (NS/SOA never pruned).
			</p>
			<Button variant="destructive" onclick={() => applyPrune()} disabled={pruneLoading}>
				{pruneLoading ? 'Pruning…' : 'Confirm prune'}
			</Button>
		</PanelSection>
	{/if}

	<PanelSection title="Managed records" description="Hashi-owned DNS record inventory.">
		{#if records.length === 0}
			<p class="text-sm text-muted-foreground">No managed records yet.</p>
		{:else}
			<div class="overflow-hidden rounded-md border border-border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Value</TableHead>
							<TableHead>TTL</TableHead>
							<TableHead>Ownership</TableHead>
							<TableHead>State</TableHead>
							<TableHead>Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each records as record (record.id ?? record.name)}
							<TableRow>
								<TableCell class="font-mono text-xs">{record.name}</TableCell>
								<TableCell>{record.type}</TableCell>
								<TableCell class="max-w-[14rem] truncate font-mono text-xs">{record.value}</TableCell>
								<TableCell>{record.ttl ?? '—'}</TableCell>
								<TableCell>{record.ownership}</TableCell>
								<TableCell>{record.enabled ? 'enabled' : 'disabled'}</TableCell>
								<TableCell class="space-x-2">
									{#if record.ownership === 'user'}
										<Button variant="outline" size="sm" onclick={() => editRecord(record)}>Edit</Button>
										<Button variant="outline" size="sm" onclick={() => toggleRecord(record)}>
											{record.enabled ? 'Disable' : 'Enable'}
										</Button>
										<Button variant="destructive" size="sm" onclick={() => deleteRecord(record)}>
											Delete
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
