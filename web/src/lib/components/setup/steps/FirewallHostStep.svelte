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

	let name = $state('edge-firewall');
	let host = $state('');
	let sshUser = $state('root');
	let subnets = $state('192.168.0.0/16');
	let traefikLink = $state('primary-traefik');
	let traefikTargetIp = $state('');
	let wanOverride = $state('');
</script>

<div class="grid max-w-xl gap-4">
	<div class="grid gap-1.5">
		<Label for="fw-name">Host name</Label>
		<Input id="fw-name" bind:value={name} />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="fw-host">Host / IP</Label>
			<Input id="fw-host" bind:value={host} />
		</div>
		<div class="grid gap-1.5">
			<Label for="fw-user">SSH username</Label>
			<Input id="fw-user" bind:value={sshUser} />
		</div>
	</div>
	<div class="grid gap-1.5">
		<Label for="fw-subnets">Managed subnets (comma-separated CIDRs)</Label>
		<Input id="fw-subnets" bind:value={subnets} />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="fw-traefik">Linked Traefik connection</Label>
			<Input id="fw-traefik" bind:value={traefikLink} />
		</div>
		<div class="grid gap-1.5">
			<Label for="fw-target">Internal Traefik target IP</Label>
			<Input id="fw-target" bind:value={traefikTargetIp} />
		</div>
	</div>
	<div class="grid gap-1.5">
		<Label for="fw-wan">Optional WAN interface override</Label>
		<Input id="fw-wan" bind:value={wanOverride} placeholder="eth0" />
	</div>

	<ApiPendingBanner
		message={apiUnavailable('Firewall host validation')}
		detail="POST /api/setup/firewall/validate will verify iptables, ipset, and managed subnet reachability."
	/>

	<div class="flex gap-2">
		<Button variant="outline" disabled>Validate host</Button>
		<Button onclick={() => oncomplete()} disabled={advancing}>Save & continue</Button>
	</div>
</div>
