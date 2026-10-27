<script lang="ts">
	import type { Health } from '$lib/design/mock';
	import StatusDot from './StatusDot.svelte';
	import { health } from './status';

	let { state, label }: { state: Health; label?: string } = $props();
</script>

<span class="inline-flex items-center gap-1.5 text-[13px] font-medium {health[state].text}">
	<StatusDot {state} />
	<span>{label ?? health[state].label}</span>
</span>
