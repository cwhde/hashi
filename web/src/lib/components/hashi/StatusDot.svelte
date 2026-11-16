<script lang="ts">
	import type { Health } from '#lib/design/mock.ts';
	import { cn } from '#lib/utils.ts';
	import { health } from './status';

	let { state, class: className = '' }: { state: Health; class?: string } = $props();
</script>

<span
	class={cn('inline-block size-2 shrink-0 rounded-full', health[state].dot, className)}
	role="img"
	aria-label={health[state].label}
></span>
