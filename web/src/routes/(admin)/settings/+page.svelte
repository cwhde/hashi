<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import {
		DEFAULT_WIDGETS,
		loadWidgetPrefs,
		saveWidgetPrefs,
		type WidgetPrefs
	} from '$lib/overview/widgets';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Settings as SettingsIcon } from 'lucide-svelte';

	let saving = $state(false);
	let message = $state<string | null>(null);
	let widgetPrefs = $state<WidgetPrefs>(loadWidgetPrefs());
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

		<PanelSection
			title="Overview widgets"
			description="Toggle and reorder default overview widgets (stored locally until settings API ships)."
		>
			<ul class="space-y-2">
				{#each DEFAULT_WIDGETS as widget (widget.id)}
					<li class="flex items-center gap-3 rounded-md border border-border px-3 py-2">
						<Checkbox
							checked={widgetPrefs.enabled[widget.id]}
							onCheckedChange={(checked) => {
								widgetPrefs.enabled[widget.id] = checked === true;
								saveWidgetPrefs(widgetPrefs);
							}}
							id={`widget-${widget.id}`}
						/>
						<Label for={`widget-${widget.id}`} class="min-w-0 flex-1">
							<span class="block text-sm text-white">{widget.title}</span>
							<span class="block truncate text-xs text-muted-foreground">{widget.description}</span>
						</Label>
					</li>
				{/each}
			</ul>
		</PanelSection>
	</div>
</AdminSectionPage>
