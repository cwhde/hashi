<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { hrefWithTab, tabFrom } from '#lib/url-tab.ts';
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Tabs from '#lib/components/ui/tabs/index.ts';
	import { Settings } from '@lucide/svelte';
	import SecurityDashboard from '#lib/components/hashi/security/SecurityDashboard.svelte';
	import CountersView from '#lib/components/hashi/security/CountersView.svelte';

	// What happened at the edge and what to do about an address (10c §1.1). The engine, CAPTCHA and session limits are
	// in Settings → Edge protection; sign-in providers and GeoIP on Connections.
	const tabs = ['dash', 'counters'];
	let tab = $state(tabFrom(page.url, tabs));
	const setTab = (t: string) =>
		goto(hrefWithTab(page.url, tabs, t), { replace: true, reset: false });
</script>

<svelte:head><title>Security · Hashi</title></svelte:head>

<PageHeader title="Security">
	{#snippet actions()}<Button
			variant="outline"
			size="icon"
			href="/settings?category=edge"
			aria-label="Edge protection settings"><Settings /></Button
		>{/snippet}
</PageHeader>

<Tabs.Root bind:value={tab} onValueChange={setTab}>
	<Tabs.List class="mb-4 max-w-full justify-start overflow-x-auto">
		<Tabs.Trigger value="dash">Dashboard</Tabs.Trigger>
		<Tabs.Trigger value="counters">Counters</Tabs.Trigger>
	</Tabs.List>
	<!-- Built when opened, so both show fresh numbers each time. -->
	<Tabs.Content value="dash"
		>{#if tab === 'dash'}<SecurityDashboard />{/if}</Tabs.Content
	>
	<Tabs.Content value="counters"
		>{#if tab === 'counters'}<CountersView />{/if}</Tabs.Content
	>
</Tabs.Root>
