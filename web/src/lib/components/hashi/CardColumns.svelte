<script lang="ts">
	import type { Snippet } from 'svelte';

	// Cards of different heights side by side (bug 158): each column stacks on its own, so a short card never leaves a
	// hole beside a tall one; the next card in its column moves up. One column on narrow screens, left first.
	let { left, right }: { left: Snippet; right: Snippet } = $props();
</script>

<div class="grid items-start gap-4 lg:grid-cols-2">
	<div class="flex min-w-0 flex-col gap-4">{@render left()}</div>
	<div class="flex min-w-0 flex-col gap-4">{@render right()}</div>
</div>
