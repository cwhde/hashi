<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { ConnectionSummary, DnsRecord } from '$lib/api/types';
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
	let records = $state<DnsRecord[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let creating = $state(false);
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
			[connections, records] = await Promise.all([
				api.listDnsConnections(),
				api.listDnsRecords()
			]);
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
						{#each connections as conn}
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
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
		{/if}
	</PanelSection>

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
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each records as record}
							<TableRow>
								<TableCell class="font-mono text-xs">{record.name}</TableCell>
								<TableCell>{record.type}</TableCell>
								<TableCell class="max-w-[14rem] truncate font-mono text-xs">{record.value}</TableCell>
								<TableCell>{record.ttl ?? '—'}</TableCell>
								<TableCell>{record.ownership}</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
		{/if}
	</PanelSection>
</AdminSectionPage>
