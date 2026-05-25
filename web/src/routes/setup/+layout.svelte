<script lang="ts">
	let { children } = $props();
</script>

<div class="flex min-h-screen items-center justify-center p-6">
	<div class="w-full max-w-6xl">
		{@render children()}
	</div>
</div>
