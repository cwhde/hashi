<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import CaptchaSettings from '$lib/components/settings/CaptchaSettings.svelte';
	import type { components } from '$lib/api/schema.js';

	type BlocklistSource = components['schemas']['BlocklistSourceResponse'];
	type BlocklistPreview = components['schemas']['BlocklistFetchPreviewResponse'];
	type BlocklistSourceRequest = components['schemas']['UpsertBlocklistSourceRequest'];
	type BlocklistMetadata = {
		recommended?: boolean;
		falsePositiveWarning?: string;
		observedFormat?: string;
		parser?: {
			csvColumnIndex?: number;
			valueColumnIndex?: number;
			cidrPrefixColumnIndex?: number;
			jsonArrayField?: string;
			jsonValueField?: string;
		};
	};

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let oidc = $state(false);
	let adguard = $state(false);
	let internalAgentDns = $state(false);
	let notifications = $state(false);
	let geoip = $state(false);
	let captcha = $state(false);
	let blocklists = $state(false);

	let adguardSaving = $state(false);
	let adguardMessage = $state<string | null>(null);
	let adguardError = $state<string | null>(null);
	let adguardForm = $state({
		name: 'home-adguard',
		baseUrl: 'http://127.0.0.1:3000',
		password: ''
	});
	let internalDnsSaving = $state(false);
	let internalDnsMessage = $state<string | null>(null);
	let internalDnsError = $state<string | null>(null);
	let internalDnsForm = $state({
		enabled: true,
		domain: 'hashi.home.arpa',
		adGuardConnectionId: ''
	});

	let notificationSaving = $state(false);
	let notificationMessage = $state<string | null>(null);
	let notificationError = $state<string | null>(null);
	let notificationForm = $state({
		name: 'alerts-telegram',
		type: 'telegram',
		settingsJson: '{}',
		enabled: true
	});

	let oidcSaving = $state(false);
	let oidcMessage = $state<string | null>(null);
	let oidcError = $state<string | null>(null);
	let oidcForm = $state({
		name: 'edge-sso',
		issuer: '',
		clientId: '',
		clientSecret: '',
		scopes: 'openid profile email',
		enabled: true
	});

	let geoipSaving = $state(false);
	let geoipUpdating = $state(false);
	let geoipMessage = $state<string | null>(null);
	let geoipError = $state<string | null>(null);
	let geoipForm = $state({
		accountId: '',
		licenseKey: '',
		updateIntervalHours: 72,
		enabled: true
	});

	let blocklistSources = $state<BlocklistSource[]>([]);
	let blocklistsLoaded = $state(false);
	let blocklistsLoading = $state(false);
	let blocklistPreviewingId = $state<string | null>(null);
	let blocklistSaving = $state(false);
	let blocklistError = $state<string | null>(null);
	let blocklistMessage = $state<string | null>(null);
	let selectedBlocklistIds = $state<string[]>([]);
	let previewBySourceId = $state<Record<string, BlocklistPreview>>({});
	let enforcementBySourceId = $state<Record<string, string>>({});
	let customBlocklistForm = $state({
		name: 'custom-blocklist',
		sourceUrl: '',
		format: 'text',
		enforcementMode: 'middleware',
		refreshIntervalHours: 24,
		allowHttp: false,
		canFirewallEnforce: true,
		csvColumnIndex: '',
		jsonArrayField: '',
		jsonValueField: ''
	});

	const recommendedBlocklistSources = $derived(blocklistSources.filter(isRecommendedBlocklist));
	const customBlocklistSources = $derived(blocklistSources.filter((source) => !isRecommendedBlocklist(source)));
	const selectedBlocklistsReady = $derived(
		selectedBlocklistIds.length > 0 && selectedBlocklistIds.every((id) => !!previewBySourceId[id])
	);

	async function saveAdGuard() {
		if (!adguardForm.name || !adguardForm.baseUrl || !adguardForm.password) {
			adguardError = 'Name, base URL, and password are required.';
			return;
		}
		adguardSaving = true;
		adguardError = null;
		adguardMessage = null;
		try {
			const created = await api.createAdGuardConnection(adguardForm);
			adguardMessage = `Saved AdGuard connection "${created.name}".`;
			internalDnsForm.adGuardConnectionId = created.id;
			adguardForm.password = '';
		} catch (e) {
			adguardError = e instanceof ApiRequestError ? e.message : 'Failed to save AdGuard connection';
		} finally {
			adguardSaving = false;
		}
	}

	async function saveInternalAgentDns() {
		if (!internalDnsForm.adGuardConnectionId) {
			internalDnsError = 'Save or select an AdGuard connection first.';
			return;
		}
		internalDnsSaving = true;
		internalDnsError = null;
		internalDnsMessage = null;
		try {
			await api.updateInternalAgentDnsSettings({
				enabled: internalDnsForm.enabled,
				domain: internalDnsForm.domain,
				keepLastRewriteWhenAgentStale: true,
				adGuardConnectionId: internalDnsForm.adGuardConnectionId,
				agents: null
			});
			internalDnsMessage = 'Saved internal agent DNS settings.';
		} catch (e) {
			internalDnsError = e instanceof ApiRequestError ? e.message : 'Failed to save internal agent DNS';
		} finally {
			internalDnsSaving = false;
		}
	}

	async function saveNotificationProvider() {
		if (!notificationForm.name) {
			notificationError = 'Provider name is required.';
			return;
		}
		notificationSaving = true;
		notificationError = null;
		notificationMessage = null;
		try {
			const created = await api.createNotificationProvider(notificationForm);
			notificationMessage = `Saved notification provider "${created.name}".`;
		} catch (e) {
			notificationError =
				e instanceof ApiRequestError ? e.message : 'Failed to save notification provider';
		} finally {
			notificationSaving = false;
		}
	}

	async function saveOidcProvider() {
		if (!oidcForm.name || !oidcForm.issuer || !oidcForm.clientId || !oidcForm.clientSecret) {
			oidcError = 'Name, issuer, client ID, and client secret are required.';
			return;
		}
		oidcSaving = true;
		oidcError = null;
		oidcMessage = null;
		try {
			await api.createEdgeSsoProvider({
				name: oidcForm.name,
				issuer: oidcForm.issuer,
				clientId: oidcForm.clientId,
				clientSecret: oidcForm.clientSecret,
				scopes: oidcForm.scopes,
				enabled: oidcForm.enabled
			});
			oidcMessage = `Saved OIDC provider "${oidcForm.name}".`;
			oidcForm.clientSecret = '';
		} catch (e) {
			oidcError = e instanceof ApiRequestError ? e.message : 'Failed to save OIDC provider';
		} finally {
			oidcSaving = false;
		}
	}

	async function saveGeoIpSettings(runUpdate = false) {
		if (!geoipForm.accountId || !geoipForm.licenseKey) {
			geoipError = 'Account ID and license key are required.';
			return;
		}
		geoipSaving = true;
		geoipError = null;
		geoipMessage = null;
		try {
			const saved = await api.updateGeoIpSettings({
				enabled: geoipForm.enabled,
				accountId: geoipForm.accountId,
				licenseKey: geoipForm.licenseKey,
				updateIntervalHours: geoipForm.updateIntervalHours
			});
			geoipForm.licenseKey = '';
			geoipMessage = saved.enabled ? 'Saved GeoIP update settings.' : 'Saved GeoIP settings.';
			if (runUpdate) {
				geoipUpdating = true;
				const result = await api.runGeoIpUpdate();
				geoipMessage = result.message ?? 'GeoIP update completed.';
			}
		} catch (e) {
			geoipError = e instanceof ApiRequestError ? e.message : 'Failed to save GeoIP settings';
		} finally {
			geoipSaving = false;
			geoipUpdating = false;
		}
	}

	function readBlocklistMetadata(source: BlocklistSource): BlocklistMetadata {
		try {
			return JSON.parse(source.metadataJson ?? '{}') as BlocklistMetadata;
		} catch {
			return {};
		}
	}

	function isRecommendedBlocklist(source: BlocklistSource): boolean {
		return readBlocklistMetadata(source).recommended === true;
	}

	function blocklistWarning(source: BlocklistSource): string {
		return (
			readBlocklistMetadata(source).falsePositiveWarning ??
			'Third-party blocklists can create false positives; preview before enabling.'
		);
	}

	function blocklistFormatNote(source: BlocklistSource): string | null {
		return readBlocklistMetadata(source).observedFormat ?? null;
	}

	function selectedEnforcementMode(source: BlocklistSource): string {
		return enforcementBySourceId[source.id] ?? source.enforcementMode ?? 'middleware';
	}

	function hasUnsupportedParserOptions(source: BlocklistSource): boolean {
		return readBlocklistMetadata(source).parser?.cidrPrefixColumnIndex !== undefined;
	}

	function blocklistPatchRequest(source: BlocklistSource): BlocklistSourceRequest {
		const parser = readBlocklistMetadata(source).parser ?? {};
		return {
			name: source.name,
			sourceUrl: source.sourceUrl,
			description: source.description || null,
			format: source.format,
			enforcementMode: selectedEnforcementMode(source),
			canFirewallEnforce: source.canFirewallEnforce,
			enabled: source.enabled,
			allowHttp: source.allowHttp,
			refreshIntervalHours: source.refreshIntervalHours,
			csvColumnIndex: parser.csvColumnIndex ?? parser.valueColumnIndex ?? null,
			jsonArrayField: parser.jsonArrayField ?? null,
			jsonValueField: parser.jsonValueField ?? null
		};
	}

	function setBlocklistSelected(sourceId: string, checked: boolean) {
		selectedBlocklistIds = checked
			? Array.from(new Set([...selectedBlocklistIds, sourceId]))
			: selectedBlocklistIds.filter((id) => id !== sourceId);
	}

	function eventChecked(event: Event): boolean {
		return (event.currentTarget as HTMLInputElement).checked;
	}

	function eventValue(event: Event): string {
		return (event.currentTarget as HTMLSelectElement).value;
	}

	function setBlocklistEnforcement(source: BlocklistSource, value: string) {
		enforcementBySourceId = { ...enforcementBySourceId, [source.id]: value };
		delete previewBySourceId[source.id];
		previewBySourceId = { ...previewBySourceId };
	}

	async function loadBlocklistSources() {
		blocklistsLoading = true;
		blocklistError = null;
		try {
			const sources = await api.listBlocklistSources();
			blocklistSources = sources;
			blocklistsLoaded = true;
			enforcementBySourceId = Object.fromEntries(
				sources.map((source) => [source.id, selectedEnforcementMode(source)])
			);
		} catch (e) {
			blocklistError = e instanceof ApiRequestError ? e.message : 'Failed to load blocklist sources';
		} finally {
			blocklistsLoaded = true;
			blocklistsLoading = false;
		}
	}

	async function previewBlocklistSource(source: BlocklistSource) {
		blocklistPreviewingId = source.id;
		blocklistError = null;
		blocklistMessage = null;
		try {
			const preview = await api.previewBlocklistSource(source.id);
			previewBySourceId = { ...previewBySourceId, [source.id]: preview };
			setBlocklistSelected(source.id, true);
			blocklistMessage = `Previewed ${source.name}: ${preview.parsedCount} parsed, ${preview.errorCount} errors.`;
		} catch (e) {
			blocklistError = e instanceof ApiRequestError ? e.message : 'Blocklist preview failed';
		} finally {
			blocklistPreviewingId = null;
		}
	}

	async function createCustomBlocklistSource() {
		if (!customBlocklistForm.sourceUrl) {
			blocklistError = 'Custom blocklist URL is required.';
			return;
		}
		blocklistSaving = true;
		blocklistError = null;
		blocklistMessage = null;
		try {
			const created = await api.createBlocklistSource({
				name: customBlocklistForm.name || 'Custom blocklist',
				sourceUrl: customBlocklistForm.sourceUrl,
				description: 'Custom URL blocklist from setup',
				format: customBlocklistForm.format,
				enforcementMode: customBlocklistForm.enforcementMode,
				canFirewallEnforce: customBlocklistForm.canFirewallEnforce,
				enabled: false,
				allowHttp: customBlocklistForm.allowHttp,
				refreshIntervalHours: customBlocklistForm.refreshIntervalHours,
				csvColumnIndex: customBlocklistForm.csvColumnIndex
					? Number(customBlocklistForm.csvColumnIndex)
					: null,
				jsonArrayField: customBlocklistForm.jsonArrayField || null,
				jsonValueField: customBlocklistForm.jsonValueField || null
			});
			customBlocklistForm.sourceUrl = '';
			customBlocklistForm.csvColumnIndex = '';
			customBlocklistForm.jsonArrayField = '';
			customBlocklistForm.jsonValueField = '';
			enforcementBySourceId = { ...enforcementBySourceId, [created.id]: created.enforcementMode };
			setBlocklistSelected(created.id, true);
			await loadBlocklistSources();
			await previewBlocklistSource(created);
		} catch (e) {
			blocklistError = e instanceof ApiRequestError ? e.message : 'Failed to add custom blocklist';
		} finally {
			blocklistSaving = false;
		}
	}

	async function enablePreviewedBlocklists() {
		const sources = blocklistSources.filter((source) => selectedBlocklistIds.includes(source.id));
		if (sources.length === 0 || sources.some((source) => !previewBySourceId[source.id])) {
			blocklistError = 'Preview each selected blocklist before enabling.';
			return;
		}
		blocklistSaving = true;
		blocklistError = null;
		blocklistMessage = null;
		try {
			let enabledCount = 0;
			for (const source of sources) {
				const enforcementChanged = selectedEnforcementMode(source) !== source.enforcementMode;
				if (enforcementChanged) {
					if (hasUnsupportedParserOptions(source)) {
						throw new ApiRequestError(
							`${source.name} uses parser options setup cannot safely preserve. Keep ${source.enforcementMode} or configure it later.`,
							400
						);
					}
					await api.updateBlocklistSource(source.id, blocklistPatchRequest(source));
				}
				if (!source.enabled) {
					await api.enableBlocklistSource(source.id);
					enabledCount += 1;
				}
			}
			blocklistMessage =
				enabledCount === 0
					? 'Selected blocklists were already enabled.'
					: `Enabled ${enabledCount} previewed blocklist source${enabledCount === 1 ? '' : 's'}.`;
			selectedBlocklistIds = [];
			previewBySourceId = {};
			await loadBlocklistSources();
		} catch (e) {
			blocklistError = e instanceof ApiRequestError ? e.message : 'Failed to enable blocklists';
		} finally {
			blocklistSaving = false;
		}
	}

	$effect(() => {
		if (blocklists && !blocklistsLoaded && !blocklistsLoading) {
			void loadBlocklistSources();
		}
	});
