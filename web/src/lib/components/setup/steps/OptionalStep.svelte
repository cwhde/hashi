<script lang="ts">
	import ApiPendingBanner, {
		apiUnavailable
	} from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let oidc = $state(false);
	let adguard = $state(false);
	let notifications = $state(false);
	let geoip = $state(false);
</script>

<div class="grid max-w-xl gap-3">
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">OIDC SSO provider</p>
			<p class="text-xs text-muted-foreground">Optional edge SSO during setup.</p>
		</div>
		<Switch bind:checked={oidc} disabled />
	</div>
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">AdGuard Home</p>
			<p class="text-xs text-muted-foreground">Internal DNS rewrite integration.</p>
		</div>
		<Switch bind:checked={adguard} disabled />
	</div>
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Notification provider</p>
			<p class="text-xs text-muted-foreground">Incident and sync notifications.</p>
		</div>
		<Switch bind:checked={notifications} disabled />
	</div>
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">GeoLite2 databases</p>
			<p class="text-xs text-muted-foreground">MaxMind account for abuse geo signals.</p>
		</div>
		<Switch bind:checked={geoip} disabled />
	</div>

	<ApiPendingBanner
		message={apiUnavailable('Optional setup providers')}
		detail="Optional setup endpoints will activate when backend publishes provider configuration APIs."
	/>

	<div class="flex justify-end gap-2 pt-2">
		<Button variant="outline" onclick={() => oncomplete()} disabled={advancing}>Skip optional</Button>
	</div>
</div>
