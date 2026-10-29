<script lang="ts">
	import Tag from '../Tag.svelte';
	import { Lock } from '@lucide/svelte';
	import type { IPSet } from '$lib/ipsets';

	// A reference to an IP set: its slug as a link plus kind, with a lock for sets the user cannot edit (05a §3).
	let { set, showKind = true }: { set: IPSet; showKind?: boolean } = $props();
	const kindText = { user: 'yours', system: 'system', feed: 'feed', dynamic: 'dynamic' };
</script>

<span class="inline-flex min-w-0 items-center gap-1.5">
	<a href="/ip-sets/{set.id}" class="truncate font-medium hover:underline">{set.slug}</a>
	{#if showKind && set.kind !== 'user'}
		<Tag
			>{#if !set.editable}<Lock class="size-3" aria-label="Not editable" />{/if}{kindText[
				set.kind
			]}</Tag
		>
	{/if}
	{#if set.resolution_status === 'partial'}<Tag tone="warn">partial</Tag>{/if}
</span>
