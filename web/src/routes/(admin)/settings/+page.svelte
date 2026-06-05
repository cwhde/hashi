<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import {
		DEFAULT_WIDGETS,
		loadWidgetPrefs,
		parseWidgetPrefsJson,
		saveWidgetPrefs,
		type WidgetPrefs
	} from '$lib/overview/widgets';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Settings as SettingsIcon } from 'lucide-svelte';
	import NotificationsSettings from '$lib/components/settings/NotificationsSettings.svelte';
	import type { AdGuardConnection, AdGuardRewritePlan } from '$lib/api/types';

	let saving = $state(false);
	let message = $state<string | null>(null);
	let widgetSaving = $state(false);
	let widgetMessage = $state<string | null>(null);
	let widgetPrefs = $state<WidgetPrefs>(loadWidgetPrefs());
	let geoipSaving = $state(false);
	let geoipUpdating = $state(false);
	let geoipMessage = $state<string | null>(null);
	let internalDnsSaving = $state(false);
	let internalDnsSyncing = $state(false);
	let internalDnsMessage = $state<string | null>(null);
	let internalDnsPlan = $state<AdGuardRewritePlan | null>(null);
	let adguardConnections = $state<AdGuardConnection[]>([]);
	let internalDnsForm = $state({
		enabled: false,
		domain: 'hashi.home.arpa',
		keepLastRewriteWhenAgentStale: true,
		adGuardConnectionId: '',
		lastSyncStatus: 'never_run',
		lastAppliedHash: null as string | null
	});
	let geoipForm = $state({
		enabled: false,
		accountId: '',
		licenseKey: '',
		updateIntervalHours: 72,
		hasLicenseKey: false,
		lastUpdateStatus: 'never_run',
		lastUpdateMessage: null as string | null,
		lastUpdateAtUtc: null as string | null,
		nextUpdateAtUtc: null as string | null,
		databaseAvailable: false,
		databases: [] as import('$lib/api/types').GeoIpDatabase[]
	});
	let form = $state({
		rootDomain: '',
		adminDomain: '',
		internalUrl: '',
		defaultSyncIntervalMinutes: 60,
		publicDashboardEnabled: true,
		publicStatusEnabled: true,
		theme: 'dark'
	});

	onMount(async () => {
		try {
			const settings = await api.getGeneralSettings();
			form = {
				rootDomain: settings.rootDomain ?? '',
				adminDomain: settings.adminDomain ?? '',
				internalUrl: settings.internalUrl ?? '',
				defaultSyncIntervalMinutes: Number(settings.defaultSyncIntervalMinutes),
				publicDashboardEnabled: settings.publicDashboardEnabled,
				publicStatusEnabled: settings.publicStatusEnabled,
				theme: settings.theme ?? 'dark'
			};
		} catch {
			// offline dev
		}

		try {
			const dashboard = await api.getDashboardSettings();
			widgetPrefs = parseWidgetPrefsJson(dashboard.overviewWidgetsJson);
			saveWidgetPrefs(widgetPrefs);
		} catch {
			// offline dev
		}

		try {
			await loadGeoIpSettings();
		} catch {
			// offline dev
		}

		try {
			const [dnsSettings, connections] = await Promise.all([
				api.getInternalAgentDnsSettings(),
				api.listAdGuardConnections().catch(() => [])
			]);
			adguardConnections = connections;
			internalDnsForm = {
				enabled: dnsSettings.enabled,
				domain: dnsSettings.domain,
				keepLastRewriteWhenAgentStale: dnsSettings.keepLastRewriteWhenAgentStale,
				adGuardConnectionId: dnsSettings.adGuardConnectionId ?? '',
				lastSyncStatus: dnsSettings.lastSyncStatus,
				lastAppliedHash: dnsSettings.lastAppliedHash
			};
		} catch {
			// offline dev
		}
	});

	async function save() {
		saving = true;
		message = null;
		try {
			await api.updateGeneralSettings({
				rootDomain: form.rootDomain || null,
				adminDomain: form.adminDomain || null,
				internalUrl: form.internalUrl || null,
				defaultSyncIntervalMinutes: form.defaultSyncIntervalMinutes,
				publicDashboardEnabled: form.publicDashboardEnabled,
				publicStatusEnabled: form.publicStatusEnabled,
				theme: form.theme || null
			});
			message = 'Settings saved.';
		} catch (e) {
			message = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	async function setWidgetEnabled(id: string, enabled: boolean) {
		widgetSaving = true;
		widgetMessage = null;
		widgetPrefs = {
			...widgetPrefs,
			enabled: { ...widgetPrefs.enabled, [id]: enabled }
		};
		saveWidgetPrefs(widgetPrefs);
		try {
			await api.updateDashboardSettings({ overviewWidgetsJson: JSON.stringify(widgetPrefs) });
			widgetMessage = 'Widget preferences saved.';
		} catch (e) {
			widgetMessage = e instanceof Error ? e.message : 'Failed to save widget preferences';
		} finally {
			widgetSaving = false;
		}
	}

	async function loadGeoIpSettings() {
		const settings = await api.getGeoIpSettings();
		geoipForm = {
			enabled: settings.enabled,
			accountId: settings.accountId ?? '',
			licenseKey: '',
			updateIntervalHours: Number(settings.updateIntervalHours),
			hasLicenseKey: settings.hasLicenseKey,
			lastUpdateStatus: settings.lastUpdateStatus,
			lastUpdateMessage: settings.lastUpdateMessage,
			lastUpdateAtUtc: settings.lastUpdateAtUtc,
			nextUpdateAtUtc: settings.nextUpdateAtUtc,
			databaseAvailable: settings.databaseAvailable,
			databases: settings.databases ?? []
		};
	}

	async function saveGeoIpSettings() {
		geoipSaving = true;
		geoipMessage = null;
		try {
			const settings = await api.updateGeoIpSettings({
				enabled: geoipForm.enabled,
				accountId: geoipForm.accountId || null,
				licenseKey: geoipForm.licenseKey || null,
				updateIntervalHours: geoipForm.updateIntervalHours
			});
			geoipForm.licenseKey = '';
			geoipForm.hasLicenseKey = settings.hasLicenseKey;
			geoipForm.lastUpdateStatus = settings.lastUpdateStatus;
			geoipForm.lastUpdateMessage = settings.lastUpdateMessage;
			geoipForm.nextUpdateAtUtc = settings.nextUpdateAtUtc;
			geoipForm.databaseAvailable = settings.databaseAvailable;
			geoipForm.databases = settings.databases ?? [];
			geoipMessage = 'GeoIP settings saved.';
		} catch (e) {
			geoipMessage = e instanceof Error ? e.message : 'Failed to save GeoIP settings';
		} finally {
			geoipSaving = false;
		}
	}

	async function runGeoIpUpdate() {
		geoipUpdating = true;
		geoipMessage = null;
		try {
			const result = await api.runGeoIpUpdate();
			geoipForm.lastUpdateStatus = result.status;
			geoipForm.lastUpdateMessage = result.message;
			geoipForm.databases = result.databases ?? [];
			geoipMessage = result.message ?? 'GeoIP update completed.';
			await loadGeoIpSettings();
		} catch (e) {
			geoipMessage = e instanceof Error ? e.message : 'GeoIP update failed';
		} finally {
			geoipUpdating = false;
		}
	}

	async function saveInternalDnsSettings() {
		internalDnsSaving = true;
		internalDnsMessage = null;
		try {
			const settings = await api.updateInternalAgentDnsSettings({
				enabled: internalDnsForm.enabled,
				domain: internalDnsForm.domain,
				keepLastRewriteWhenAgentStale: internalDnsForm.keepLastRewriteWhenAgentStale,
				adGuardConnectionId: internalDnsForm.adGuardConnectionId || null,
				agents: null
			});
			internalDnsForm.lastSyncStatus = settings.lastSyncStatus;
			internalDnsForm.lastAppliedHash = settings.lastAppliedHash;
			internalDnsMessage = 'Internal DNS settings saved.';
		} catch (e) {
			internalDnsMessage = e instanceof Error ? e.message : 'Failed to save internal DNS settings';
		} finally {
			internalDnsSaving = false;
		}
	}

	async function previewInternalDnsSync() {
		internalDnsSyncing = true;
		internalDnsMessage = null;
		try {
			internalDnsPlan = await api.previewInternalAgentDnsSync();
			internalDnsMessage = `${internalDnsPlan.changes.length} pending rewrite change${internalDnsPlan.changes.length === 1 ? '' : 's'}.`;
		} catch (e) {
			internalDnsMessage = e instanceof Error ? e.message : 'Failed to preview internal DNS sync';
		} finally {
			internalDnsSyncing = false;
		}
	}

	async function applyInternalDnsSync() {
		if (!internalDnsPlan) return;
		internalDnsSyncing = true;
		internalDnsMessage = null;
		try {
			const result = await api.applyInternalAgentDnsSync({
				planId: internalDnsPlan.planId,
				confirmDestructive: internalDnsPlan.requiresConfirmation
			});
			internalDnsForm.lastSyncStatus = result.status;
			internalDnsMessage = result.succeeded ? 'Internal DNS sync applied.' : (result.message ?? result.status);
			internalDnsPlan = null;
		} catch (e) {
			internalDnsMessage = e instanceof Error ? e.message : 'Failed to apply internal DNS sync';
		} finally {
			internalDnsSyncing = false;
		}
	}

	function formatDate(value: string | null) {
		if (!value) return 'Never';
		return new Date(value).toLocaleString();
	}

	function formatSize(value: string | number | null) {
		if (!value) return 'No file';
		return `${Math.round(Number(value) / 1024)} KB`;
	}
</script>

<AdminSectionPage
	title="Settings"
	description="General preferences, overview widgets, and asset overrides."
	icon={SettingsIcon}
>
	<div class="grid max-w-xl gap-6">
		<PanelSection title="General" description="Domain, sync interval, and public page toggles.">
			<div class="grid gap-4">
				<div class="grid gap-1.5">
					<Label for="settings-root">Root domain</Label>
					<Input id="settings-root" bind:value={form.rootDomain} />
				</div>
				<div class="grid gap-1.5">
					<Label for="settings-admin">Admin domain</Label>
					<Input id="settings-admin" bind:value={form.adminDomain} />
				</div>
				<div class="grid gap-1.5">
					<Label for="settings-internal">Internal URL</Label>
					<Input id="settings-internal" bind:value={form.internalUrl} />
				</div>
				<div class="grid gap-1.5">
					<Label for="settings-sync">Default sync interval (minutes)</Label>
					<Input
						id="settings-sync"
						type="number"
						min="5"
						bind:value={form.defaultSyncIntervalMinutes}
					/>
				</div>
				<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
					<span class="text-sm text-white">Public dashboard</span>
					<Switch bind:checked={form.publicDashboardEnabled} />
				</div>
				<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
					<span class="text-sm text-white">Public status page</span>
					<Switch bind:checked={form.publicStatusEnabled} />
				</div>
				<div class="grid gap-1.5">
					<Label for="settings-theme">Theme</Label>
					<Input id="settings-theme" bind:value={form.theme} />
				</div>
				{#if message}
					<p class="text-xs text-muted-foreground">{message}</p>
				{/if}
				<Button onclick={() => save()} disabled={saving}>
					{saving ? 'Saving…' : 'Save settings'}
				</Button>
			</div>
		</PanelSection>

		<PanelSection title="GeoIP" description="MaxMind GeoLite2 update settings and database status.">
			<div class="grid gap-4">
				<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
					<div>
						<p class="text-sm text-white">Automatic updates</p>
						<p class="text-xs text-muted-foreground">
							{geoipForm.databaseAvailable ? 'Databases available' : 'Databases unavailable'}
						</p>
					</div>
					<Switch bind:checked={geoipForm.enabled} />
				</div>
				<div class="grid grid-cols-2 gap-3">
					<div class="grid gap-1.5">
						<Label for="settings-geoip-account">Account ID</Label>
						<Input id="settings-geoip-account" bind:value={geoipForm.accountId} />
					</div>
					<div class="grid gap-1.5">
						<Label for="settings-geoip-license">
							License key{geoipForm.hasLicenseKey ? ' stored' : ''}
						</Label>
						<Input
							id="settings-geoip-license"
							type="password"
							bind:value={geoipForm.licenseKey}
							placeholder={geoipForm.hasLicenseKey ? 'Leave blank to keep current key' : ''}
						/>
					</div>
				</div>
				<div class="grid gap-1.5">
					<Label for="settings-geoip-interval">Update interval (hours)</Label>
					<Input
						id="settings-geoip-interval"
						type="number"
						min="12"
						max="168"
						bind:value={geoipForm.updateIntervalHours}
					/>
				</div>
				<div class="grid gap-2 text-xs text-muted-foreground">
					<p>Status: {geoipForm.lastUpdateStatus}</p>
					<p>Last update: {formatDate(geoipForm.lastUpdateAtUtc)}</p>
					<p>Next update: {formatDate(geoipForm.nextUpdateAtUtc)}</p>
					{#if geoipForm.lastUpdateMessage}
						<p>{geoipForm.lastUpdateMessage}</p>
					{/if}
				</div>
				<div class="grid gap-2">
					{#each geoipForm.databases as db (db.editionId)}
						<div class="grid grid-cols-[1fr_auto] gap-3 rounded-md border border-border px-3 py-2">
							<div class="min-w-0">
								<p class="truncate text-sm text-white">{db.editionId}</p>
								<p class="truncate text-xs text-muted-foreground">
									{db.status} - {formatSize(db.sizeBytes)}
								</p>
							</div>
							<p class="text-right text-xs text-muted-foreground">{formatDate(db.lastDownloadedAtUtc)}</p>
						</div>
					{/each}
				</div>
				{#if geoipMessage}
					<p class="text-xs text-muted-foreground">{geoipMessage}</p>
				{/if}
				<div class="flex gap-2">
					<Button onclick={() => saveGeoIpSettings()} disabled={geoipSaving || geoipUpdating}>
						{geoipSaving ? 'Saving...' : 'Save GeoIP settings'}
					</Button>
					<Button
						variant="outline"
						onclick={() => runGeoIpUpdate()}
						disabled={geoipSaving || geoipUpdating || !geoipForm.enabled}
					>
						{geoipUpdating ? 'Updating...' : 'Update now'}
					</Button>
				</div>
			</div>
		</PanelSection>

		<PanelSection title="Internal agent DNS" description="Pulse agent rewrites for AdGuard Home.">
			<div class="grid gap-4">
				<div class="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-100">
					DNS-only: this does not create Traefik routers or reverse-proxy resources.
				</div>
				<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
					<div>
						<p class="text-sm text-white">Enable internal agent DNS</p>
						<p class="text-xs text-muted-foreground">Status: {internalDnsForm.lastSyncStatus}</p>
					</div>
					<Switch bind:checked={internalDnsForm.enabled} />
				</div>
				<div class="grid gap-1.5">
					<Label for="settings-internal-dns-domain">Domain</Label>
					<Input id="settings-internal-dns-domain" bind:value={internalDnsForm.domain} />
				</div>
				<div class="grid gap-1.5">
					<Label for="settings-internal-dns-adguard">AdGuard connection</Label>
					<select
						id="settings-internal-dns-adguard"
						class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
						bind:value={internalDnsForm.adGuardConnectionId}
					>
						<option value="">Select connection</option>
						{#each adguardConnections as connection (connection.id)}
							<option value={connection.id}>{connection.name}</option>
						{/each}
					</select>
				</div>
				<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
					<span class="text-sm text-white">Keep last rewrite when stale</span>
					<Switch bind:checked={internalDnsForm.keepLastRewriteWhenAgentStale} />
				</div>
				{#if internalDnsForm.lastAppliedHash}
					<p class="truncate font-mono text-xs text-muted-foreground">
						Last hash: {internalDnsForm.lastAppliedHash}
					</p>
				{/if}
				{#if internalDnsPlan}
					<div class="grid gap-2 rounded-md border border-border p-3 text-xs">
						{#each internalDnsPlan.changes as change}
							<p class="font-mono text-muted-foreground">
								{change.kind} {change.domain}: {change.currentAnswer ?? 'none'} -> {change.desiredAnswer ?? 'none'}
							</p>
						{/each}
					</div>
				{/if}
				{#if internalDnsMessage}
					<p class="text-xs text-muted-foreground">{internalDnsMessage}</p>
				{/if}
				<div class="flex flex-wrap gap-2">
					<Button onclick={() => saveInternalDnsSettings()} disabled={internalDnsSaving}>
						{internalDnsSaving ? 'Saving...' : 'Save internal DNS'}
					</Button>
					<Button
						variant="outline"
						onclick={() => previewInternalDnsSync()}
						disabled={internalDnsSyncing || !internalDnsForm.enabled}
					>
						{internalDnsSyncing ? 'Previewing...' : 'Preview sync'}
					</Button>
					<Button
						variant="outline"
						onclick={() => applyInternalDnsSync()}
						disabled={internalDnsSyncing || !internalDnsPlan}
					>
						Apply preview
					</Button>
				</div>
			</div>
		</PanelSection>

		<PanelSection
			title="Overview widgets"
			description="Toggle and reorder default overview widgets."
		>
			<ul class="space-y-2">
				{#each DEFAULT_WIDGETS as widget (widget.id)}
					<li class="flex items-center gap-3 rounded-md border border-border px-3 py-2">
						<Checkbox
							checked={widgetPrefs.enabled[widget.id]}
							disabled={widgetSaving}
							onCheckedChange={(checked) => setWidgetEnabled(widget.id, checked === true)}
							id={`widget-${widget.id}`}
						/>
						<Label for={`widget-${widget.id}`} class="min-w-0 flex-1">
							<span class="block text-sm text-white">{widget.title}</span>
							<span class="block truncate text-xs text-muted-foreground">{widget.description}</span>
						</Label>
					</li>
				{/each}
			</ul>
			{#if widgetMessage}
				<p class="mt-3 text-xs text-muted-foreground">{widgetMessage}</p>
			{/if}
		</PanelSection>

		<PanelSection
			title="Notifications"
			description="Alert providers for status monitoring (SMTP, Telegram, Discord)."
		>
			<NotificationsSettings />
		</PanelSection>
	</div>
</AdminSectionPage>
