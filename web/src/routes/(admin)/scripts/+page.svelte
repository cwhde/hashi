<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
	import type { ConnectionSummary, Script } from '$lib/api/types';
	import ShellCodeEditor from '$lib/components/editors/ShellCodeEditor.svelte';
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
		body: '',
		cronExpression: '',
		connectionId: ''
	});

	let selectedScript = $state<Script | null>(null);
	let showConfirmModal = $state(false);
	let editForm = $state({
		name: '',
		description: '',
		body: '',
		cronExpression: ''
	});

	const diffLines = $derived(selectedScript ? computeDiff(selectedScript.body, editForm.body) : []);

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
		if (!form.name || !form.connectionId || !form.body.trim()) return;
		saving = true;
		error = null;
		message = null;
		try {
			const created = await withReauth(() =>
				api.createScript({
					connectionId: form.connectionId,
					name: form.name,
					description: form.description || '',
					body: form.body,
					cronExpression: form.cronExpression,
					runTimeoutSeconds: 300
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
			const result = await withReauth(() => api.runScript(script.id));
			if (result) {
				message = result.succeeded ? `Run completed for ${script.name}.` : `Run failed for ${script.name}.`;
				runOutput = result.error ? `${result.output}\n\n${result.error}` : result.output;
				await load();
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Script run failed';
		} finally {
			runningId = null;
		}
	}

	async function toggleScript(script: Script) {
		try {
			await withReauth(() =>
				api.updateScript(script.id, {
					name: null,
					description: null,
					body: null,
					cronExpression: null,
					enabled: !script.enabled
				})
			);
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update script';
		}
	}

	async function deleteScript(script: Script) {
		if (!confirm(`Delete script "${script.name}"?`)) return;
		try {
			await withReauth(() => api.deleteScript(script.id));
			message = `Deleted ${script.name}.`;
			if (selectedScript?.id === script.id) {
				selectedScript = null;
			}
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to delete script';
		}
	}

	function selectScript(script: Script) {
		selectedScript = script;
		editForm.name = script.name;
		editForm.description = script.description;
		editForm.body = script.body;
		editForm.cronExpression = script.cronExpression;
		error = null;
		message = null;
	}

	async function saveScript() {
		if (!selectedScript) return;
		saving = true;
		error = null;
		message = null;
		try {
			const updated = await withReauth(() =>
				api.updateScript(selectedScript!.id, {
					name: editForm.name,
					description: editForm.description,
					body: editForm.body,
					cronExpression: editForm.cronExpression,
					enabled: selectedScript!.enabled
				})
			);
			if (updated) {
				message = `Script "${updated.name}" updated successfully.`;
				selectedScript = null;
				showConfirmModal = false;
				await load();
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to update script';
		} finally {
			saving = false;
		}
	}

	function computeDiff(original: string, modified: string) {
		const origLines = original.split('\n');
		const modLines = modified.split('\n');
		const matrix: number[][] = Array(origLines.length + 1)
			.fill(0)
			.map(() => Array(modLines.length + 1).fill(0));

		for (let i = 1; i <= origLines.length; i++) {
			for (let j = 1; j <= modLines.length; j++) {
				if (origLines[i - 1] === modLines[j - 1]) {
					matrix[i][j] = matrix[i - 1][j - 1] + 1;
				} else {
					matrix[i][j] = Math.max(matrix[i - 1][j], matrix[i][j - 1]);
				}
			}
		}

		interface DiffLine {
			type: 'added' | 'removed' | 'unchanged';
			text: string;
		}
		const diff: DiffLine[] = [];
		let i = origLines.length;
		let j = modLines.length;

		while (i > 0 || j > 0) {
			if (i > 0 && j > 0 && origLines[i - 1] === modLines[j - 1]) {
				diff.unshift({ type: 'unchanged', text: origLines[i - 1] });
				i--;
				j--;
			} else if (j > 0 && (i === 0 || matrix[i][j - 1] >= matrix[i - 1][j])) {
				diff.unshift({ type: 'added', text: modLines[j - 1] });
				j--;
			} else {
				diff.unshift({ type: 'removed', text: origLines[i - 1] });
				i--;
			}
		}

		return diff;
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
				<p class="text-sm font-medium leading-none">Script body</p>
				<ShellCodeEditor minHeight="10rem" bind:value={form.body} />
			</div>
			<Button
				onclick={() => createScript()}
				disabled={saving || !form.name || !form.connectionId || !form.body.trim()}
			>
				{saving ? 'Creating…' : 'Create script'}
			</Button>
		</div>
	</PanelSection>

	{#if selectedScript}
		<PanelSection
			title="Edit script: {selectedScript.name}"
			description="Saving requires passkey reauthentication. Target connections are listed below."
		>
			<div class="grid max-w-2xl gap-3">
				<div class="grid grid-cols-2 gap-3">
					<div class="grid gap-1.5">
						<Label for="edit-script-name">Name</Label>
						<Input id="edit-script-name" bind:value={editForm.name} placeholder="backup-iptables" />
					</div>
					<div class="grid gap-1.5">
						<Label for="edit-script-cron">Cron expression (optional)</Label>
						<Input id="edit-script-cron" bind:value={editForm.cronExpression} placeholder="0 3 * * *" />
					</div>
				</div>
				<div class="grid gap-1.5">
					<Label for="edit-script-desc">Description</Label>
					<Input id="edit-script-desc" bind:value={editForm.description} placeholder="Weekly maintenance" />
				</div>
				<div class="grid gap-1.5">
					<p class="text-sm font-medium leading-none">Target Connections</p>
					<div class="flex flex-wrap gap-2 rounded-md border border-border p-3 bg-background/50">
						{#if selectedScript.targets.length === 0}
							<span class="text-xs text-muted-foreground">No target hosts defined.</span>
						{:else}
							{#each selectedScript.targets as target}
								<span class="rounded bg-secondary/80 px-2 py-1 text-xs font-mono text-secondary-foreground border border-border/50">
									{target.connectionName} ({target.enabled ? 'enabled' : 'disabled'})
								</span>
							{/each}
						{/if}
					</div>
				</div>
				<div class="grid gap-1.5">
					<p class="text-sm font-medium leading-none">Script body</p>
					<ShellCodeEditor minHeight="10rem" bind:value={editForm.body} />
				</div>

				{#if editForm.body !== selectedScript.body}
					<div class="grid gap-1.5 mt-2">
						<p class="text-sm font-medium leading-none text-muted-foreground">Inline Diff Preview</p>
						<div class="max-h-60 overflow-y-auto rounded-md border border-border bg-black/40 p-3 font-mono text-xs space-y-1">
							{#each diffLines as line}
								{#if line.type === 'added'}
									<div class="text-emerald-400 bg-emerald-950/20 px-1 py-0.5 whitespace-pre-wrap">+ {line.text}</div>
								{:else if line.type === 'removed'}
									<div class="text-rose-400 bg-rose-950/20 px-1 py-0.5 line-through whitespace-pre-wrap">- {line.text}</div>
								{:else}
									<div class="text-muted-foreground/85 px-1 py-0.5 whitespace-pre-wrap">  {line.text}</div>
								{/if}
							{/each}
						</div>
					</div>
				{/if}

				<div class="flex gap-2">
					<Button
						onclick={() => { showConfirmModal = true; }}
						disabled={saving || !editForm.name || !editForm.body.trim()}
					>
						Save changes
					</Button>
					<Button variant="ghost" onclick={() => { selectedScript = null; }}>
						Cancel
					</Button>
				</div>
			</div>
		</PanelSection>
	{/if}

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
							<TableHead>Cron</TableHead>
							<TableHead>Last run</TableHead>
							<TableHead>Enabled</TableHead>
							<TableHead class="w-36"></TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each scripts as script (script.id)}
							<TableRow>
								<TableCell>{script.name}</TableCell>
								<TableCell class="font-mono text-xs">{script.cronExpression || '—'}</TableCell>
								<TableCell class="text-xs">
									{script.lastRunAtUtc
										? new Date(script.lastRunAtUtc).toLocaleString()
										: '—'}
								</TableCell>
								<TableCell>
									<Button variant="ghost" size="sm" onclick={() => toggleScript(script)}>
										{script.enabled ? 'yes' : 'no'}
									</Button>
								</TableCell>
								<TableCell class="space-x-1">
									<Button
										variant="ghost"
										size="icon-sm"
										disabled={runningId === script.id}
										onclick={() => runScript(script)}
										title="Run script"
									>
										<Play class="size-4" />
									</Button>
									<Button variant="ghost" size="sm" onclick={() => selectScript(script)}>Edit</Button>
									<Button variant="ghost" size="sm" onclick={() => deleteScript(script)}>Delete</Button>
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

	{#if showConfirmModal && selectedScript}
		<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm animate-in fade-in duration-200">
			<div class="relative w-full max-w-2xl rounded-lg border border-border bg-card p-6 shadow-xl space-y-4 max-h-[85vh] overflow-y-auto">
				<h3 class="text-lg font-semibold text-white">Confirm Script Changes</h3>
				<p class="text-sm text-muted-foreground">
					Are you sure you want to apply these changes to <span class="text-emerald-400 font-medium">{selectedScript.name}</span>?
				</p>
				
				<div class="space-y-2">
					<p class="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Target Connections</p>
					<div class="flex flex-wrap gap-2">
						{#if selectedScript.targets.length === 0}
							<span class="text-xs text-muted-foreground">No target connections.</span>
						{:else}
							{#each selectedScript.targets as target}
								<span class="rounded bg-secondary px-2.5 py-1 text-xs font-mono text-secondary-foreground border border-border">
									{target.connectionName}
								</span>
							{/each}
						{/if}
					</div>
				</div>
				
				<div class="space-y-2">
					<p class="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Diff Preview</p>
					<div class="max-h-60 overflow-y-auto rounded-md border border-border bg-black/40 p-3 font-mono text-xs space-y-1">
						{#each diffLines as line}
							{#if line.type === 'added'}
								<div class="text-emerald-400 bg-emerald-950/30 px-1 py-0.5 whitespace-pre-wrap">+ {line.text}</div>
							{:else if line.type === 'removed'}
								<div class="text-rose-400 bg-rose-950/30 px-1 py-0.5 line-through whitespace-pre-wrap">- {line.text}</div>
							{:else}
								<div class="text-muted-foreground px-1 py-0.5 whitespace-pre-wrap">  {line.text}</div>
							{/if}
						{/each}
					</div>
				</div>
				
				<div class="flex justify-end gap-3 pt-2">
					<Button variant="ghost" onclick={() => showConfirmModal = false}>Cancel</Button>
					<Button onclick={() => saveScript()} disabled={saving}>
						{saving ? 'Applying...' : 'Confirm & Apply'}
					</Button>
				</div>
			</div>
		</div>
	{/if}
</AdminSectionPage>
