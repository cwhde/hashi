<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
	import type { NotificationProvider, NotificationRoute } from '$lib/api/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';

	let providers = $state<NotificationProvider[]>([]);
	let routes = $state<NotificationRoute[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let discoveringChat = $state(false);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let form = $state({
		name: '',
		type: 'telegram',
		enabled: true,
		telegramBotToken: '',
		telegramChatId: '',
		discordWebhookUrl: '',
		smtpHost: '',
		smtpPort: 587,
		smtpUsername: '',
		smtpPassword: '',
		smtpFrom: '',
		smtpTo: '',
		smtpUseTls: true
	});
	let routeForm = $state({
		name: '',
		providerId: '',
		eventKind: 'all',
		severity: 'info',
		matchJson: '{}',
		enabled: true,
		cooldownMinutes: 0,
		sendRecovery: true
	});
	const routeMatchPlaceholder = '{"name":"my-endpoint"}';

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			providers = await api.listNotificationProviders();
			routes = await api.listNotificationRoutes();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load notification settings';
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

	function resetProviderFields() {
		form.telegramBotToken = '';
		form.telegramChatId = '';
		form.discordWebhookUrl = '';
		form.smtpHost = '';
		form.smtpPort = 587;
		form.smtpUsername = '';
		form.smtpPassword = '';
		form.smtpFrom = '';
		form.smtpTo = '';
		form.smtpUseTls = true;
	}

	function formatCooldown(value: NotificationRoute['cooldownMinutes']) {
		const cooldown = Number(value);
		return cooldown > 0 ? `${cooldown}m` : 'off';
	}

	function buildSettingsJson(): string | null {
		if (form.type === 'telegram') {
			if (!form.telegramBotToken.trim()) {
				error = 'Telegram bot token is required.';
				return null;
			}
			if (!form.telegramChatId.trim()) {
				error = 'Telegram chat ID is required.';
				return null;
			}

			return JSON.stringify({
				botToken: form.telegramBotToken.trim(),
				chatId: form.telegramChatId.trim()
			});
		}

		if (form.type === 'discord') {
			if (!form.discordWebhookUrl.trim()) {
				error = 'Discord webhook URL is required.';
				return null;
			}

			return JSON.stringify({
				webhookUrl: form.discordWebhookUrl.trim()
			});
		}

		if (!form.smtpHost.trim()) {
			error = 'SMTP host is required.';
			return null;
		}
		if (!form.smtpUsername.trim()) {
			error = 'SMTP username is required.';
			return null;
		}
		if (!form.smtpPassword.trim()) {
			error = 'SMTP password is required.';
			return null;
		}
		if (!form.smtpFrom.trim() || !form.smtpTo.trim()) {
			error = 'SMTP from/to addresses are required.';
			return null;
		}

		return JSON.stringify({
			host: form.smtpHost.trim(),
			port: form.smtpPort,
			username: form.smtpUsername.trim(),
			password: form.smtpPassword,
			from: form.smtpFrom.trim(),
			to: form.smtpTo.trim(),
			useTls: form.smtpUseTls
		});
	}

	async function createProvider(runSmtpTestAfterCreate = false) {
		if (!form.name.trim()) return;
		const settingsJson = buildSettingsJson();
		if (!settingsJson) return;

		saving = true;
		error = null;
		message = null;
		try {
			const created = await withReauth(() =>
				api.createNotificationProvider({
					name: form.name.trim(),
					type: form.type,
					settingsJson,
					enabled: form.enabled
				})
			);
			if (!created) {
				return;
			}

			message = `Provider "${created.name}" created.`;
			if (runSmtpTestAfterCreate && form.type === 'smtp') {
				const testResult = await withReauth(() =>
					api.testNotificationProvider(created.id, {
						subject: 'Hashi SMTP setup test',
						body: 'SMTP setup test from Hashi notifications settings.'
					})
				);
				if (testResult?.sent) {
					message = `Provider "${created.name}" created and SMTP test email sent.`;
				} else if (testResult) {
					message = `Provider created, but SMTP test failed: ${testResult.error ?? 'unknown error'}`;
				}
			}

			form.name = '';
			resetProviderFields();
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create provider';
		} finally {
			saving = false;
		}
	}

	async function discoverTelegramChat() {
		if (!form.telegramBotToken.trim()) {
			error = 'Enter the Telegram bot token first.';
			return;
		}

		discoveringChat = true;
		error = null;
		message = null;
		try {
			const result = await withReauth(() => api.discoverTelegramChat(form.telegramBotToken.trim()));
			if (!result) {
				return;
			}
			if (result.found && result.chatId) {
				form.telegramChatId = result.chatId;
				message = result.chatTitle
					? `Discovered chat "${result.chatTitle}" (${result.chatId}).`
					: `Discovered chat ID ${result.chatId}.`;
			} else {
				error = result.error ?? 'No Telegram chats discovered yet.';
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to discover Telegram chat';
		} finally {
			discoveringChat = false;
		}
	}

	async function testProvider(providerId: string) {
		saving = true;
		error = null;
		message = null;
		try {
			const result = await withReauth(() =>
				api.testNotificationProvider(providerId, {
					subject: 'Hashi test alert',
					body: 'This is a test notification from Hashi.'
				})
			);
			if (result) {
				message = result.sent ? 'Test notification sent.' : `Test failed: ${result.error ?? 'unknown error'}`;
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Test send failed';
		} finally {
			saving = false;
		}
	}

	async function deleteProvider(providerId: string) {
		if (!confirm('Delete this notification provider?')) return;
		try {
			await withReauth(() => api.deleteNotificationProvider(providerId));
			message = 'Provider deleted.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to delete provider';
		}
	}

	async function createRoute() {
		if (!routeForm.name.trim() || !routeForm.providerId) return;
		saving = true;
		error = null;
		message = null;
		try {
			await withReauth(() =>
				api.createNotificationRoute({
					providerId: routeForm.providerId,
					name: routeForm.name.trim(),
					eventKind: routeForm.eventKind,
					severity: routeForm.severity,
					matchJson: routeForm.matchJson,
					enabled: routeForm.enabled,
					cooldownMinutes: Number(routeForm.cooldownMinutes),
					sendRecovery: routeForm.sendRecovery
				})
			);
			message = 'Route created.';
			routeForm.name = '';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create route';
		} finally {
			saving = false;
		}
	}

	async function deleteRoute(routeId: string) {
		if (!confirm('Delete this notification route?')) return;
		try {
			await withReauth(() => api.deleteNotificationRoute(routeId));
			message = 'Route deleted.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to delete route';
		}
	}
</script>

<div class="grid gap-6">
	<h3 class="text-sm font-semibold text-white">Notification Providers</h3>
	<div class="grid gap-3 rounded-md border border-border p-3">
		<div class="grid grid-cols-2 gap-3">
			<div class="grid gap-1.5">
				<Label for="notify-name">Name</Label>
				<Input id="notify-name" bind:value={form.name} placeholder="alerts-telegram" />
			</div>
			<div class="grid gap-1.5">
				<Label for="notify-type">Type</Label>
				<select
					id="notify-type"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={form.type}
					onchange={() => resetProviderFields()}
				>
					<option value="smtp">SMTP email</option>
					<option value="telegram">Telegram bot</option>
					<option value="discord">Discord webhook</option>
				</select>
			</div>
		</div>
		{#if form.type === 'telegram'}
			<div class="grid gap-2 rounded-md border border-border p-3">
				<p class="text-xs text-muted-foreground">
					Step 1: paste bot token. Step 2: message your bot. Step 3: discover chat ID.
				</p>
				<div class="grid gap-1.5">
					<Label for="notify-telegram-token">Bot token</Label>
					<Input id="notify-telegram-token" bind:value={form.telegramBotToken} placeholder="123456:ABC..." />
				</div>
				<div class="grid gap-1.5">
					<Label for="notify-telegram-chat-id">Chat ID</Label>
					<Input id="notify-telegram-chat-id" bind:value={form.telegramChatId} placeholder="-1001234567890" />
				</div>
				<Button
					variant="secondary"
					onclick={() => discoverTelegramChat()}
					disabled={saving || discoveringChat || !form.telegramBotToken.trim()}
				>
					{discoveringChat ? 'Discovering…' : 'Discover chat'}
				</Button>
			</div>
		{:else if form.type === 'discord'}
			<div class="grid gap-2 rounded-md border border-border p-3">
				<p class="text-xs text-muted-foreground">Paste your Discord incoming webhook URL.</p>
				<div class="grid gap-1.5">
					<Label for="notify-discord-webhook">Webhook URL</Label>
					<Input
						id="notify-discord-webhook"
						bind:value={form.discordWebhookUrl}
						placeholder="https://discord.com/api/webhooks/..."
					/>
				</div>
			</div>
		{:else}
			<div class="grid gap-2 rounded-md border border-border p-3">
				<p class="text-xs text-muted-foreground">Configure SMTP details and optionally send a setup test email.</p>
				<div class="grid grid-cols-2 gap-3">
					<div class="grid gap-1.5">
						<Label for="notify-smtp-host">Host</Label>
						<Input id="notify-smtp-host" bind:value={form.smtpHost} placeholder="smtp.example.com" />
					</div>
					<div class="grid gap-1.5">
						<Label for="notify-smtp-port">Port</Label>
						<Input id="notify-smtp-port" type="number" bind:value={form.smtpPort} />
					</div>
					<div class="grid gap-1.5">
						<Label for="notify-smtp-user">Username</Label>
						<Input id="notify-smtp-user" bind:value={form.smtpUsername} placeholder="smtp-user" />
					</div>
					<div class="grid gap-1.5">
						<Label for="notify-smtp-pass">Password</Label>
						<Input id="notify-smtp-pass" type="password" bind:value={form.smtpPassword} />
					</div>
					<div class="grid gap-1.5">
						<Label for="notify-smtp-from">From email</Label>
						<Input id="notify-smtp-from" bind:value={form.smtpFrom} placeholder="hashi@example.com" />
					</div>
					<div class="grid gap-1.5">
						<Label for="notify-smtp-to">To email</Label>
						<Input id="notify-smtp-to" bind:value={form.smtpTo} placeholder="admin@example.com" />
					</div>
				</div>
				<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
					<span class="text-sm text-white">Use TLS</span>
					<Switch bind:checked={form.smtpUseTls} />
				</div>
			</div>
		{/if}
		<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
			<span class="text-sm text-white">Enabled</span>
			<Switch bind:checked={form.enabled} />
		</div>
		<div class="flex flex-wrap gap-2">
			<Button onclick={() => createProvider()} disabled={saving || !form.name.trim()}>
				{saving ? 'Creating…' : 'Add provider'}
			</Button>
			{#if form.type === 'smtp'}
				<Button variant="secondary" onclick={() => createProvider(true)} disabled={saving || !form.name.trim()}>
					{saving ? 'Working…' : 'Add + send test email'}
				</Button>
			{/if}
		</div>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else if error}
		<p class="text-sm text-destructive">{error}</p>
	{:else if providers.length > 0}
		<div class="overflow-hidden rounded-md border border-border">
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Type</TableHead>
						<TableHead>Enabled</TableHead>
						<TableHead class="w-40"></TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each providers as provider (provider.id)}
						<TableRow>
							<TableCell>{provider.name}</TableCell>
							<TableCell>{provider.type}</TableCell>
							<TableCell>{provider.enabled ? 'yes' : 'no'}</TableCell>
							<TableCell class="space-x-2">
								<Button variant="ghost" size="sm" disabled={saving} onclick={() => testProvider(provider.id)}>
									Test
								</Button>
								<Button variant="ghost" size="sm" onclick={() => deleteProvider(provider.id)}>
									Delete
								</Button>
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		</div>
	{:else}
		<p class="text-sm text-muted-foreground">No notification providers configured.</p>
	{/if}

	<h3 class="text-sm font-semibold text-white">Notification Routes</h3>
	<div class="grid gap-3 rounded-md border border-border p-3">
		<div class="grid grid-cols-2 gap-3">
			<div class="grid gap-1.5">
				<Label for="route-name">Name</Label>
				<Input id="route-name" bind:value={routeForm.name} placeholder="critical-monitor-alerts" />
			</div>
			<div class="grid gap-1.5">
				<Label for="route-provider">Provider</Label>
				<select
					id="route-provider"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={routeForm.providerId}
				>
					<option value="">Select provider...</option>
					{#each providers as provider (provider.id)}
						<option value={provider.id}>{provider.name}</option>
					{/each}
				</select>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div class="grid gap-1.5">
				<Label for="route-event-kind">Event kind</Label>
				<select
					id="route-event-kind"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={routeForm.eventKind}
				>
					<option value="all">All</option>
					<option value="monitor">Monitor</option>
					<option value="security">Security</option>
				</select>
			</div>
			<div class="grid gap-1.5">
				<Label for="route-severity">Severity threshold</Label>
				<select
					id="route-severity"
					class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
					bind:value={routeForm.severity}
				>
					<option value="info">Info</option>
					<option value="warning">Warning</option>
					<option value="critical">Critical</option>
				</select>
			</div>
		</div>
		<div class="grid gap-1.5">
			<Label for="route-match-json">Match JSON (empty = all endpoints)</Label>
			<Input id="route-match-json" bind:value={routeForm.matchJson} placeholder={routeMatchPlaceholder} />
		</div>
		<div class="grid grid-cols-3 gap-3">
			<div class="grid gap-1.5">
				<Label for="route-cooldown">Cooldown (minutes, 0 = off)</Label>
				<Input id="route-cooldown" type="number" bind:value={routeForm.cooldownMinutes} />
			</div>
			<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
				<span class="text-sm text-white">Send recovery</span>
				<Switch bind:checked={routeForm.sendRecovery} />
			</div>
			<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
				<span class="text-sm text-white">Enabled</span>
				<Switch bind:checked={routeForm.enabled} />
			</div>
		</div>
		<Button onclick={() => createRoute()} disabled={saving || !routeForm.name.trim() || !routeForm.providerId}>
			{saving ? 'Creating...' : 'Add route'}
		</Button>
	</div>

	{#if routes.length > 0}
		<div class="overflow-hidden rounded-md border border-border">
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Event kind</TableHead>
						<TableHead>Severity</TableHead>
						<TableHead>Cooldown</TableHead>
						<TableHead>Recovery</TableHead>
						<TableHead>Enabled</TableHead>
						<TableHead class="w-20"></TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each routes as route (route.id)}
						<TableRow>
							<TableCell>{route.name}</TableCell>
							<TableCell>{route.eventKind}</TableCell>
							<TableCell>{route.severity}</TableCell>
							<TableCell>{formatCooldown(route.cooldownMinutes)}</TableCell>
							<TableCell>{route.sendRecovery ? 'yes' : 'no'}</TableCell>
							<TableCell>{route.enabled ? 'yes' : 'no'}</TableCell>
							<TableCell>
								<Button variant="ghost" size="sm" onclick={() => deleteRoute(route.id)}>
									Delete
								</Button>
							</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		</div>
	{/if}
	{#if message}
		<p class="text-xs text-emerald-300">{message}</p>
	{/if}
</div>
