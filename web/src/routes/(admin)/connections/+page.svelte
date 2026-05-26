<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { ConnectionSummary } from '$lib/api/types';
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
	import { Cable } from 'lucide-svelte';

	let connections = $state<ConnectionSummary[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let creating = $state(false);
	let form = $state({
		name: 'traefik-ssh',
		connectionType: 'traefik',
		host: '',
		port: 22,
		username: 'root',
		authMode: 'password',
		password: '',
		privateKeyPem: '',
		privateKeyPassphrase: ''
	});

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		try {
			connections = await api.listConnections();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load connections';
		} finally {
			loading = false;
		}
	}

	async function create() {
		creating = true;
		error = null;
		try {
			await api.createSshConnection({
				name: form.name,
				connectionType: form.connectionType,
				host: form.host,
				port: form.port,
				username: form.username,
				authMode: form.authMode,
				password: form.authMode === 'password' ? form.password : null,
				privateKeyPem: form.authMode === 'privateKey' ? form.privateKeyPem : null,
				privateKeyPassphrase: form.privateKeyPassphrase || null
			});
			message = 'Connection created.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create connection';
		} finally {
			creating = false;
		}
	}

	async function validate(id: string) {
		try {
			await api.validateConnection(id, {
				name: form.name,
				connectionType: form.connectionType,
				host: form.host,
				port: form.port,
				username: form.username,
				authMode: form.authMode,
				password: form.password || null,
				privateKeyPem: form.privateKeyPem || null,
				privateKeyPassphrase: form.privateKeyPassphrase || null
			});
			message = 'Connection validated.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Validation failed';
		}
	}
</script>

<AdminSectionPage
	title="Connections"
	description="DNS providers, Traefik SSH, AdGuard, notifications, and SSO."
	icon={Cable}
>
	<PanelSection title="SSH connection" description="Create Traefik or firewall host SSH connections.">
		<div class="grid max-w-xl gap-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="conn-name">Name</Label>
					<Input id="conn-name" bind:value={form.name} />
				</div>
				<div class="grid gap-1.5">
					<Label for="conn-type">Type</Label>
					<Input id="conn-type" bind:value={form.connectionType} placeholder="traefik" />
				</div>
			</div>
			<div class="grid grid-cols-3 gap-3">
				<div class="col-span-2 grid gap-1.5">
					<Label for="conn-host">Host</Label>
					<Input id="conn-host" bind:value={form.host} />
				</div>
				<div class="grid gap-1.5">
					<Label for="conn-port">Port</Label>
					<Input id="conn-port" type="number" bind:value={form.port} />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="conn-user">Username</Label>
				<Input id="conn-user" bind:value={form.username} />
			</div>
			<div class="grid gap-1.5">
				<Label for="conn-pass">Password (if authMode=password)</Label>
				<Input id="conn-pass" type="password" bind:value={form.password} />
			</div>
			<Button onclick={() => create()} disabled={creating || !form.host}>
				{creating ? 'Creating…' : 'Create connection'}
			</Button>
		</div>
	</PanelSection>

	{#if message}<p class="text-xs text-emerald-300">{message}</p>{/if}
	{#if error}<p class="text-xs text-destructive">{error}</p>{/if}

	<PanelSection title="Registered connections" description="All connection types in Hashi.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if connections.length === 0}
			<p class="text-sm text-muted-foreground">No connections yet.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Type</TableHead>
						<TableHead>Health</TableHead>
						<TableHead>Last validated</TableHead>
						<TableHead></TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each connections as conn}
						<TableRow>
							<TableCell>{conn.name}</TableCell>
							<TableCell>{conn.type}</TableCell>
							<TableCell>{conn.healthState}</TableCell>
							<TableCell class="text-xs">
								{conn.lastValidatedAtUtc
									? new Date(conn.lastValidatedAtUtc).toLocaleString()
									: '—'}
							</TableCell>
							<TableCell>
								<Button variant="outline" size="sm" onclick={() => validate(conn.id)}>
									Validate
								</Button>
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>
</AdminSectionPage>
