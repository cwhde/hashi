<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	let solved = $state(false);
</script>

<svelte:head><title>Checking your connection</title></svelte:head>
<!-- Edge challenge page shown to visitors: minimal, no admin assets, no sign-in (06 §7.2). -->
<main class="grid min-h-dvh place-items-center px-4">
	<div class="w-full max-w-md rounded-3xl border bg-card p-7 text-center">
		<h1 class="text-[19px] font-semibold">Checking your connection</h1>
		<p class="mt-2 text-[13.5px] text-muted-foreground">
			git.example.com received a lot of traffic from your network. Complete the check to continue.
		</p>
		<div
			class="mx-auto mt-6 flex h-16 max-w-xs items-center justify-center rounded-xl border bg-surface-2 text-[13px]"
		>
			{#if solved}<span class="text-ok-text">Done. Taking you back…</span>{:else}<span
					class="text-muted-foreground">Cap widget</span
				>{/if}
		</div>
		<Button class="mt-5" onclick={() => (solved = true)}>Continue to git.example.com</Button>
	</div>
</main>
