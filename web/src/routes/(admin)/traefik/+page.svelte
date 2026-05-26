<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type {
		ConnectionSummary,
		TraefikApplyResponse,
		TraefikDetectExistingResponse,
		TraefikHostState,
		TraefikRenderResponse,
		TraefikUserMiddleware
	} from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Label } from '$lib/components/ui/label';
	import { Network } from 'lucide-svelte';

	let render = $state<TraefikRenderResponse | null>(null);
	let connections = $state<ConnectionSummary[]>([]);
	let selectedConnectionId = $state('');
	let hostState = $state<TraefikHostState | null>(null);
	let existingConfig = $state<TraefikDetectExistingResponse | null>(null);
	let userMiddlewares = $state<TraefikUserMiddleware | null>(null);
	let middlewareYaml = $state('');
	let middlewareValidation = $state<string | null>(null);
	let confirmReplace = $state(false);
	let loading = $state(false);
	let applying = $state(false);
	let rollingBack = $state(false);
	let savingMiddleware = $state(false);
	let error = $state<string | null>(null);
	let applyResult = $state<TraefikApplyResponse | null>(null);
	let activeTab = $state<'preview' | 'middlewares' | 'apply'>('preview');

	async function loadAll() {
		loading = true;
		error = null;
		try {
			const [renderResult, connList, middlewareResult] = await Promise.all([
				api.getTraefikRender(),
				api.listConnections('traefik_host'),
				api.getTraefikUserMiddlewares()
			]);
			render = renderResult;
			connections = connList;
			userMiddlewares = middlewareResult;
			middlewareYaml = middlewareResult.yaml;
			if (!selectedConnectionId && connList.length > 0) {
				selectedConnectionId = connList[0].id;
			}
			await refreshConnectionState();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load Traefik manager';
		} finally {
			loading = false;
		}
	}

	async function refreshConnectionState() {
		if (!selectedConnectionId) {
			hostState = null;
			existingConfig = null;
			return;
		}

		const [state, existing] = await Promise.all([
			api.getTraefikHostState(selectedConnectionId),
			api.detectExistingTraefikConfig(selectedConnectionId)
		]);
		hostState = state;
		existingConfig = existing;
	}

	async function validateMiddlewareYaml() {
		middlewareValidation = null;
		try {
			const result = await api.validateTraefikUserMiddlewares({ yaml: middlewareYaml });
			middlewareValidation = result.isValid
				? `Valid. Middlewares: ${result.middlewareNames.join(', ') || 'none'}`
				: (result.error ?? 'Invalid YAML');
		} catch (e) {
			middlewareValidation = e instanceof ApiRequestError ? e.message : 'Validation failed';
		}
	}

	async function saveMiddlewareYaml() {
		savingMiddleware = true;
		error = null;
		try {
			userMiddlewares = await api.updateTraefikUserMiddlewares({ yaml: middlewareYaml });
			middlewareYaml = userMiddlewares.yaml;
			middlewareValidation = 'Saved.';
			render = await api.getTraefikRender();
			await refreshConnectionState();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save middleware YAML';
		} finally {
			savingMiddleware = false;
		}
	}

	async function applyConfig() {
		if (!selectedConnectionId) return;
		applying = true;
		error = null;
		applyResult = null;
		try {
			applyResult = await api.applyTraefikConnection(selectedConnectionId, {
				confirmReplaceExisting: confirmReplace
			});
			await refreshConnectionState();
			render = await api.getTraefikRender();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to apply Traefik config';
		} finally {
			applying = false;
		}
	}

	async function rollbackConfig() {
		if (!selectedConnectionId) return;
		rollingBack = true;
		error = null;
		applyResult = null;
		try {
			applyResult = await api.rollbackTraefikConnection(selectedConnectionId);
			await refreshConnectionState();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to rollback Traefik config';
		} finally {
			rollingBack = false;
		}
	}

	$effect(() => {
		void loadAll();
	});

	$effect(() => {
		if (selectedConnectionId) {
			void refreshConnectionState();
		}
	});
</script>

<AdminSectionPage
	title="Traefik"
	description="Config render, user middleware editor, apply, and rollback."
	icon={Network}
