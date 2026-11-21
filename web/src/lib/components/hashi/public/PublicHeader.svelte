<script lang="ts">
	// The public pages' own look (07 §9, bug 30): the owner's icon in the tab, their logo in place of the title, and the
	// background when it is switched on. Each theme mode shows its own logo; without one it shows the title as text, so
	// a white logo never lands on a light page. Only the current mode's logo is loaded. Files come from Hashi only
	// (/assets/…).
	import { theme } from '#lib/theme.svelte.ts';
	import type { Schemas } from '#lib/api/client.ts';
	import type { Snippet } from 'svelte';

	let {
		title,
		assets,
		suffix = '',
		children
	}: {
		title: string;
		assets?: Schemas['PublicPageAssets'];
		suffix?: string;
		children?: Snippet;
	} = $props();
	const logo = $derived(theme.mode === 'light' ? assets?.logo_light : assets?.logo_dark);
</script>

<svelte:head>
	{#if assets?.icon}
		<link rel="icon" href={assets.icon} />
		<link rel="apple-touch-icon" href={assets.icon} />
	{/if}
</svelte:head>

{#if assets?.background}
	<div aria-hidden="true" class="fixed inset-0 -z-10">
		<img src={assets.background} alt="" class="size-full object-cover" />
		<!-- The scrim keeps text on the page at its usual contrast (10-ui §2). -->
		<div class="absolute inset-0 bg-background/85"></div>
	</div>
{/if}

<h1 class="flex min-w-48 flex-1 items-center gap-3 text-title font-semibold">
	{#if logo}<img
			src={logo}
			alt={title}
			class="h-10 w-auto max-w-64 object-contain"
		/>{:else}{title}{/if}
	{#if suffix}<span>{suffix}</span>{/if}
	{@render children?.()}
</h1>
