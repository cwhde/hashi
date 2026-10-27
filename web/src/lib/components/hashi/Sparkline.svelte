<script lang="ts">
	// Thin line, no fill, no axes (10a §5.2 item 11). Colour comes from the parent's text colour.
	let {
		values,
		width = 120,
		height = 28,
		class: className = ''
	}: { values: number[]; width?: number; height?: number; class?: string } = $props();

	const points = $derived.by(() => {
		const max = Math.max(...values, 1);
		const min = Math.min(...values, 0);
		const step = width / Math.max(values.length - 1, 1);
		return values
			.map(
				(v, i) =>
					`${(i * step).toFixed(1)},${(height - 2 - ((v - min) / (max - min || 1)) * (height - 4)).toFixed(1)}`
			)
			.join(' ');
	});
</script>

<svg
	class={className}
	viewBox="0 0 {width} {height}"
	{width}
	{height}
	aria-hidden="true"
	preserveAspectRatio="none"
>
	<polyline
		{points}
		fill="none"
		stroke="currentColor"
		stroke-width="1.5"
		stroke-linejoin="round"
		vector-effect="non-scaling-stroke"
	/>
</svg>
