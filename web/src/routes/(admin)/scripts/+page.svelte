<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
	import type { ConnectionSummary, Script } from '$lib/api/types';
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
	import { FileCode, Play } from 'lucide-svelte';

	let scripts = $state<Script[]>([]);
	let connections = $state<ConnectionSummary[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let runningId = $state<string | null>(null);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let runOutput = $state<string | null>(null);
	let form = $state({
		name: '',
		description: '',
		body: '#!/bin/bash\nset -euo pipefail\necho "Hashi script stub"',
		cronExpression: '',
		connectionId: ''
	});

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			const [scriptList, connectionList] = await Promise.all([
				api.listScripts(),
				api.listConnections()
			]);
			scripts = scriptList;
			connections = connectionList.filter(
				(c) => c.type === 'firewall_host' || c.type === 'traefik_host'
			);
			if (!form.connectionId && connections.length > 0) {
				form.connectionId = connections[0].id;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load scripts';
		} finally {
			loading = false;
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

	async function createScript() {
		if (!form.name || !form.connectionId) return;
		saving = true;
		error = null;
		message = null;
		try {
			const created = await withReauth(() =>
				api.createScript({
					connectionId: form.connectionId,
					name: form.name,
					description: form.description || 'Stub script',
					body: form.body,
					cronExpression: form.cronExpression
				})
			);
			if (created) {
				message = `Script "${created.name}" created.`;
				form.name = '';
				form.description = '';
				await load();
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create script';
		} finally {
			saving = false;
		}
	}

	async function runScript(script: Script) {
		runningId = script.id;
		error = null;
		message = null;
		runOutput = null;
		try {
			const result = await withReauth(() => api.runScript(script.id, {}));
			if (result) {
				message = result.succeeded ? `Run completed for ${script.name}.` : `Run failed for ${script.name}.`;
				runOutput = result.error ? `${result.output}\n\n${result.error}` : result.output;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Script run failed';
		} finally {
			runningId = null;
		}
	}
</script>

<AdminSectionPage
	title="Scripts"
	description="Privileged shell scripts, cron schedules, and manual run output."
	icon={FileCode}
>
	<PanelSection
		title="Create script"
		description="Saving requires passkey reauthentication. Scripts run on the linked connection host."
	>
		<div class="grid max-w-2xl gap-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="script-name">Name</Label>
					<Input id="script-name" bind:value={form.name} placeholder="backup-iptables" />
				</div>
				<div class="grid gap-1.5">
					<Label for="script-cron">Cron expression (optional)</Label>
					<Input id="script-cron" bind:value={form.cronExpression} placeholder="0 3 * * *" />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="script-desc">Description</Label>
				<Input id="script-desc" bind:value={form.description} placeholder="Weekly maintenance" />
			</div>
			<div class="grid gap-1.5">
				<Label for="script-connection">Target connection</Label>
				<select
					id="script-connection"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={form.connectionId}
				>
					{#if connections.length === 0}
						<option value="">No SSH connections</option>
					{:else}
						{#each connections as connection (connection.id)}
							<option value={connection.id}>{connection.name} ({connection.type})</option>
						{/each}
					{/if}
				</select>
			</div>
			<div class="grid gap-1.5">
				<Label for="script-body">Script body</Label>
				<textarea
					id="script-body"
					class="min-h-28 rounded-md border border-border bg-background px-3 py-2 font-mono text-xs text-white"
					bind:value={form.body}
				></textarea>
			</div>
			<Button
				onclick={() => createScript()}
				disabled={saving || !form.name || !form.connectionId}
			>
				{saving ? 'Creating…' : 'Create script'}
			</Button>
		</div>
	</PanelSection>

	<PanelSection title="Script inventory" description="Manual runs require passkey reauthentication.">
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else if error}
			<p class="text-sm text-destructive">{error}</p>
		{:else if scripts.length === 0}
			<p class="text-sm text-muted-foreground">No scripts configured.</p>
		{:else}
			<div class="overflow-hidden rounded-md border border-border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Description</TableHead>
							<TableHead>Enabled</TableHead>
							<TableHead class="w-12"></TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each scripts as script (script.id)}
							<TableRow>
								<TableCell>{script.name}</TableCell>
								<TableCell class="max-w-md truncate text-xs">{script.description}</TableCell>
								<TableCell>{script.enabled ? 'yes' : 'no'}</TableCell>
								<TableCell>
									<Button
										variant="ghost"
										size="icon-sm"
										disabled={runningId === script.id}
										onclick={() => runScript(script)}
										title="Run script"
									>
										<Play class="size-4" />
									</Button>
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
		{/if}
		{#if message}
			<p class="mt-3 text-xs text-emerald-300">{message}</p>
		{/if}
		{#if runOutput}
			<pre
				class="mt-3 max-h-48 overflow-auto rounded-md border border-border bg-background/50 p-3 font-mono text-xs text-muted-foreground"
			>{runOutput}</pre>
		{/if}
	</PanelSection>
</AdminSectionPage>
