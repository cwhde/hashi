<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let preview = $state<string | null>(null);
	let message = $state<string | null>(null);
	let error = $state<string | null>(null);
	let busy = $state(false);

	async function plan() {
		busy = true;
		error = null;
		message = null;
		try {
			const body = await api.planSystemResourceSync();
			preview = body.previewMarkdown ?? null;
			message = body.message ?? 'Plan ready.';
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to plan system resource sync';
		} finally {
			busy = false;
		}
	}

	async function syncAndContinue() {
		busy = true;
		error = null;
		try {
			const body = await api.syncSystemResource();
			message = body.message ?? 'Sync applied.';
			await oncomplete();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'System resource sync failed';
		} finally {
			busy = false;
		}
	}

	async function verifyHttps() {
		busy = true;
		error = null;
		message = null;
		try {
			const result = await api.verifySetupHttps();
			if (result.verified) {
				message = 'HTTPS admin domain verified. Continue to passkey setup.';
			} else {
				error = result.error ?? 'HTTPS verification failed. Open Hashi on the admin HTTPS URL first.';
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'HTTPS verification failed';
		} finally {
			busy = false;
		}
	}
</script>

<Alert>
	<AlertTitle>Hashi system resource</AlertTitle>
	<AlertDescription>
		Hashi creates a non-deletable admin domain resource, syncs DNS/Traefik/firewall state, and waits
		for HTTPS access on the configured admin domain.
	</AlertDescription>
</Alert>

{#if error}
	<p class="mt-4 text-sm text-destructive">{error}</p>
{/if}
{#if message}
	<p class="mt-4 text-sm text-muted-foreground">{message}</p>
{/if}
{#if preview}
	<pre class="mt-2 max-h-48 overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px]">{preview}</pre>
{/if}

<div class="mt-4 flex flex-wrap gap-2">
	<Button variant="outline" onclick={() => plan()} disabled={busy || advancing}>Preview sync plan</Button>
	<Button variant="outline" onclick={() => verifyHttps()} disabled={busy || advancing}>
		Verify HTTPS access
	</Button>
	<Button onclick={() => syncAndContinue()} disabled={busy || advancing}>
		{busy ? 'Syncing…' : 'Sync and continue'}
	</Button>
</div>
