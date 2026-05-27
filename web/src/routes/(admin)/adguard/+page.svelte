<script lang="ts">
	import { page } from '$app/stores';
	import { api, ApiRequestError } from '$lib/api/client';
	import type { AdGuardConnection, AdGuardRewrite } from '$lib/api/types';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
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
	import { Radio } from 'lucide-svelte';

	let connections = $state<AdGuardConnection[]>([]);
	let rewrites = $state<AdGuardRewrite[]>([]);
	let selectedConnectionId = $state<string | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let syncing = $state(false);
	let testing = $state(false);
	let deletingId = $state<string | null>(null);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let connectionForm = $state({
		name: 'home-adguard',
		baseUrl: 'http://127.0.0.1:3000',
		password: ''
	});
	let rewriteForm = $state({
		domain: '',
		answer: ''
	});

	$effect(() => {
		const queryId = $page.url.searchParams.get('connection');
		if (queryId && queryId !== selectedConnectionId) {
			selectedConnectionId = queryId;
		}
	});

	$effect(() => {
		void loadConnections();
	});

	$effect(() => {
		if (selectedConnectionId) {
			void loadRewrites(selectedConnectionId);
		} else {
			rewrites = [];
		}
	});

	async function loadConnections() {
		loading = true;
		error = null;
		try {
			connections = await api.listAdGuardConnections();
			if (!selectedConnectionId && connections.length > 0) {
				selectedConnectionId = connections[0]?.id ?? null;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load AdGuard connections';
		} finally {
			loading = false;
		}
	}

	async function loadRewrites(connectionId: string) {
		try {
			rewrites = await api.listAdGuardRewrites(connectionId);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load rewrites';
		}
	}

	async function withReauth<T>(action: () => Promise<T>): Promise<T | null> {
		try {
			return await action();
		} catch (e) {
			if (e instanceof ApiRequestError && e.code === 'reauth_required') {
				const ok = await performPasskeyReauthentication();
				if (ok) {
					return await action();
				}
				error = 'Passkey reauthentication failed.';
				return null;
			}
			throw e;
		}
	}

	async function createConnection() {
		if (!connectionForm.name || !connectionForm.baseUrl || !connectionForm.password) {
			error = 'Name, base URL, and password are required.';
			return;
		}
		saving = true;
		error = null;
		message = null;
		try {
			const created = await withReauth(() =>
				api.createAdGuardConnection({
					name: connectionForm.name,
					baseUrl: connectionForm.baseUrl,
					password: connectionForm.password
				})
			);
			if (created) {
				message = `AdGuard connection "${created.name}" created.`;
				connectionForm.password = '';
				selectedConnectionId = created.id;
				await loadConnections();
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create AdGuard connection';
		} finally {
			saving = false;
		}
	}

	async function upsertRewrite() {
		if (!selectedConnectionId || !rewriteForm.domain || !rewriteForm.answer) {
			error = 'Select a connection and enter domain plus answer.';
			return;
		}
		saving = true;
		error = null;
		message = null;
		try {
			const result = await withReauth(() =>
				api.upsertAdGuardRewrite(selectedConnectionId!, {
					domain: rewriteForm.domain,
					answer: rewriteForm.answer
				})
			);
			if (result) {
				const planned = result.plan.changes.length;
				message = `Rewrite planned for ${result.rewrite?.domain ?? rewriteForm.domain}. ${planned} pending change${planned === 1 ? '' : 's'} will apply on push sync.`;
				rewriteForm.domain = '';
				rewriteForm.answer = '';
				await loadRewrites(selectedConnectionId);
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save rewrite';
		} finally {
			saving = false;
		}
	}

	async function syncConnection() {
		if (!selectedConnectionId) return;
		syncing = true;
		error = null;
		message = null;
		try {
			await withReauth(() => api.syncAdGuardConnection(selectedConnectionId!));
			message = 'Managed rewrites pushed to AdGuard Home.';
			await loadRewrites(selectedConnectionId);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Sync failed';
		} finally {
			syncing = false;
		}
	}

	async function testConnection() {
		if (!selectedConnectionId) return;
		testing = true;
		error = null;
		message = null;
		try {
			const result = await withReauth(() => api.testAdGuardConnection(selectedConnectionId!));
			message = result?.connected ? 'Connection test succeeded.' : `Connection failed: ${result?.error ?? 'unknown error'}`;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Connection test failed';
		} finally {
			testing = false;
		}
	}

	async function deleteRewrite(rewriteId: string) {
		if (!selectedConnectionId || !confirm('Delete this managed rewrite?')) return;
		deletingId = rewriteId;
		error = null;
		message = null;
		try {
			const result = await withReauth(() => api.deleteAdGuardRewrite(selectedConnectionId!, rewriteId));
			const planned = result?.plan.changes.length ?? 0;
			message = `Rewrite delete planned. ${planned} pending change${planned === 1 ? '' : 's'} will apply on push sync.`;
			await loadRewrites(selectedConnectionId);
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to delete rewrite';
		} finally {
			deletingId = null;
		}
	}
</script>

<AdminSectionPage
	title="AdGuard Home"
	description="Optional DNS rewrites managed by Hashi. Linked from Connections — not a primary nav section."
	icon={Radio}
>
	<PanelSection title="Connection" description="Register an AdGuard Home control API endpoint.">
		<div class="grid max-w-xl gap-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="ag-name">Name</Label>
					<Input id="ag-name" bind:value={connectionForm.name} />
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-url">Base URL</Label>
					<Input id="ag-url" bind:value={connectionForm.baseUrl} placeholder="http://adguard:3000" />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="ag-pass">Admin password</Label>
				<Input id="ag-pass" type="password" bind:value={connectionForm.password} />
			</div>
			<Button onclick={() => createConnection()} disabled={saving || loading}>
				{saving ? 'Saving…' : 'Add connection'}
			</Button>
		</div>
	</PanelSection>

	<PanelSection title="Managed rewrites" description="Hashi-owned rewrites only; manual entries are never deleted.">
		<div class="mb-4 flex flex-wrap items-end gap-3">
			<div class="grid min-w-[12rem] gap-1.5">
				<Label for="ag-conn">Connection</Label>
				<select
					id="ag-conn"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={selectedConnectionId}
					disabled={connections.length === 0}
				>
					{#each connections as conn (conn.id)}
						<option value={conn.id}>{conn.name}</option>
					{/each}
				</select>
			</div>
			<Button variant="outline" onclick={() => testConnection()} disabled={!selectedConnectionId || testing}>
				{testing ? 'Testing…' : 'Test connection'}
			</Button>
			<Button variant="outline" onclick={() => syncConnection()} disabled={!selectedConnectionId || syncing}>
				{syncing ? 'Syncing…' : 'Push sync'}
			</Button>
		</div>

		<div class="mb-4 grid max-w-xl gap-3 rounded-md border border-border p-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="ag-domain">Domain</Label>
					<Input id="ag-domain" bind:value={rewriteForm.domain} placeholder="*.internal.example" />
				</div>
				<div class="grid gap-1.5">
					<Label for="ag-answer">Answer</Label>
					<Input id="ag-answer" bind:value={rewriteForm.answer} placeholder="10.0.0.5" />
				</div>
			</div>
			<Button onclick={() => upsertRewrite()} disabled={saving || !selectedConnectionId}>
				{saving ? 'Saving…' : 'Save rewrite'}
			</Button>
		</div>

		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if !selectedConnectionId}
			<p class="text-sm text-muted-foreground">Add an AdGuard connection to manage rewrites.</p>
		{:else if rewrites.length === 0}
			<p class="text-sm text-muted-foreground">No Hashi-managed rewrites for this connection.</p>
		{:else}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Domain</TableHead>
						<TableHead>Answer</TableHead>
						<TableHead>Managed</TableHead>
						<TableHead class="w-24"></TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each rewrites as rewrite (rewrite.id)}
						<TableRow>
							<TableCell class="font-mono text-xs">{rewrite.domain}</TableCell>
							<TableCell class="font-mono text-xs">{rewrite.answer}</TableCell>
							<TableCell>{rewrite.managedByHashi ? 'yes' : 'no'}</TableCell>
							<TableCell>
								{#if rewrite.managedByHashi}
									<Button
										variant="ghost"
										size="sm"
										disabled={deletingId === rewrite.id}
										onclick={() => deleteRewrite(rewrite.id)}
									>
										{deletingId === rewrite.id ? '…' : 'Delete'}
									</Button>
								{/if}
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		{/if}
	</PanelSection>

	{#if message}<p class="text-xs text-emerald-300">{message}</p>{/if}
	{#if error}<p class="text-xs text-destructive">{error}</p>{/if}
</AdminSectionPage>
