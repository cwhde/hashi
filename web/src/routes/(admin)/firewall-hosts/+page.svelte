<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Shield } from 'lucide-svelte';

	let script = $state('');
	let loading = $state(false);
	let error = $state<string | null>(null);
	let form = $state({
		name: 'edge-firewall',
		domain: '',
		managedSubnets: '192.168.0.0/16',
		linkedTraefikHost: '',
		internalTraefikIp: ''
	});

	async function render() {
		loading = true;
		error = null;
		try {
			const result = await api.renderFirewall({
				name: form.name,
				domain: form.domain,
				managedSubnets: form.managedSubnets.split(',').map((s) => s.trim()),
				linkedTraefikHost: form.linkedTraefikHost,
				internalTraefikIp: form.internalTraefikIp
			});
			script = result.script;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to render firewall script';
		} finally {
			loading = false;
		}
	}
</script>

<AdminSectionPage
	title="Firewall Hosts"
	description="Linux edge hosts, managed subnets, port forwards, and Hashi chains."
	icon={Shield}
>
	<PanelSection title="Render firewall script" description="Preview generated iptables script for a host profile.">
		<div class="grid max-w-xl gap-3">
			<div class="grid gap-1.5">
				<Label for="fw-name">Host name</Label>
				<Input id="fw-name" bind:value={form.name} />
			</div>
			<div class="grid gap-1.5">
				<Label for="fw-domain">Domain</Label>
				<Input id="fw-domain" bind:value={form.domain} />
			</div>
			<div class="grid gap-1.5">
				<Label for="fw-subnets">Managed subnets (comma-separated)</Label>
				<Input id="fw-subnets" bind:value={form.managedSubnets} />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="fw-traefik">Linked Traefik host</Label>
					<Input id="fw-traefik" bind:value={form.linkedTraefikHost} />
				</div>
				<div class="grid gap-1.5">
					<Label for="fw-ip">Internal Traefik IP</Label>
					<Input id="fw-ip" bind:value={form.internalTraefikIp} />
				</div>
			</div>
			<Button onclick={() => render()} disabled={loading}>
				{loading ? 'Rendering…' : 'Render script'}
			</Button>
		</div>
	</PanelSection>

	{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
	{#if script}
		<PanelSection title="Generated script" description="Review before apply via sync workflow.">
			<pre
				class="max-h-[32rem] overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px]">{script}</pre>
		</PanelSection>
	{/if}
</AdminSectionPage>
