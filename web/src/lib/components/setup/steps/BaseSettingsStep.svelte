<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let saving = $state(false);
	let error = $state<string | null>(null);
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
				defaultSyncIntervalMinutes: settings.defaultSyncIntervalMinutes,
				publicDashboardEnabled: settings.publicDashboardEnabled,
				publicStatusEnabled: settings.publicStatusEnabled,
				theme: settings.theme ?? 'dark'
			};
		} catch {
			// defaults are fine for first load
		}
	});

	async function saveAndContinue() {
		saving = true;
		error = null;
		try {
			await api.updateGeneralSettings({
				rootDomain: form.rootDomain || undefined,
				adminDomain: form.adminDomain || undefined,
				internalUrl: form.internalUrl || undefined,
				defaultSyncIntervalMinutes: form.defaultSyncIntervalMinutes,
				publicDashboardEnabled: form.publicDashboardEnabled,
				publicStatusEnabled: form.publicStatusEnabled,
				theme: form.theme
			});
			await oncomplete();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}
</script>

<div class="grid max-w-xl gap-4">
	<div class="grid gap-1.5">
		<Label for="root-domain">Root domain</Label>
		<Input id="root-domain" bind:value={form.rootDomain} placeholder="example.com" />
	</div>
	<div class="grid gap-1.5">
		<Label for="admin-domain">Admin public Hashi domain</Label>
		<Input id="admin-domain" bind:value={form.adminDomain} placeholder="hashi.example.com" />
	</div>
	<div class="grid gap-1.5">
		<Label for="internal-url">Internal Hashi URL / IP and port</Label>
		<Input id="internal-url" bind:value={form.internalUrl} placeholder="http://192.168.1.10:8080" />
	</div>
	<div class="grid gap-1.5">
		<Label for="sync-interval">Default sync interval (minutes)</Label>
		<Input id="sync-interval" type="number" min="5" bind:value={form.defaultSyncIntervalMinutes} />
	</div>
	<div class="flex items-center justify-between gap-4 rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Public app dashboard (8081)</p>
			<p class="text-xs text-muted-foreground">Homelab service tiles for visitors.</p>
		</div>
		<Switch bind:checked={form.publicDashboardEnabled} />
	</div>
	<div class="flex items-center justify-between gap-4 rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Public status page (8082)</p>
			<p class="text-xs text-muted-foreground">External uptime and incident view.</p>
		</div>
		<Switch bind:checked={form.publicStatusEnabled} />
	</div>
	<div class="grid gap-1.5">
		<Label for="theme">Theme preference</Label>
		<Input id="theme" bind:value={form.theme} placeholder="dark" />
	</div>
	{#if error}
		<p class="text-xs text-destructive">{error}</p>
	{/if}
	<div class="flex justify-end pt-2">
		<Button onclick={() => saveAndContinue()} disabled={saving || advancing}>
			{saving ? 'Saving…' : 'Save & continue'}
		</Button>
	</div>
</div>
