<script lang="ts">
	import Mono from '../Mono.svelte';
	import { addressHref } from '$lib/ipsets';

	// Every address in the UI opens the address page (05a §6).
	let { address, class: className = '' }: { address: string; class?: string } = $props();
</script>

<a href={addressHref(address)} class="hover:underline {className}"><Mono>{address}</Mono></a>