>
	<div class="flex flex-wrap gap-2">
		<Button variant={activeTab === 'preview' ? 'default' : 'outline'} onclick={() => (activeTab = 'preview')}>
			Preview
		</Button>
		<Button
			variant={activeTab === 'middlewares' ? 'default' : 'outline'}
			onclick={() => (activeTab = 'middlewares')}
		>
			User middlewares
		</Button>
		<Button variant={activeTab === 'apply' ? 'default' : 'outline'} onclick={() => (activeTab = 'apply')}>
			Apply / rollback
		</Button>
		<Button variant="outline" onclick={() => loadAll()} disabled={loading}>
			{loading ? 'Refreshing…' : 'Refresh'}
		</Button>
	</div>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	{#if activeTab === 'preview' && render}
		<div class="flex items-center gap-2 text-xs text-muted-foreground">
			<span>hash {render.contentHash}</span>
		</div>
		<PanelSection title="Static config" description="Generated traefik.yml preview.">
			<pre
				class="max-h-96 overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px] text-hashi-foreground">{render.staticConfigYaml}</pre>
		</PanelSection>
		<PanelSection title="Dynamic HTTP" description="Generated HTTP routers and services.">
			<pre
				class="max-h-96 overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px] text-hashi-foreground">{render.dynamicHttpYaml}</pre>
		</PanelSection>
		{#if render.dynamicFiles}
			<PanelSection title="User middleware file" description="Contents of 30-user-middlewares.yml.">
				<pre
					class="max-h-64 overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px] text-hashi-foreground">{render.dynamicFiles.userMiddlewaresYaml}</pre>
			</PanelSection>
			<PanelSection title="Stream resources" description="TCP/UDP dynamic config.">
				<pre
					class="max-h-64 overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px] text-hashi-foreground">{render.dynamicFiles.streamResourcesYaml}</pre>
			</PanelSection>
		{/if}
	{/if}

	{#if activeTab === 'middlewares'}
		<PanelSection
			title="30-user-middlewares.yml"
			description="Edit extra Traefik middlewares. Parse errors keep the last applied file."
		>
			{#if userMiddlewares?.lastParseError}
				<p class="mb-2 text-xs text-destructive">Last parse error: {userMiddlewares.lastParseError}</p>
			{/if}
			<textarea
				class="min-h-72 w-full rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px] text-hashi-foreground"
				bind:value={middlewareYaml}
			></textarea>
			<div class="mt-3 flex flex-wrap gap-2">
				<Button variant="outline" onclick={() => validateMiddlewareYaml()}>Validate YAML</Button>
				<Button onclick={() => saveMiddlewareYaml()} disabled={savingMiddleware}>
					{savingMiddleware ? 'Saving…' : 'Save middlewares'}
				</Button>
			</div>
			{#if middlewareValidation}
				<p class="mt-2 text-xs text-muted-foreground">{middlewareValidation}</p>
			{/if}
			{#if userMiddlewares && userMiddlewares.middlewareNames.length > 0}
				<p class="mt-3 text-xs text-muted-foreground">
					Available for resources: {userMiddlewares.middlewareNames.join(', ')}
				</p>
			{/if}
		</PanelSection>
	{/if}

	{#if activeTab === 'apply'}
		<PanelSection title="Traefik connection" description="Apply rendered config over SSH using stored credentials.">
			{#if connections.length === 0}
				<p class="text-sm text-muted-foreground">No Traefik connections configured yet.</p>
			{:else}
				<div class="grid max-w-xl gap-3">
					<div class="grid gap-1.5">
						<Label for="traefik-connection">Connection</Label>
						<select
							id="traefik-connection"
							class="rounded-md border border-border bg-background px-2 py-1 text-sm"
							bind:value={selectedConnectionId}
						>
							{#each connections as conn (conn.id)}
								<option value={conn.id}>{conn.name}</option>
							{/each}
						</select>
					</div>

					{#if hostState}
						<div class="rounded-md border border-border p-3 text-xs">
							<StatusRow label="Last applied hash" value={hostState.lastAppliedContentHash ?? 'never'} />
							<StatusRow label="Current render hash" value={hostState.currentContentHash ?? '—'} />
							<StatusRow
								label="Pending changes"
								value={hostState.hasPendingChanges ? 'yes' : 'no'}
								status={hostState.hasPendingChanges ? 'warn' : 'ok'}
							/>
							<StatusRow
								label="Backup available"
								value={hostState.hasBackup ? 'yes' : 'no'}
								status={hostState.hasBackup ? 'ok' : 'neutral'}
							/>
						</div>
					{/if}

					{#if existingConfig?.found}
						<div class="rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-xs">
							<p class="font-medium text-amber-200">Existing config detected at {existingConfig.remotePath}</p>
							<pre class="mt-2 max-h-40 overflow-auto whitespace-pre-wrap text-muted-foreground">{existingConfig.preview}</pre>
						</div>
					{/if}

					<div class="flex items-center gap-2">
						<Checkbox bind:checked={confirmReplace} id="traefik-confirm" />
						<Label for="traefik-confirm">Confirm backup and Hashi ownership before applying</Label>
					</div>

					<div class="flex flex-wrap gap-2">
						<Button onclick={() => applyConfig()} disabled={applying || !selectedConnectionId}>
							{applying ? 'Applying…' : 'Apply config'}
						</Button>
						<Button
							variant="outline"
							onclick={() => rollbackConfig()}
							disabled={rollingBack || !hostState?.hasBackup}
						>
							{rollingBack ? 'Rolling back…' : 'Rollback static config'}
						</Button>
					</div>
				</div>
			{/if}
		</PanelSection>

		{#if applyResult}
			<PanelSection title="Last operation" description="Result from apply or rollback.">
				<StatusRow label="Succeeded" value={String(applyResult.succeeded)} status={applyResult.succeeded ? 'ok' : 'error'} />
				<StatusRow label="Skipped" value={String(applyResult.skipped)} />
				<StatusRow label="Hash" value={applyResult.contentHash} />
				{#if applyResult.message}
					<p class="mt-2 text-xs text-muted-foreground">{applyResult.message}</p>
				{/if}
			</PanelSection>
		{/if}
	{/if}
</AdminSectionPage>