</script>

<div class="grid max-w-xl gap-3">
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">OIDC SSO provider</p>
			<p class="text-xs text-muted-foreground">Optional edge SSO during setup.</p>
		</div>
		<Switch bind:checked={oidc} />
	</div>
	{#if oidc}
		<div class="grid gap-3 rounded-md border border-border bg-hashi-bg-dark p-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="setup-oidc-name">Name</Label>
					<Input id="setup-oidc-name" bind:value={oidcForm.name} />
				</div>
				<div class="grid gap-1.5">
					<Label for="setup-oidc-issuer">Issuer</Label>
					<Input
						id="setup-oidc-issuer"
						bind:value={oidcForm.issuer}
						placeholder="https://auth.example.com/realms/main"
					/>
				</div>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="setup-oidc-client-id">Client ID</Label>
					<Input id="setup-oidc-client-id" bind:value={oidcForm.clientId} />
				</div>
				<div class="grid gap-1.5">
					<Label for="setup-oidc-secret">Client secret</Label>
					<Input id="setup-oidc-secret" type="password" bind:value={oidcForm.clientSecret} />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="setup-oidc-scopes">Scopes</Label>
				<Input id="setup-oidc-scopes" bind:value={oidcForm.scopes} />
			</div>
			<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
				<span class="text-sm text-white">Enabled</span>
				<Switch bind:checked={oidcForm.enabled} />
			</div>
			<Button onclick={() => saveOidcProvider()} disabled={oidcSaving || advancing}>
				{oidcSaving ? 'Saving…' : 'Save OIDC provider'}
			</Button>
			{#if oidcError}<p class="text-xs text-destructive">{oidcError}</p>{/if}
			{#if oidcMessage}<p class="text-xs text-emerald-300">{oidcMessage}</p>{/if}
		</div>
	{/if}
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">AdGuard Home</p>
			<p class="text-xs text-muted-foreground">Internal DNS rewrite integration.</p>
		</div>
		<Switch bind:checked={adguard} />
	</div>
	{#if adguard}
		<div class="grid gap-3 rounded-md border border-border bg-hashi-bg-dark p-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="setup-adguard-name">Connection name</Label>
					<Input id="setup-adguard-name" bind:value={adguardForm.name} />
				</div>
				<div class="grid gap-1.5">
					<Label for="setup-adguard-url">Base URL</Label>
					<Input id="setup-adguard-url" bind:value={adguardForm.baseUrl} placeholder="http://adguard:3000" />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="setup-adguard-password">Admin password</Label>
				<Input id="setup-adguard-password" type="password" bind:value={adguardForm.password} />
			</div>
			<div class="flex gap-2">
				<Button onclick={() => saveAdGuard()} disabled={adguardSaving || advancing}>
					{adguardSaving ? 'Saving…' : 'Save AdGuard connection'}
				</Button>
				<Button variant="outline" href="/adguard">Open full AdGuard settings</Button>
			</div>
			{#if adguardError}<p class="text-xs text-destructive">{adguardError}</p>{/if}
			{#if adguardMessage}<p class="text-xs text-emerald-300">{adguardMessage}</p>{/if}
		</div>
	{/if}
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Internal agent DNS</p>
			<p class="text-xs text-muted-foreground">AdGuard rewrites for Pulse agents.</p>
		</div>
		<Switch bind:checked={internalAgentDns} />
	</div>
	{#if internalAgentDns}
		<div class="grid gap-3 rounded-md border border-border bg-hashi-bg-dark p-3">
			<div class="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-100">
				DNS-only: this does not create Traefik routers or reverse-proxy resources.
			</div>
			<div class="grid gap-1.5">
				<Label for="setup-internal-dns-domain">Domain</Label>
				<Input id="setup-internal-dns-domain" bind:value={internalDnsForm.domain} />
			</div>
			<div class="grid gap-1.5">
				<Label for="setup-internal-dns-connection">AdGuard connection ID</Label>
				<Input id="setup-internal-dns-connection" bind:value={internalDnsForm.adGuardConnectionId} />
			</div>
			<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
				<span class="text-sm text-white">Enabled</span>
				<Switch bind:checked={internalDnsForm.enabled} />
			</div>
			<Button onclick={() => saveInternalAgentDns()} disabled={internalDnsSaving || advancing}>
				{internalDnsSaving ? 'Saving...' : 'Save internal DNS'}
			</Button>
			{#if internalDnsError}<p class="text-xs text-destructive">{internalDnsError}</p>{/if}
			{#if internalDnsMessage}<p class="text-xs text-emerald-300">{internalDnsMessage}</p>{/if}
		</div>
	{/if}
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Notification provider</p>
			<p class="text-xs text-muted-foreground">Incident and sync notifications.</p>
		</div>
		<Switch bind:checked={notifications} />
	</div>
	{#if notifications}
		<div class="grid gap-3 rounded-md border border-border bg-hashi-bg-dark p-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="setup-notify-name">Provider name</Label>
					<Input id="setup-notify-name" bind:value={notificationForm.name} />
				</div>
				<div class="grid gap-1.5">
					<Label for="setup-notify-type">Type</Label>
					<select
						id="setup-notify-type"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={notificationForm.type}
					>
						<option value="smtp">SMTP email</option>
						<option value="telegram">Telegram bot</option>
						<option value="discord">Discord bot</option>
					</select>
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="setup-notify-settings">Settings JSON</Label>
				<Input
					id="setup-notify-settings"
					bind:value={notificationForm.settingsJson}
					placeholder="Provider settings JSON"
					class="font-mono text-xs"
				/>
			</div>
			<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
				<span class="text-sm text-white">Enabled</span>
				<Switch bind:checked={notificationForm.enabled} />
			</div>
			<div class="flex gap-2">
				<Button onclick={() => saveNotificationProvider()} disabled={notificationSaving || advancing}>
					{notificationSaving ? 'Saving…' : 'Save notification provider'}
				</Button>
				<Button variant="outline" href="/settings">Open full notification settings</Button>
			</div>
			{#if notificationError}<p class="text-xs text-destructive">{notificationError}</p>{/if}
			{#if notificationMessage}<p class="text-xs text-emerald-300">{notificationMessage}</p>{/if}
		</div>
	{/if}
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Cap CAPTCHA</p>
			<p class="text-xs text-muted-foreground">Self-hosted challenge integration.</p>
		</div>
		<Switch bind:checked={captcha} />
	</div>
	{#if captcha}
		<div class="grid gap-3 rounded-md border border-border bg-hashi-bg-dark p-3">
			<CaptchaSettings />
		</div>
	{/if}
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Blocklist sources</p>
			<p class="text-xs text-muted-foreground">Recommended and custom abuse feeds.</p>
		</div>
		<Switch bind:checked={blocklists} />
	</div>
	{#if blocklists}
		<div class="grid gap-4 rounded-md border border-border bg-hashi-bg-dark p-3">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<div>
					<p class="text-sm text-white">Recommended feeds</p>
					<p class="text-xs text-muted-foreground">
						Select feeds, preview parsed entries, then enable the previewed selection.
					</p>
				</div>
				<Button variant="outline" onclick={() => loadBlocklistSources()} disabled={blocklistsLoading || advancing}>
					{blocklistsLoading ? 'Loading...' : 'Reload'}
				</Button>
			</div>
			{#if blocklistError}<p class="text-xs text-destructive">{blocklistError}</p>{/if}
			{#if blocklistMessage}<p class="text-xs text-emerald-300">{blocklistMessage}</p>{/if}
			{#if blocklistsLoading && recommendedBlocklistSources.length === 0}
				<p class="text-xs text-muted-foreground">Loading recommended blocklists...</p>
			{:else if recommendedBlocklistSources.length === 0}
				<p class="text-xs text-muted-foreground">No recommended blocklists are available yet.</p>
			{:else}
				<div class="grid gap-2">
					{#each recommendedBlocklistSources as source (source.id)}
						<div class="grid gap-3 rounded-md border border-border px-3 py-2">
							<div class="flex flex-wrap items-start justify-between gap-3">
								<label class="flex min-w-0 flex-1 items-start gap-2 text-sm">
									<input
										class="mt-1"
										type="checkbox"
										checked={selectedBlocklistIds.includes(source.id)}
										disabled={source.enabled || advancing}
										onchange={(event) => setBlocklistSelected(source.id, eventChecked(event))}
									/>
									<span class="min-w-0">
										<span class="block text-white">{source.name}</span>
										<span class="block break-all text-xs text-muted-foreground">{source.sourceUrl}</span>
									</span>
								</label>
								<div class="grid min-w-36 gap-1">
									<Label for={`setup-blocklist-enforcement-${source.id}`}>Enforcement</Label>
									<select
										id={`setup-blocklist-enforcement-${source.id}`}
										class="h-9 rounded-md border border-border bg-background px-2 text-sm text-white"
										value={selectedEnforcementMode(source)}
										disabled={source.enabled || hasUnsupportedParserOptions(source) || advancing}
										onchange={(event) => setBlocklistEnforcement(source, eventValue(event))}
									>
										<option value="observe">Observe</option>
										<option value="middleware">Middleware</option>
										<option value="firewall">Firewall</option>
									</select>
								</div>
							</div>
							<p class="text-xs text-muted-foreground">{blocklistWarning(source)}</p>
							{#if blocklistFormatNote(source)}
								<p class="text-xs text-muted-foreground">{blocklistFormatNote(source)}</p>
							{/if}
							{#if hasUnsupportedParserOptions(source)}
								<p class="text-xs text-amber-200">
									This feed keeps its seeded enforcement mode during setup to preserve parser metadata.
								</p>
							{/if}
							<div class="flex flex-wrap items-center gap-2">
								<Button
									variant="outline"
									onclick={() => previewBlocklistSource(source)}
									disabled={blocklistPreviewingId === source.id || advancing}
								>
									{blocklistPreviewingId === source.id ? 'Previewing...' : 'Preview parsed entries'}
								</Button>
								{#if source.enabled}
									<span class="text-xs text-emerald-300">Enabled</span>
								{:else if previewBySourceId[source.id]}
									<span class="text-xs text-muted-foreground">
										Parsed {previewBySourceId[source.id].parsedCount}, ignored {previewBySourceId[source.id].ignoredCount},
										errors {previewBySourceId[source.id].errorCount}
									</span>
								{:else if selectedBlocklistIds.includes(source.id)}
									<span class="text-xs text-amber-200">Preview required before enable.</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}

			<div class="grid gap-3 rounded-md border border-border px-3 py-2">
				<p class="text-sm text-white">Custom URL</p>
				<div class="grid grid-cols-2 gap-3">
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-name">Name</Label>
						<Input id="setup-blocklist-name" bind:value={customBlocklistForm.name} />
					</div>
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-url">Source URL</Label>
						<Input
							id="setup-blocklist-url"
							bind:value={customBlocklistForm.sourceUrl}
							placeholder="https://example.test/feed.txt"
						/>
					</div>
				</div>
				<div class="grid grid-cols-3 gap-3">
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-format">Format</Label>
						<select
							id="setup-blocklist-format"
							class="h-9 rounded-md border border-border bg-background px-2 text-sm text-white"
							bind:value={customBlocklistForm.format}
						>
							<option value="text">Text</option>
							<option value="netset">Netset</option>
							<option value="csv">CSV</option>
							<option value="tsv">TSV</option>
							<option value="json">JSON</option>
							<option value="json_lines">JSON lines</option>
						</select>
					</div>
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-custom-enforcement">Enforcement</Label>
						<select
							id="setup-blocklist-custom-enforcement"
							class="h-9 rounded-md border border-border bg-background px-2 text-sm text-white"
							bind:value={customBlocklistForm.enforcementMode}
						>
							<option value="observe">Observe</option>
							<option value="middleware">Middleware</option>
							<option value="firewall">Firewall</option>
						</select>
					</div>
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-refresh">Refresh hours</Label>
						<Input
							id="setup-blocklist-refresh"
							type="number"
							min="1"
							max="168"
							bind:value={customBlocklistForm.refreshIntervalHours}
						/>
					</div>
				</div>
				<div class="grid grid-cols-3 gap-3">
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-csv-column">CSV/TSV column</Label>
						<Input id="setup-blocklist-csv-column" bind:value={customBlocklistForm.csvColumnIndex} />
					</div>
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-json-array">JSON array field</Label>
						<Input id="setup-blocklist-json-array" bind:value={customBlocklistForm.jsonArrayField} />
					</div>
					<div class="grid gap-1.5">
						<Label for="setup-blocklist-json-value">JSON value field</Label>
						<Input id="setup-blocklist-json-value" bind:value={customBlocklistForm.jsonValueField} />
					</div>
				</div>
				<div class="grid gap-2 sm:grid-cols-2">
					<label class="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-xs">
						<input type="checkbox" bind:checked={customBlocklistForm.canFirewallEnforce} />
						Can firewall enforce
					</label>
					<label class="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-xs">
						<input type="checkbox" bind:checked={customBlocklistForm.allowHttp} />
						Allow HTTP source with warning
					</label>
				</div>
				<div class="flex flex-wrap gap-2">
					<Button
						variant="outline"
						onclick={() => createCustomBlocklistSource()}
						disabled={blocklistSaving || advancing}
					>
						{blocklistSaving ? 'Saving...' : 'Add custom and preview'}
					</Button>
					<Button
						onclick={() => enablePreviewedBlocklists()}
						disabled={blocklistSaving || advancing || !selectedBlocklistsReady}
					>
						Enable selected previewed sources
					</Button>
					<Button variant="outline" href="/security">Open full security settings</Button>
				</div>
				{#if customBlocklistSources.length > 0}
					<div class="grid gap-2">
						<p class="text-xs text-muted-foreground">Custom sources created during setup or earlier:</p>
						{#each customBlocklistSources as source (source.id)}
							<div class="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border px-3 py-2 text-xs">
								<label class="flex min-w-0 items-center gap-2">
									<input
										type="checkbox"
										checked={selectedBlocklistIds.includes(source.id)}
										disabled={source.enabled || advancing}
										onchange={(event) => setBlocklistSelected(source.id, eventChecked(event))}
									/>
									<span class="break-all">{source.name}: {source.sourceUrl}</span>
								</label>
								<Button
									variant="outline"
									onclick={() => previewBlocklistSource(source)}
									disabled={blocklistPreviewingId === source.id || advancing}
								>
									{blocklistPreviewingId === source.id ? 'Previewing...' : 'Preview'}
								</Button>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">GeoLite2 databases</p>
			<p class="text-xs text-muted-foreground">MaxMind account for abuse geo signals.</p>
		</div>
		<Switch bind:checked={geoip} />
	</div>
	{#if geoip}
		<div class="grid gap-3 rounded-md border border-border bg-hashi-bg-dark p-3">
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="setup-geoip-account">Account ID</Label>
					<Input id="setup-geoip-account" bind:value={geoipForm.accountId} />
				</div>
				<div class="grid gap-1.5">
					<Label for="setup-geoip-license">License key</Label>
					<Input id="setup-geoip-license" type="password" bind:value={geoipForm.licenseKey} />
				</div>
			</div>
			<div class="grid gap-1.5">
				<Label for="setup-geoip-interval">Update interval (hours)</Label>
				<Input
					id="setup-geoip-interval"
					type="number"
					min="12"
					max="168"
					bind:value={geoipForm.updateIntervalHours}
				/>
			</div>
			<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
				<span class="text-sm text-white">Automatic updates</span>
				<Switch bind:checked={geoipForm.enabled} />
			</div>
			<div class="flex gap-2">
				<Button onclick={() => saveGeoIpSettings(false)} disabled={geoipSaving || advancing}>
					{geoipSaving && !geoipUpdating ? 'Saving...' : 'Save GeoIP settings'}
				</Button>
				<Button
					variant="outline"
					onclick={() => saveGeoIpSettings(true)}
					disabled={geoipSaving || geoipUpdating || advancing}
				>
					{geoipUpdating ? 'Updating...' : 'Save and update'}
				</Button>
				<Button
					variant="outline"
					href="https://www.maxmind.com/en/geolite2/signup"
					target="_blank"
					rel="noreferrer"
				>
					Open MaxMind signup
				</Button>
				<Button variant="outline" href="/security">Open security dashboard</Button>
			</div>
			{#if geoipError}<p class="text-xs text-destructive">{geoipError}</p>{/if}
			{#if geoipMessage}<p class="text-xs text-emerald-300">{geoipMessage}</p>{/if}
		</div>
	{/if}

	<div class="flex justify-end gap-2 pt-2">
		<Button variant="outline" onclick={() => oncomplete()} disabled={advancing}>Skip optional</Button>
	</div>
</div>
