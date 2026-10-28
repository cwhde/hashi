<script lang="ts">
	import Tag from '../Tag.svelte';
	import { setBySlug } from '$lib/design/security';
	import { Lock } from '@lucide/svelte';

	// A reference to an IP set: its name as a link plus kind, with a lock for sets the user cannot edit (05a §3).
	let { slug, showKind = true }: { slug: string; showKind?: boolean } = $props();
	const set = $derived(setBySlug(slug));
	const kindLabel = { user: 'yours', system: 'system', feed: 'feed', dynamic: 'dynamic' };
</script>

<span class="inline-flex min-w-0 items-center gap-1.5">
	<a
		href={set ? `/design/ip-sets/${set.id}` : '/design/ip-sets'}
		class="truncate font-medium hover:underline">{slug}</a
	>
	{#if set && showKind && set.kind !== 'user'}
		<Tag
			>{#if set.locked}<Lock class="size-3" aria-label="Not editable" />{/if}{kindLabel[
				set.kind
			]}</Tag
		>
	{/if}
	{#if set?.status === 'partial'}<Tag tone="warn">partial</Tag>{/if}
</span>
