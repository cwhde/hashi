<script lang="ts">
	import ApiPendingBanner from '$lib/components/layout/ApiPendingBanner.svelte';
	import StatusRow from '$lib/components/layout/StatusRow.svelte';
	import OverviewWidget from '$lib/components/overview/OverviewWidget.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Search } from 'lucide-svelte';

	let search = $state('');
</script>

<section class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-white">Status</h1>
		<p class="text-sm text-muted-foreground">Public uptime view on port 8082.</p>
	</div>

	<ApiPendingBanner
		message="Waiting for public status API"
		detail="Monitor rows and 60-minute strips will wire to /api/public/status when backend ships monitoring endpoints."
	/>

	<div class="relative max-w-md">
		<Search class="absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
		<Input bind:value={search} placeholder="Search monitored services…" class="pl-9" />
	</div>

	<div class="grid gap-4 lg:grid-cols-2">
		<OverviewWidget title="Overall uptime" description="Aggregate public status summary.">
			<StatusRow label="Services up" value="0 / 0" status="ok" />
			<StatusRow label="Degraded" value="0" status="warn" />
			<StatusRow label="Down" value="0" status="error" />
		</OverviewWidget>
		<OverviewWidget title="Recent incidents" description="Latest public incidents.">
			<StatusRow label="Open incidents" value="0" />
			<StatusRow label="Last 24h events" value="0" />
		</OverviewWidget>
	</div>

	<div class="overflow-hidden rounded-lg border border-border">
		<div class="grid grid-cols-[1fr_6rem_5rem_5rem] gap-2 border-b border-border bg-card/40 px-3 py-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
			<span>Service</span>
			<span>State</span>
			<span>Latency</span>
			<span>60m</span>
		</div>
		<p class="px-3 py-6 text-sm text-muted-foreground">No monitored endpoints yet.</p>
	</div>
</section>
