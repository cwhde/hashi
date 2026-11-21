<script lang="ts">
	import './layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { syncIcon } from '#lib/theme.svelte.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	// app.html put the remembered theme on the page before this ran; the icon follows it, then every switch (bug 159).
	onMount(() => syncIcon());
</script>

<svelte:head>
	<link rel="icon" href={favicon} data-hashi />
</svelte:head>

{@render children()}
