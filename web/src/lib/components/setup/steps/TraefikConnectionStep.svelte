<script lang="ts">
	import ApiPendingBanner, {
		apiUnavailable
	} from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let name = $state('primary-traefik');
	let host = $state('');
	let sshUser = $state('root');
	let sshPassword = $state('');
	let useKey = $state(true);
	let keyPassphrase = $state('');
	let internalIp = $state('');
	let configPath = $state('/etc/traefik');
	let replaceConfirmed = $state(false);

	const discoveredFiles = ['traefik.yml', 'dynamic/hashi.yml', 'certs/acme.json'];
</script>

<div class="grid max-w-xl gap-4">
	<div class="grid gap-1.5">
		<Label for="traefik-name">Connection name</Label>
		<Input id="traefik-name" bind:value={name} />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="traefik-host">Host / IP</Label>
			<Input id="traefik-host" bind:value={host} placeholder="10.0.0.5" />
		</div>
		<div class="grid gap-1.5">
			<Label for="traefik-user">SSH username</Label>
			<Input id="traefik-user" bind:value={sshUser} />
		</div>
	</div>
	<div class="flex items-center gap-2 text-xs">
		<Checkbox bind:checked={useKey} id="traefik-key" />
		<Label for="traefik-key">Use SSH private key instead of password</Label>
	</div>
	{#if useKey}
		<div class="grid gap-1.5">
			<Label for="traefik-passphrase">Private key passphrase (optional)</Label>
			<Input id="traefik-passphrase" type="password" bind:value={keyPassphrase} />
		</div>
	{:else}
		<div class="grid gap-1.5">
			<Label for="traefik-password">SSH password</Label>
			<Input id="traefik-password" type="password" bind:value={sshPassword} />
		</div>
	{/if}
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="traefik-internal">Internal Traefik IP</Label>
			<Input id="traefik-internal" bind:value={internalIp} />
		</div>
		<div class="grid gap-1.5">
			<Label for="traefik-path">Config path</Label>
			<Input id="traefik-path" bind:value={configPath} />
		</div>
	</div>

	<ApiPendingBanner
		message={apiUnavailable('Traefik connection validation')}
		detail="POST /api/setup/traefik/validate will SSH, detect OS, and check write permissions."
	/>

	<div class="rounded-md border border-border p-3 text-xs">
		<p class="mb-2 font-medium text-hashi-contrast">Discovered configs (preview)</p>
		<ul class="space-y-1 text-muted-foreground">
			{#each discoveredFiles as file}
				<li class="font-mono">{file}</li>
			{/each}
		</ul>
		<div class="mt-3 flex items-center gap-2">
			<Checkbox bind:checked={replaceConfirmed} id="traefik-replace" />
			<Label for="traefik-replace">Confirm backup and Hashi ownership if configs exist</Label>
		</div>
	</div>

	<div class="flex gap-2">
		<Button variant="outline" disabled>Validate SSH</Button>
		<Button onclick={() => oncomplete()} disabled={advancing}>Save & continue</Button>
	</div>
</div>
