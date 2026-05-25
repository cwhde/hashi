<script lang="ts">
	import ApiPendingBanner, {
		apiUnavailable
	} from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let acmeEmail = $state('');
	let eabKeyId = $state('');
	let eabHmac = $state('');
	let dnsDelay = $state(30);
	let resolvers = $state('1.1.1.1,8.8.8.8');
</script>

<div class="grid max-w-xl gap-4">
	<div class="grid gap-1.5">
		<Label for="acme-email">ACME email</Label>
		<Input id="acme-email" type="email" bind:value={acmeEmail} />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="eab-key-id">EAB key ID</Label>
			<Input id="eab-key-id" bind:value={eabKeyId} />
		</div>
		<div class="grid gap-1.5">
			<Label for="eab-hmac">EAB HMAC</Label>
			<Input id="eab-hmac" type="password" bind:value={eabHmac} />
		</div>
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="dns-delay">DNS challenge delay (seconds)</Label>
			<Input id="dns-delay" type="number" bind:value={dnsDelay} />
		</div>
		<div class="grid gap-1.5">
			<Label for="resolvers">Resolver list</Label>
			<Input id="resolvers" bind:value={resolvers} />
		</div>
	</div>

	<ApiPendingBanner
		message={apiUnavailable('Certificate provider setup')}
		detail="POST /api/setup/certificate/validate will bind ACME to the configured DNS provider."
	/>

	<div class="flex gap-2">
		<Button variant="outline" disabled>Validate ACME</Button>
		<Button onclick={() => oncomplete()} disabled={advancing}>Save & continue</Button>
	</div>
</div>
