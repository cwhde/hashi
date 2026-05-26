<script lang="ts">
	import '../app.css';
	import { TooltipProvider } from '$lib/components/ui/tooltip';

	let { children } = $props();
</script>

<TooltipProvider>
	<div class="dark min-h-screen bg-hashi-bg text-hashi-foreground">
		{@render children()}
	</div>
</TooltipProvider>
