<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
	import type { NotificationProvider } from '$lib/api/types';
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
	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);
	let form = $state({
		name: '',
		type: 'telegram',
		settingsJson: '{}',
		enabled: true
	});

	$effect(() => {
		void load();
	});

	async function load() {
		loading = true;
		error = null;
		try {
			providers = await api.listNotificationProviders();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load notification providers';
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

	async function createProvider() {
		if (!form.name) return;
		saving = true;
		error = null;
		message = null;
		try {
			const created = await withReauth(() =>
				api.createNotificationProvider({
					name: form.name,
					type: form.type,
					settingsJson: form.settingsJson || '{}',
					enabled: form.enabled
				})
			);
			if (created) {
				message = `Provider "${created.name}" created.`;
				form.name = '';
				form.settingsJson = '{}';
				await load();
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to create provider';
		} finally {
			saving = false;
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
</script>

<div class="grid gap-4">
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
				>
					<option value="smtp">SMTP email</option>
					<option value="telegram">Telegram bot</option>
					<option value="discord">Discord bot</option>
				</select>
			</div>
		</div>
		<div class="grid gap-1.5">
			<Label for="notify-settings">Settings JSON (stub)</Label>
			<Input
				id="notify-settings"
				bind:value={form.settingsJson}
				placeholder="Provider-specific JSON settings"
				class="font-mono text-xs"
			/>
		</div>
		<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
			<span class="text-sm text-white">Enabled</span>
			<Switch bind:checked={form.enabled} />
		</div>
		<Button onclick={() => createProvider()} disabled={saving || !form.name}>
			{saving ? 'Creating…' : 'Add provider'}
		</Button>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading providers…</p>
	{:else if error}
		<p class="text-sm text-destructive">{error}</p>
	{:else if providers.length === 0}
		<p class="text-sm text-muted-foreground">No notification providers configured.</p>
	{:else}
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
	{/if}
	{#if message}
		<p class="text-xs text-emerald-300">{message}</p>
	{/if}
</div>
