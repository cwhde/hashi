<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import CaptchaSettings from '$lib/components/settings/CaptchaSettings.svelte';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let oidc = $state(false);
	let adguard = $state(false);
	let notifications = $state(false);
	let geoip = $state(false);
	let captcha = $state(false);

	let adguardSaving = $state(false);
	let adguardMessage = $state<string | null>(null);
	let adguardError = $state<string | null>(null);
	let adguardForm = $state({
		name: 'home-adguard',
		baseUrl: 'http://127.0.0.1:3000',
		password: ''
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
			adguardForm.password = '';
		} catch (e) {
			adguardError = e instanceof ApiRequestError ? e.message : 'Failed to save AdGuard connection';
		} finally {
			adguardSaving = false;
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
