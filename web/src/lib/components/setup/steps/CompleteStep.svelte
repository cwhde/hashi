<script lang="ts">
	import { navigate } from '$lib/navigation';
	import { api, ApiRequestError } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { PartyPopper } from 'lucide-svelte';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let finishing = $state(false);
	let error = $state<string | null>(null);

	async function finishSetup() {
		finishing = true;
		error = null;
		try {
			const result = await api.completeSetup();
			if (!result.succeeded) {
				error = result.error ?? 'Setup completion requirements not met.';
				return;
			}
			await oncomplete();
			await navigate('/');
		} catch (e) {
			error =
				e instanceof ApiRequestError
					? (e.body?.error ?? e.message)
					: 'Failed to complete setup';
		} finally {
			finishing = false;
		}
	}
</script>

<div class="flex flex-col items-start gap-4 py-6">
	<div class="flex items-center gap-3">
		<PartyPopper class="size-8 text-hashi-contrast" />
		<div>
			<p class="text-lg font-semibold text-white">Setup complete</p>
			<p class="text-sm text-muted-foreground">
				Bootstrap credentials are discarded and passkey auth becomes the admin login method.
			</p>
		</div>
	</div>
	{#if error}
		<p class="text-xs text-destructive">{error}</p>
	{/if}
	<Button onclick={() => finishSetup()} disabled={advancing || finishing}>
		{finishing ? 'Finalizing…' : 'Enter admin shell'}
	</Button>
</div>
