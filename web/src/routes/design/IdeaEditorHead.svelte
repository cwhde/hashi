<script lang="ts">
	// Idea mock: where the editor lives. "App display" becomes "Public pages" with three tabs: Layout (this editor, both
	// pages), Look (logo, icon, background; out of Settings) and Settings (on/off, titles, default sort, badges).
	import PageHeader from '#lib/components/hashi/PageHeader.svelte';
	import * as Tabs from '#lib/components/ui/tabs/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import type { Snippet } from 'svelte';

	let {
		preview = $bindable('apps'),
		variant,
		extra
	}: { preview?: string; variant: string; extra?: Snippet } = $props();
</script>

<PageHeader title="Public pages">
	{#snippet actions()}
		<Button variant="outline" size="sm">Discard</Button><Button size="sm">Save layout</Button>
	{/snippet}
</PageHeader>
<Tabs.Root value="layout">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="layout">Layout</Tabs.Trigger>
		<Tabs.Trigger value="look">Look</Tabs.Trigger>
		<Tabs.Trigger value="settings">Settings</Tabs.Trigger>
	</Tabs.List>
</Tabs.Root>
<div class="mb-4 flex flex-wrap items-center gap-3">
	<span class="text-md text-muted-foreground">Show as</span>
	<ToggleGroup.Root type="single" bind:value={preview} variant="outline" size="sm">
		<ToggleGroup.Item value="apps">Apps page</ToggleGroup.Item>
		<ToggleGroup.Item value="status">Status page</ToggleGroup.Item>
	</ToggleGroup.Root>
	{@render extra?.()}
	<span class="ml-auto rounded-full border border-dashed px-3 py-1 text-xs text-muted-foreground"
		>idea {variant}</span
	>
</div>
