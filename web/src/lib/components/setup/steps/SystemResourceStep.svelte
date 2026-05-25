<script lang="ts">
	import ApiPendingBanner, {
		apiUnavailable
	} from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();
</script>

<Alert>
	<AlertTitle>Hashi system resource</AlertTitle>
	<AlertDescription>
		Hashi creates a non-deletable admin domain resource, syncs DNS/Traefik/firewall state, and waits
		for HTTPS access on the configured admin domain.
	</AlertDescription>
</Alert>

<ApiPendingBanner
	class="mt-4"
	message={apiUnavailable('System resource sync')}
	detail="POST /api/setup/system-resource/sync will apply DNS, Traefik, and firewall plans for the admin domain."
/>

<div class="mt-4 flex gap-2">
	<Button variant="outline" disabled>Preview sync plan</Button>
	<Button onclick={() => oncomplete()} disabled={advancing}>Continue after sync</Button>
</div>
