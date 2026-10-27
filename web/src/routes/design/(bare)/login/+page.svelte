<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Fingerprint } from '@lucide/svelte';
	let failed = $state(false);
</script>

<svelte:head><title>Sign in · Hashi</title></svelte:head>
<main class="grid min-h-dvh place-items-center px-4">
	<div class="w-full max-w-sm rounded-3xl border bg-card p-7">
		<span
			class="grid size-10 place-items-center rounded-xl bg-primary text-[20px] font-semibold text-primary-foreground"
			>橋</span
		>
		<h1 class="mt-5 text-[20px] font-semibold">Sign in to Hashi</h1>
		<p class="mt-1 text-[13.5px] text-muted-foreground">hashi.example.com</p>
		{#if failed}
			<p class="mt-5 rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text">
				The passkey prompt was closed or timed out. Try again, or use another passkey on this
				device.
			</p>
		{/if}
		<Button class="mt-6 h-10 w-full" onclick={() => (failed = !failed)}
			><Fingerprint />Sign in with a passkey</Button
		>
		<p class="mt-5 text-[12.5px] text-muted-foreground">
			Lost every passkey? Run <code class="font-mono">hashi admin recover</code> on the server for a one-time
			link.
		</p>
	</div>
</main>
