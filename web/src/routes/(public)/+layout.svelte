<script lang="ts">
	let { children } = $props();
</script>

<div class="min-h-screen bg-hashi-bg text-hashi-foreground">
	<header class="border-b border-border bg-hashi-bg-dark/80 px-6 py-4 backdrop-blur-sm">
		<div class="mx-auto flex max-w-6xl items-center justify-between gap-4">
			<div>
				<p class="text-sm font-semibold text-white">Hashi</p>
				<p class="text-xs text-muted-foreground">Public view — read only</p>
			</div>
		</div>
	</header>
	<main class="mx-auto max-w-6xl p-6">
		{@render children()}
	</main>
</div>
