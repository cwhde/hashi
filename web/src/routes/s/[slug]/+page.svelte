<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import PublicService from '#lib/components/hashi/public/PublicService.svelte';

	// A service page lives on the status listener only (07 §7.2); anywhere else this address means nothing.
	const publicPage = document.querySelector('meta[name="hashi-public"]')?.getAttribute('content');
	onMount(() => {
		if (publicPage !== 'status') goto('/', { replace: true });
	});
</script>

{#if publicPage === 'status'}
	{#key page.params.slug}<PublicService slug={page.params.slug ?? ''} />{/key}
{/if}
