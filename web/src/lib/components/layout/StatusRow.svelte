<script lang="ts">
	import { cn } from '$lib/utils';

	let {
		label,
		value,
		status = 'neutral'
	}: {
		label: string;
		value: string;
		status?: 'ok' | 'warn' | 'error' | 'neutral';
	} = $props();

	const valueClass = $derived(
		status === 'ok'
			? 'text-emerald-300'
			: status === 'warn'
				? 'text-hashi-contrast'
				: status === 'error'
					? 'text-destructive'
					: 'text-white'
	);
</script>

<div class="flex items-center justify-between gap-3 py-1 text-xs">
	<span class="truncate text-muted-foreground">{label}</span>
	<span class={cn('shrink-0 font-medium tabular-nums', valueClass)}>{value}</span>
</div>
