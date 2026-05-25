<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import ApiPendingBanner from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Lock } from 'lucide-svelte';

	const ranges = ['1 hour', '24 hours', '7 days', '30 days'] as const;
	let range = $state<(typeof ranges)[number]>('24 hours');
	let resourceFilter = $state('');
</script>

<section class="mx-auto max-w-7xl space-y-6">
	<PageHeader
		title="Security"
		description="Edge abuse visibility, WAF detections, and active blocks."
		icon={Lock}
	/>

	<ApiPendingBanner
		message="Waiting for Security analytics API"
		detail="Metrics below are layout placeholders per spec §19 until /api/security/* endpoints ship."
	/>

	<div class="flex flex-wrap items-end gap-4">
		<div class="grid gap-1.5">
			<Label for="sec-range">Time range</Label>
			<select
				id="sec-range"
				bind:value={range}
				class="rounded-md border border-border bg-card px-3 py-2 text-sm text-white"
			>
				{#each ranges as item}
					<option value={item}>{item}</option>
				{/each}
			</select>
		</div>
		<div class="grid min-w-[12rem] flex-1 gap-1.5">
			<Label for="sec-resource">Resource filter</Label>
			<Input id="sec-resource" bind:value={resourceFilter} placeholder="All resources" />
		</div>
	</div>

	<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
		<OverviewWidget title="Allowed" description="{range} traffic allowed.">
			<StatusRow label="Requests" value="—" />
		</OverviewWidget>
		<OverviewWidget title="Blocked" description="{range} traffic blocked.">
			<StatusRow label="Requests" value="—" status="error" />
		</OverviewWidget>
		<OverviewWidget title="Challenged" description="{range} challenged requests.">
			<StatusRow label="Requests" value="—" status="warn" />
		</OverviewWidget>
		<OverviewWidget title="WAF" description="Detections and blocks.">
			<StatusRow label="Detections" value="—" />
			<StatusRow label="Blocks" value="—" status="error" />
		</OverviewWidget>
	</div>

	<div class="grid gap-4 xl:grid-cols-2">
		<OverviewWidget title="Top blocked IPs" description="Count, geo, ASN, reason, expiry.">
			<p class="text-xs text-muted-foreground">No block data yet.</p>
		</OverviewWidget>
		<OverviewWidget title="Top blocked countries" description="By request count.">
			<p class="text-xs text-muted-foreground">No block data yet.</p>
		</OverviewWidget>
		<OverviewWidget title="Top blocked ASNs" description="By request count.">
			<p class="text-xs text-muted-foreground">No block data yet.</p>
		</OverviewWidget>
		<OverviewWidget title="Top targeted resources" description="Blocked or challenged traffic.">
			<p class="text-xs text-muted-foreground">No block data yet.</p>
		</OverviewWidget>
	</div>

	<OverviewWidget title="Recent security events" description="Latest abuse and WAF events.">
		<p class="text-xs text-muted-foreground">Event stream will appear when backend publishes security events.</p>
	</OverviewWidget>
</section>
