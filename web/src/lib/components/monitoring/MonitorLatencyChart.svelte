<script lang="ts">
	import { onMount } from 'svelte';
	import uPlot from 'uplot';
	import 'uplot/dist/uPlot.min.css';

	let {
		timestamps,
		latencies,
		height = 160
	}: {
		timestamps: number[];
		latencies: number[];
		height?: number;
	} = $props();

	let container: HTMLDivElement | undefined = $state();
	let chart: uPlot | null = $state(null);

	onMount(() => {
		return () => {
			chart?.destroy();
			chart = null;
		};
	});

	$effect(() => {
		if (!container) return;
		chart?.destroy();
		if (timestamps.length === 0) {
			chart = null;
			return;
		}

		chart = new uPlot(
			{
				width: container.clientWidth || 400,
				height,
				series: [
					{},
					{
						label: 'Latency (ms)',
						stroke: '#FAD000',
						width: 2,
						fill: 'rgba(250, 208, 0, 0.12)'
					}
				],
				axes: [
					{
						stroke: '#A599E9',
						grid: { show: false }
					},
					{
						stroke: '#A599E9',
						grid: { stroke: 'rgba(165, 153, 233, 0.15)' }
					}
				],
				scales: {
					x: { time: true }
				}
			},
			[timestamps, latencies],
			container
		);
	});
</script>

<div bind:this={container} class="w-full rounded-md border border-border bg-hashi-bg-dark/40 p-2" style:min-height="{height}px">
	{#if timestamps.length === 0}
		<p class="py-8 text-center text-xs text-muted-foreground">No latency samples for this window.</p>
	{/if}
</div>
