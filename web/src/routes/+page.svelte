<script lang="ts">
	import { LayoutDashboard } from 'lucide-svelte';
</script>

<section class="mx-auto max-w-5xl space-y-6">
	<div class="flex items-center gap-3">
		<div
			class="flex size-10 items-center justify-center rounded-lg bg-hashi-hover/20 text-hashi-contrast"
		>
			<LayoutDashboard class="size-5" aria-hidden="true" />
		</div>
		<div>
			<h2 class="text-2xl font-semibold text-white">Overview</h2>
			<p class="text-sm text-muted-foreground">Homelab edge orchestration at a glance.</p>
		</div>
	</div>

	<div
		class="rounded-xl border border-border bg-card/60 p-8 text-center text-muted-foreground backdrop-blur-sm"
	>
		<p class="text-sm">Overview dashboard placeholder — routes, DNS, and status widgets coming soon.</p>
	</div>
</section>
