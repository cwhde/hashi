<script lang="ts">
	// Settings (09 §2, 10c §1.2): one category per subsystem, in the navigation's order, one component each (bug 46).
	// Categories stay mounted once opened, so a draft survives switching to another one (bug 91).
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import { cn } from '#lib/utils.ts';
	import GeneralSettings from '#lib/components/hashi/settings/GeneralSettings.svelte';
	import SecuritySettings from '#lib/components/hashi/settings/SecuritySettings.svelte';
	import EdgeProtection from '#lib/components/hashi/settings/EdgeProtection.svelte';
	import TraefikSettings from '#lib/components/hashi/settings/TraefikSettings.svelte';
	import AgentDns from '#lib/components/hashi/pulse/AgentDns.svelte';
	import FirewallSettings from '#lib/components/hashi/settings/FirewallSettings.svelte';
	import PulseSettings from '#lib/components/hashi/pulse/PulseSettings.svelte';
	import MonitoringSettings from '#lib/components/hashi/settings/MonitoringSettings.svelte';
	import PublicPages from '#lib/components/hashi/settings/PublicPages.svelte';
	import AppearanceSettings from '#lib/components/hashi/settings/AppearanceSettings.svelte';
	import type { Component } from 'svelte';

	const cats: { id: string; label: string; view: Component }[] = [
		{ id: 'general', label: 'General', view: GeneralSettings },
		{ id: 'security', label: 'Security', view: SecuritySettings },
		{ id: 'edge', label: 'Edge protection', view: EdgeProtection },
		{ id: 'traefik', label: 'Services and Traefik', view: TraefikSettings },
		{ id: 'dns', label: 'DNS and agent names', view: AgentDns },
		{ id: 'firewall', label: 'Firewall', view: FirewallSettings },
		{ id: 'pulse', label: 'Pulse', view: PulseSettings },
		{ id: 'monitoring', label: 'Monitoring and notifications', view: MonitoringSettings },
		{ id: 'public', label: 'Public pages', view: PublicPages },
		{ id: 'appearance', label: 'Appearance', view: AppearanceSettings }
	];
	// Links from before the regroup land on the category that holds the setting now (bug 36).
	const moved: Record<string, string> = {
		account: 'security',
		dashboard: 'public',
		notifications: 'monitoring',
		'agent-dns': 'dns'
	};
	const asked = $derived(page.url.searchParams.get('category') ?? 'general');
	const cat = $derived(moved[asked] ?? (cats.some((c) => c.id === asked) ? asked : 'general'));
	const show = (id: string) => goto(`?category=${id}`, { replace: true, reset: false });
	// The address names the category shown, so an old or unknown name is replaced by the one that holds it now.
	$effect(() => {
		if (asked !== cat && page.url.searchParams.has('category')) show(cat);
	});

	let opened = $state<string[]>([]);
	$effect(() => {
		if (!opened.includes(cat)) opened = [...opened, cat];
	});
</script>

<svelte:head><title>Settings · Hashi</title></svelte:head>

<PageHeader title="Settings" />
<div class="grid gap-5 lg:grid-cols-[14rem_minmax(0,1fr)]">
	<nav aria-label="Settings categories" class="flex gap-1 overflow-x-auto lg:flex-col">
		{#each cats as c (c.id)}
			<button
				onclick={() => show(c.id)}
				aria-current={cat === c.id ? 'page' : undefined}
				class={cn(
					'h-9 shrink-0 rounded-[10px] px-3 text-left text-md',
					cat === c.id ? 'bg-surface-2 font-medium' : 'text-muted-foreground hover:text-foreground'
				)}>{c.label}</button
			>
		{/each}
	</nav>

	<div class="min-w-0">
		{#each cats as c (c.id)}
			{#if opened.includes(c.id)}
				<div class="space-y-4" hidden={cat !== c.id}><c.view /></div>
			{/if}
		{/each}
	</div>
</div>
