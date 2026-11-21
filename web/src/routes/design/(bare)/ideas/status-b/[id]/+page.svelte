<script lang="ts">
	// Idea B, a service's own page: same parts as the side panel in idea A, with room for a wider chart.
	import { page } from '$app/state';
	import { ChevronLeft } from '@lucide/svelte';
	import IdeaDetail from '../../../../IdeaDetail.svelte';
	import { entries } from '../../../../ideas.ts';

	const e = $derived(entries.find((x) => x.id === page.params.id) ?? entries[0]!);
</script>

<svelte:head><title>{e.name} · example.com status (idea B)</title></svelte:head>
<div class="mx-auto max-w-4xl px-4 py-10">
	<a
		href="/design/ideas/status-b"
		class="mb-4 inline-flex items-center gap-1 text-md text-muted-foreground hover:text-foreground"
		><ChevronLeft class="size-4" />All services</a
	>
	<IdeaDetail {e} />
</div>
